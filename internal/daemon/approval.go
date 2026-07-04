package daemon

import (
	"context"
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

// Ask parks until an answer arrives or ctx ends (the run's own cancel and
// the durable-park semantics stay with the agent loop — this is only the
// cross-process rendezvous).
func (b *ApprovalBroker) Ask(ctx context.Context, session, approvalID string) (ApprovalAnswer, error) {
	ch := make(chan ApprovalAnswer, 1)
	k := key(session, approvalID)
	b.mu.Lock()
	b.pending[k] = ch
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.pending, k)
		b.mu.Unlock()
	}()
	select {
	case a := <-ch:
		return a, nil
	case <-ctx.Done():
		return ApprovalAnswer{}, ctx.Err()
	}
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
