package daemon

import (
	"context"
	"fmt"
	"sync"
)

// ApprovalAnswer is a human's verdict routed over the socket.
type ApprovalAnswer struct {
	Approve bool
	Reason  string
}

// ApprovalBroker parks daemon-hosted asks until an `approve` command answers
// them (S6 模块④, S5 回访: 审批沿 correlation 跨进程路由 — a child's ask
// shares the ROOT session's resolver, so it parks here keyed by the hosted
// session and surfaces on the attach stream; the answering client addresses
// it by (session, approval_id)).
type ApprovalBroker struct {
	mu      sync.Mutex
	pending map[string]chan ApprovalAnswer
}

func NewApprovalBroker() *ApprovalBroker {
	return &ApprovalBroker{pending: map[string]chan ApprovalAnswer{}}
}

func key(session, approvalID string) string { return session + "/" + approvalID }

// Register parks a NEW ask and returns the id the answer must address —
// unique even when deterministic call ids collide across concurrently-asking
// siblings (S6 review: two children asking at turn 1/index 0 share
// apr-eff-tool-call_1_0): a taken id gets a #<n> suffix, and the CALLER must
// surface the returned id, not the original. Pair with Wait.
func (b *ApprovalBroker) Register(session, approvalID string) (string, <-chan ApprovalAnswer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := approvalID
	for i := 2; ; i++ {
		if _, taken := b.pending[key(session, id)]; !taken {
			break
		}
		id = fmt.Sprintf("%s#%d", approvalID, i)
	}
	ch := make(chan ApprovalAnswer, 1)
	b.pending[key(session, id)] = ch
	return id, ch
}

// Wait blocks until the registered ask is answered or ctx ends, then removes
// the registration (its own only — the id is unique per Register).
func (b *ApprovalBroker) Wait(ctx context.Context, session, id string, ch <-chan ApprovalAnswer) (ApprovalAnswer, error) {
	defer func() {
		b.mu.Lock()
		delete(b.pending, key(session, id))
		b.mu.Unlock()
	}()
	select {
	case a := <-ch:
		return a, nil
	case <-ctx.Done():
		return ApprovalAnswer{}, ctx.Err()
	}
}

// Ask is Register + Wait for callers that control their own id uniqueness.
func (b *ApprovalBroker) Ask(ctx context.Context, session, approvalID string) (ApprovalAnswer, error) {
	id, ch := b.Register(session, approvalID)
	return b.Wait(ctx, session, id, ch)
}

// Answer resolves a parked ask; false when nothing is waiting under that key
// (wrong id, or the ask already resolved).
func (b *ApprovalBroker) Answer(session, approvalID string, a ApprovalAnswer) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch, ok := b.pending[key(session, approvalID)]
	if !ok {
		return false
	}
	select {
	case ch <- a:
		return true
	default:
		return false // already answered; buffered-1 channel is full
	}
}
