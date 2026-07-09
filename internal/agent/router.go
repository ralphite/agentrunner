// TreeRouter is the session tree's message fabric (INC-12, DESIGN §3
// 树内消息): any member may post an input to any other member's DURABLE
// per-session inbox (the same store.AppendInbox mailbox the daemon uses —
// fsync-before-ack, command-id idempotency, monotonic delivery seq), with a
// best-effort live wake so a running target consumes it at its next safe
// point. The router is tree-shared like the blackboard; the HOST process of
// the tree root is the single writer of every member's inbox file, so the
// mailbox single-writer discipline holds tree-wide.
package agent

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/store"
)

// TreeRouter routes tree-internal messages. Registration is ephemeral
// process wiring (who is live right now); the durable truth is always the
// target's inbox file. A message to an unregistered (quiescent) member
// lands durably and triggers a revive request to the member's parent.
type TreeRouter struct {
	root    string // tree root session id
	rootDir string // tree root session dir

	mu    sync.Mutex
	ports map[string]chan<- protocol.UserInput // live member wake channels
	// revives carries "this child has undelivered mail" requests to the
	// parent that owns the child's re-host (INC-12.2). Keyed by parent id.
	revives map[string]chan string
}

// NewTreeRouter builds the router for one session tree.
func NewTreeRouter(rootSession, rootDir string) *TreeRouter {
	return &TreeRouter{
		root: rootSession, rootDir: rootDir,
		ports:   map[string]chan<- protocol.UserInput{},
		revives: map[string]chan string{},
	}
}

// Register wires a live member's wake channel (and its revive listening
// post). Called by each loop as it starts driving; Deregister on the way
// out.
func (r *TreeRouter) Register(sid string, port chan<- protocol.UserInput, revive chan string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ports[sid] = port
	if revive != nil {
		r.revives[sid] = revive
	}
}

// Deregister removes a member's live wiring. Its inbox file stays — mail
// keeps accumulating durably while it is quiescent.
func (r *TreeRouter) Deregister(sid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.ports, sid)
	delete(r.revives, sid)
}

// InTree reports whether sid names the root or a descendant.
func (r *TreeRouter) InTree(sid string) bool {
	return sid == r.root || strings.HasPrefix(sid, r.root+"-sub-")
}

// DirOf maps a tree member's session id to its journal/inbox directory:
// every "-sub-" segment is a "/sub/" path step under the root (INC-1
// addressing, the write-side twin of the CLI's read-side mapping).
func (r *TreeRouter) DirOf(sid string) (string, error) {
	if sid == r.root {
		return r.rootDir, nil
	}
	if !r.InTree(sid) {
		return "", fmt.Errorf("%s is not in this session tree", sid)
	}
	rest := strings.TrimPrefix(sid, r.root+"-sub-")
	dir := filepath.Join(r.rootDir, "sub", strings.ReplaceAll(rest, "-sub-", "/sub/"))
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("no such team member %s (never spawned?)", sid)
	}
	return dir, nil
}

// ParentOf returns the parent session id of a tree member ("" for the root).
func ParentOf(sid string) string {
	idx := strings.LastIndex(sid, "-sub-")
	if idx < 0 {
		return ""
	}
	return sid[:idx]
}

// Send appends one message durably to the target's inbox and wakes it (or,
// when the target is quiescent, requests a revive from its parent —
// INC-12.2). Returns the assigned delivery seq. The caller mints a stable
// CommandID so a re-run of the same send is idempotent.
func (r *TreeRouter) Send(to string, in protocol.UserInput) (int64, error) {
	dir, err := r.DirOf(to)
	if err != nil {
		return 0, err
	}
	accepted, err := store.AppendInbox(dir, in)
	if err != nil {
		return 0, fmt.Errorf("deliver to %s: %w", to, err)
	}

	r.mu.Lock()
	port, live := r.ports[to]
	var revive chan string
	if !live {
		// Walk up until a live ancestor is found: it owns (directly or by
		// resuming the intermediate chain) the quiescent target's re-host.
		for p := ParentOf(to); p != ""; p = ParentOf(p) {
			if ch, ok := r.revives[p]; ok {
				revive = ch
				break
			}
		}
	}
	r.mu.Unlock()

	switch {
	case live:
		select {
		case port <- accepted:
		default:
			// The wake channel is full — the mail is durable; the member
			// reconciles its inbox tail at the next resume/revive.
			slog.Warn("tree message wake dropped (channel full); durable in inbox",
				"to", to, "seq", accepted.DeliverySeq)
		}
	case revive != nil:
		select {
		case revive <- to:
		default:
			// Revive queue full: the pending-mail scan at the ancestor's next
			// safe point picks it up (same durable truth).
			slog.Warn("revive request dropped (channel full); durable in inbox", "to", to)
		}
	default:
		slog.Warn("tree message delivered to quiescent member with no live ancestor; "+
			"it will be consumed on the member's next resume", "to", to)
	}
	return accepted.DeliverySeq, nil
}

// PendingMail reports whether a member's inbox holds entries past the given
// consumed high-water mark — the durable "has undelivered mail" test used by
// the revive scan (INC-12.2) and the quiescence race close-out.
func (r *TreeRouter) PendingMail(sid string, consumed int64) bool {
	dir, err := r.DirOf(sid)
	if err != nil {
		return false
	}
	tail, err := store.ReadInbox(dir, consumed)
	if err != nil {
		return false
	}
	return len(tail) > 0
}
