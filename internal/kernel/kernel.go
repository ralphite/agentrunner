// Package kernel is the actor runtime: each actor is a goroutine draining
// a mailbox channel; the bus routes envelopes by target (send) or fans out
// by type (publish). No wall-clock access here — time comes in as events.
//
// Locking rule: b.mu protects the actor/subscription tables ONLY. No
// mailbox send ever happens while holding it — a blocking send under the
// lock wedges the whole bus the moment one mailbox fills (the drain path
// and Close both need the lock).
package kernel

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ralphite/agentrunner/internal/event"
)

// HandlerFunc processes one envelope and returns child envelopes. The
// actor stamps each child with causation/correlation (ChildOf) and routes
// it: non-empty Target → send, empty Target → publish by type.
type HandlerFunc func(ctx context.Context, env event.Envelope) ([]event.Envelope, error)

const mailboxSize = 64

type actor struct {
	name    string
	inbox   chan event.Envelope
	handler HandlerFunc
	seen    map[string]bool
	dead    bool
}

// Bus owns all actors of one session runtime.
type Bus struct {
	mu     sync.Mutex
	actors map[string]*actor
	subs   map[string][]string // event type → subscriber actor names
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
}

func NewBus() *Bus {
	ctx, cancel := context.WithCancel(context.Background())
	return &Bus{
		actors: map[string]*actor{},
		subs:   map[string][]string{},
		ctx:    ctx,
		cancel: cancel,
	}
}

// Register starts an actor goroutine. Registering a duplicate name or
// registering on a closed bus panics — wiring is static, done at startup.
func (b *Bus) Register(name string, h HandlerFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		panic("kernel: register on closed bus")
	}
	if _, ok := b.actors[name]; ok {
		panic(fmt.Sprintf("kernel: actor %q already registered", name))
	}
	a := &actor{
		name:    name,
		inbox:   make(chan event.Envelope, mailboxSize),
		handler: h,
		seen:    map[string]bool{},
	}
	b.actors[name] = a
	b.wg.Add(1)
	go b.run(a)
}

// Subscribe routes published envelopes of the given type to the actor.
func (b *Bus) Subscribe(name, eventType string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.actors[name]; !ok {
		panic(fmt.Sprintf("kernel: subscribe: unknown actor %q", name))
	}
	b.subs[eventType] = append(b.subs[eventType], name)
}

// lookup resolves a live target under the lock; delivery happens outside.
func (b *Bus) lookup(target string) (*actor, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errors.New("kernel: bus closed")
	}
	a, ok := b.actors[target]
	if !ok {
		return nil, fmt.Errorf("kernel: unknown target %q", target)
	}
	if a.dead {
		return nil, fmt.Errorf("kernel: actor %q crashed, not restarted", target)
	}
	return a, nil
}

// Send routes an envelope to its Target's mailbox. It may block on a full
// mailbox (backpressure) but never while holding the bus lock.
func (b *Bus) Send(env event.Envelope) error {
	a, err := b.lookup(env.Target)
	if err != nil {
		return err
	}
	select {
	case a.inbox <- env:
		return nil
	case <-b.ctx.Done():
		return errors.New("kernel: bus closed")
	}
}

// Publish fans an envelope out to every subscriber of its type. Dead or
// missing subscribers are skipped — publish is best-effort by design.
func (b *Bus) Publish(env event.Envelope) {
	b.mu.Lock()
	names := append([]string(nil), b.subs[env.Type]...)
	b.mu.Unlock()
	for _, name := range names {
		a, err := b.lookup(name)
		if err != nil {
			continue
		}
		delivery := env
		delivery.Target = name
		select {
		case a.inbox <- delivery:
		case <-b.ctx.Done():
			return
		}
	}
}

// Close stops delivery and waits for all actor goroutines to exit.
// Mailboxes are never closed — actors exit via ctx, and undrained
// envelopes are dropped with the bus.
func (b *Bus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.mu.Unlock()
	b.cancel()
	b.wg.Wait()
}

func (b *Bus) run(a *actor) {
	defer b.wg.Done()
	for {
		var env event.Envelope
		select {
		case env = <-a.inbox:
		case <-b.ctx.Done():
			return
		}
		// Idempotent dedup by envelope id: a command delivered twice is
		// processed once. In-memory is enough — replay-time dedup falls
		// out of the fold, not the mailbox.
		if env.ID != "" {
			if a.seen[env.ID] {
				continue
			}
			a.seen[env.ID] = true
		}
		children, err := safeHandle(a.handler, b.ctx, env)
		if err != nil {
			b.crash(a, env, err)
			return
		}
		for _, child := range children {
			child = child.ChildOf(env)
			if child.Target != "" {
				if serr := b.Send(child); serr != nil {
					b.crash(a, env, fmt.Errorf("routing child: %w", serr))
					return
				}
			} else {
				b.Publish(child)
			}
		}
	}
}

// crash marks the actor dead (no auto-restart) and publishes ActorCrashed
// as a child of the envelope being handled.
func (b *Bus) crash(a *actor, cause event.Envelope, err error) {
	b.mu.Lock()
	a.dead = true
	b.mu.Unlock()
	crashed, cerr := event.New(event.TypeActorCrashed,
		&event.ActorCrashed{Actor: a.name, Error: err.Error()})
	if cerr != nil {
		return
	}
	b.Publish(crashed.ChildOf(cause))
}

func safeHandle(h HandlerFunc, ctx context.Context, env event.Envelope) (children []event.Envelope, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return h(ctx, env)
}
