// Package kill holds the live cancel table behind the user's kill gesture.
// It sits below both the agent loop (which publishes into it) and the daemon
// (which fires from it), so neither has to import the other.
package kill

import (
	"context"
	"sort"
	"sync"

	"github.com/ralphite/agentrunner/internal/errs"
)

// Registry is an agent tree's live cancel table: every in-flight tool call,
// background command and spawned child publishes its own cancel here for as
// long as it runs. A user kill looks the target up and fires it —
// synchronously, on the caller's goroutine, cutting exactly that unit and
// nothing else.
//
// Two properties are load-bearing and deliberate:
//
//   - INSTANT. The registry is plain memory reached straight from the wire
//     handler; a kill never queues behind the drive loop's safe points. That
//     matters precisely because the loop is usually parked inside a tool
//     batch when a user wants out — the moment a safe-point drain would have
//     to wait for is the moment they are trying to escape.
//   - STATELESS. Firing a cancel writes nothing durable: no mark, no gate,
//     no receipt. The journal records what happened to the WORK (an
//     ActivityCancelled terminal) and nothing about the session. So a
//     scheduled session killed mid-run starts again on its next tick, and
//     pausing the schedule — not killing a run — is how you stop that.
//
// One registry is created per tree at the root and inherited by every
// member, so a kill aimed at a grandchild's tool call is reachable from the
// session the user is actually looking at.
type Registry struct {
	mu      sync.Mutex
	entries map[string]*entry
}

type entry struct {
	cancel context.CancelCauseFunc
	target Target
}

// Target describes one killable unit to the surface. ID is what a kill
// request names: a tool call id for a call, a handle for background work
// (they coincide — a handle IS the call id that spawned it).
type Target struct {
	ID string `json:"id"`
	// Kind is "tool" for a tool call or background command, "agent" for a
	// spawned child. The surface renders a different verb for each.
	Kind string `json:"kind"`
	// Name is the tool name, or the agent name for a child.
	Name string `json:"name"`
	// Session is the tree member the work belongs to — the root session for
	// its own calls, a child session id for a descendant's.
	Session string `json:"session"`
}

// NewRegistry returns an empty registry. Hosts create one per tree.
func NewRegistry() *Registry {
	return &Registry{entries: map[string]*entry{}}
}

// Register publishes one unit's cancel and returns its unregister func,
// which is safe to call more than once. Re-registering the same id replaces
// the entry (a foreground call handing itself over to the background
// runtime keeps its id but changes which cancel reaches it); the superseded
// unregister then becomes a no-op, so the late defer of the foreground
// launch cannot unpublish the live background work.
func (r *Registry) Register(t Target, cancel context.CancelCauseFunc) func() {
	if r == nil || t.ID == "" || cancel == nil {
		return func() {}
	}
	e := &entry{cancel: cancel, target: t}
	r.mu.Lock()
	r.entries[t.ID] = e
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		if cur, ok := r.entries[t.ID]; ok && cur == e {
			delete(r.entries, t.ID)
		}
		r.mu.Unlock()
	}
}

// Kill fires the target's cancel and reports what it hit. An unknown id is
// an ordinary miss, not an error: the work may have finished microseconds
// before the click, and killing something already gone is a no-op by
// construction. The entry is left for its owner to unregister — the owner
// unwinds through its normal cancelled path, which is what journals the
// work's terminal.
func (r *Registry) Kill(id string) (Target, bool) {
	if r == nil {
		return Target{}, false
	}
	r.mu.Lock()
	e, ok := r.entries[id]
	r.mu.Unlock()
	if !ok {
		return Target{}, false
	}
	e.cancel(errs.ErrKilled)
	return e.target, true
}

// KillSession fires every unit belonging to one tree member — the "stop
// this subagent, whatever it is doing" gesture, which must also cut the
// tool call the child is parked in. Returns what it hit, id-ordered.
func (r *Registry) KillSession(session string) []Target {
	if r == nil || session == "" {
		return nil
	}
	r.mu.Lock()
	var hits []*entry
	for _, e := range r.entries {
		if e.target.Session == session {
			hits = append(hits, e)
		}
	}
	r.mu.Unlock()
	out := make([]Target, 0, len(hits))
	for _, e := range hits {
		e.cancel(errs.ErrKilled)
		out = append(out, e.target)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// List snapshots everything currently killable, id-ordered. The surface
// uses it to answer "what can I stop right now" without folding a journal.
func (r *Registry) List() []Target {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	out := make([]Target, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.target)
	}
	r.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
