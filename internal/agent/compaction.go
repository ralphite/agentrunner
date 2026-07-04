package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
)

// compactionSystemPrompt instructs the summarizer. It is harness-owned, not
// the agent's own prompt: compaction is a maintenance call, not a turn.
const compactionSystemPrompt = "You are a context summarizer. Produce a concise but complete " +
	"summary of the conversation so far, preserving key facts, decisions, tool results, and any " +
	"open threads the assistant must still act on. Write only the summary."

// estimateContextTokens is a coarse token estimate of the assembled view
// (~4 bytes/token). It is deliberately provider-agnostic and cheap — the
// compaction trigger only needs an order-of-magnitude signal, and basing it
// on the ASSEMBLED (already-compacted) view means a fresh summary shrinks
// the estimate below the threshold, so compaction self-terminates.
func estimateContextTokens(s state.State) int {
	bytes := 0
	for _, m := range assembleMessages(s) {
		for _, p := range m.Parts {
			bytes += len(p.Text) + len(p.Args) + len(p.Result)
		}
	}
	return bytes / 4
}

// compactionDue reports whether the loop should compact before the next turn
// (S4.5). It is pure — decided from the fold plus spec — so resume reaches
// the same verdict. The boundary guard ensures each compaction consumes at
// least two new messages, so a run cannot churn compaction turn after turn.
func compactionDue(s state.State, spec *AgentSpec) bool {
	limit := spec.Model.CompactAtTokens
	if limit <= 0 {
		return false
	}
	if len(s.Conversation.Messages) <= s.Compaction.Boundary+1 {
		return false
	}
	return estimateContextTokens(s) > limit
}

// compactContext runs the summarizer as a recorded LLM activity and journals
// ContextCompacted (S4.5 / DESIGN §context-assembly: compaction is itself a
// nondeterministic LLM call whose event changes later fold results). It is
// idempotent — re-summarizing on resume is safe — and does NOT pass the
// permission pipeline: it is a harness-internal maintenance call the model
// never directed. Its token usage still settles into the budget.
func (l *Loop) compactContext(ctx context.Context, ds *driveState, appendE AppendFunc,
	exec *ActivityExecutor, turn int) error {

	req := provider.CompleteRequest{
		Model:     l.Spec.Model.ID,
		MaxTokens: l.Spec.Model.MaxTokens,
		System:    compactionSystemPrompt,
		Messages:  assembleMessages(ds.s),
		Turn:      turn,
	}
	var summary string
	err := exec.Do(ctx, Activity{
		ID: compactActivityID(turn), Kind: event.KindLLM,
		Name: "compact", Idempotent: true,
		Run: func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			collected, cerr := provider.CollectTurn(l.Provider.Complete(ctx, req))
			if cerr != nil {
				return nil, nil, false, cerr
			}
			summary = assistantText(collected.Message)
			u := collected.Usage
			return nil, &u, false, nil
		},
	})
	if err != nil {
		return err
	}

	// DroppedTurns is informational: turns wholly behind the new boundary.
	dropped := turn - 1 - ds.s.Compaction.UptoTurn
	if dropped < 0 {
		dropped = 0
	}
	if _, err := appendE(event.TypeContextCompacted, &event.ContextCompacted{
		UptoTurn: turn - 1, Summary: summary, DroppedTurns: dropped,
	}); err != nil {
		return err
	}
	l.emit(protocol.Event{Kind: protocol.KindDiscard, Turn: turn,
		Text: "context compacted"})
	return nil
}

func compactActivityID(turn int) string {
	return fmt.Sprintf("compact-t%d", turn)
}
