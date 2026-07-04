// Package notify is the S6 模块⑤ notifier: lifecycle moments (run endings,
// pending approvals) become user notifications through a USER-configured
// shell command — the channel is a documented carve-out, project config
// never touches it — with a stderr line as the fallback. Delivery is
// deduplicated against the notifier's OWN event stream (NotificationSent),
// so restarts and startup reconciliation never double-notify.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/store"
)

// Notification is one lifecycle moment. Key is the dedup identity — the
// SAME moment must map to the same key on every path (live tee, startup
// reconciliation).
type Notification struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Session string `json:"session,omitempty"`
	Text    string `json:"text"`
}

// commandTimeout bounds one delivery attempt; a hung channel command must
// not wedge the notifier goroutine forever.
const commandTimeout = 30 * time.Second

// Notifier delivers deduplicated notifications. Safe for concurrent Notify.
type Notifier struct {
	mu      sync.Mutex
	store   *store.EventStore
	sent    map[string]bool
	command []string
	stderr  io.Writer
}

// Open loads the notifier stream at dir and folds the already-sent set.
// command is the user-configured channel argv (empty = stderr fallback
// only); the notification JSON arrives on the command's stdin.
func Open(dir string, command []string, stderr io.Writer) (*Notifier, error) {
	es, err := store.OpenEventStore(dir)
	if err != nil {
		return nil, fmt.Errorf("notifier: %w", err)
	}
	events, err := store.ReadEvents(dir)
	if err != nil {
		_ = es.Close()
		return nil, fmt.Errorf("notifier: %w", err)
	}
	sent := map[string]bool{}
	for _, e := range events {
		if e.Type != event.TypeNotificationSent {
			continue
		}
		decoded, derr := event.DecodePayload(e)
		if derr != nil {
			continue
		}
		sent[decoded.(*event.NotificationSent).Key] = true
	}
	return &Notifier{store: es, sent: sent, command: command, stderr: stderr}, nil
}

// Close releases the stream.
func (n *Notifier) Close() error { return n.store.Close() }

// Seen reports whether a key was already notified (reconciliation reads it).
func (n *Notifier) Seen(key string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.sent[key]
}

// Notify delivers once per key: journal-before-send (a crash after the
// journal loses one notification rather than ever duplicating one — the
// journal is the dedup truth), then the command channel, stderr on failure.
func (n *Notifier) Notify(nt Notification) {
	n.mu.Lock()
	if n.sent[nt.Key] {
		n.mu.Unlock()
		return
	}
	n.sent[nt.Key] = true
	channel := "stderr"
	if len(n.command) > 0 {
		channel = "command"
	}
	env, err := event.New(event.TypeNotificationSent, &event.NotificationSent{
		Key: nt.Key, Kind: nt.Kind, Session: nt.Session, Text: nt.Text, Channel: channel,
	})
	if err == nil {
		_, err = n.store.Append(env)
	}
	n.mu.Unlock()
	if err != nil {
		fmt.Fprintf(n.stderr, "notify: journal failed (%v); delivering anyway\n", err)
	}

	if len(n.command) > 0 {
		if derr := n.deliver(nt); derr == nil {
			return
		} else {
			fmt.Fprintf(n.stderr, "notify: command failed: %v\n", derr)
		}
	}
	fmt.Fprintf(n.stderr, "notify: [%s] %s\n", nt.Kind, nt.Text)
}

// deliver runs the channel command with the notification JSON on stdin.
func (n *Notifier) deliver(nt Notification) error {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	payload, _ := json.Marshal(nt)
	cmd := exec.CommandContext(ctx, n.command[0], n.command[1:]...)
	cmd.Stdin = bytes.NewReader(append(payload, '\n'))
	cmd.Stderr = n.stderr
	return cmd.Run()
}
