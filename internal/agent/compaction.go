package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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
	exec *ActivityExecutor, turn int, directive string, manual bool) error {

	system := compactionSystemPrompt
	if d := strings.TrimSpace(directive); d != "" {
		system += "\n\nThe user asked you to focus this summary on: " + d
	}
	// Terminate the summarizer request with a USER turn. Auto-compaction fires
	// right after a user message (the request already ends with user), but a
	// manual compact at idle summarizes a conversation ending in an ASSISTANT
	// message — several providers (Gemini among them) return an EMPTY reply
	// when asked to continue after their own turn. An explicit user prompt
	// makes the request well-formed regardless of where compaction is invoked.
	msgs := append(assembleMessages(ds.s), provider.Message{
		Role:  provider.RoleUser,
		Parts: []provider.Part{{Kind: provider.PartText, Text: "Now produce the summary as instructed."}},
	})
	req := provider.CompleteRequest{
		Model:     l.Spec.Model.ID,
		MaxTokens: l.Spec.Model.MaxTokens,
		System:    system,
		Messages:  msgs,
		GenStep:   turn,
	}
	// A manual compact uses a distinct activity-id namespace so it can never
	// collide with (and dedup against) an auto-compaction at the same turn.
	actID := compactActivityID(turn)
	if manual {
		actID = "compact-manual-t" + strconv.Itoa(turn)
	}
	var summary string
	err := exec.Do(ctx, Activity{
		ID: actID, Kind: event.KindLLM,
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
	// Never journal a context-LOSING compaction: an empty summary would drop
	// the whole prefix (assembly reads only msgs[Boundary:] with no summary).
	// If the summarizer produced nothing, keep the context and skip — the user
	// can retry, and no history is silently lost.
	if strings.TrimSpace(summary) == "" {
		l.emit(protocol.Event{Kind: protocol.KindMessage,
			Text: "compact skipped: summarizer produced no summary; context unchanged"})
		return nil
	}

	// DroppedTurns is informational: turns wholly behind the new boundary.
	dropped := turn - 1 - ds.s.Compaction.UptoGenStep
	if dropped < 0 {
		dropped = 0
	}
	if _, err := appendE(event.TypeContextCompacted, &event.ContextCompacted{
		UptoGenStep: turn - 1, Summary: summary, DroppedTurns: dropped,
	}); err != nil {
		return err
	}
	l.emit(protocol.Event{Kind: protocol.KindDiscard, N: turn,
		Text: "context compacted"})
	return nil
}

func compactActivityID(turn int) string {
	return fmt.Sprintf("compact-t%d", turn)
}

// drainControls applies any queued session-maintenance controls (G7) at a
// safe boundary. Both paths funnel here: the busy path reads fresh controls
// off l.Controls; the idle path stored them on ds.pendingControls (awaitInput
// can't run the summarizer itself). Each control is applied against the
// current fold — appendE folds each ContextCompacted straight into ds.s, so a
// second control in the same batch sees the advanced boundary and no-ops.
func (l *Loop) drainControls(ctx context.Context, ds *driveState, appendE AppendFunc, exec *ActivityExecutor) error {
	for l.Controls != nil {
		select {
		case ctl := <-l.Controls:
			ds.pendingControls = append(ds.pendingControls, ctl)
			continue
		default:
		}
		break
	}
	pending := ds.pendingControls
	ds.pendingControls = nil
	for _, ctl := range pending {
		switch ctl.Kind {
		case protocol.ControlGoalAttach, protocol.ControlGoalPause, protocol.ControlGoalResume,
			protocol.ControlGoalUpdate, protocol.ControlGoalCancel:
			// Goal controls (INC-D1) apply regardless of conversation size — a
			// goal can be attached to a fresh or freshly-cleared session.
			if err := l.applyGoalControl(ds, appendE, ctl); err != nil {
				return err
			}
		case protocol.ControlClear, protocol.ControlCompact:
			// Nothing new since the last boundary → the op is a no-op (avoids a
			// degenerate empty compaction on an idle or freshly-cleared session).
			if len(ds.s.Conversation.Messages) <= ds.s.Compaction.Boundary {
				continue
			}
			upto := ds.s.Session.GenStep
			if ctl.Kind == protocol.ControlClear {
				if err := l.clearContext(ds, appendE, upto); err != nil {
					return err
				}
			} else if err := l.compactContext(ctx, ds, appendE, exec, upto+1, ctl.Directive, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// clearContext drops the whole context prefix (G7 /clear) with NO summarizer
// call: it advances the boundary to the current message count and leaves an
// EMPTY summary, so assembly shows only the messages after the boundary. The
// full log stays intact in the journal (truth); only the assembled view resets.
func (l *Loop) clearContext(ds *driveState, appendE AppendFunc, upto int) error {
	dropped := upto - ds.s.Compaction.UptoGenStep
	if dropped < 0 {
		dropped = 0
	}
	if _, err := appendE(event.TypeContextCompacted, &event.ContextCompacted{
		UptoGenStep: upto, Summary: "", Cleared: true, DroppedTurns: dropped,
	}); err != nil {
		return err
	}
	l.emit(protocol.Event{Kind: protocol.KindDiscard, N: upto, Text: "context cleared"})
	return nil
}
