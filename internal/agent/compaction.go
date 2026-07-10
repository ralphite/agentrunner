package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/memory"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/tool"
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

// microcompactRecentGuard keeps this many trailing messages out of any
// micro boundary: the model's working set stays verbatim, and each trigger
// moves the boundary well behind the frontier so the assembled prefix stays
// byte-stable between triggers (prompt-cache friendly).
const microcompactRecentGuard = 8

// microcompactMinResultBytes: results at or below this size are not worth
// replacing — the placeholder itself costs ~70 bytes.
const microcompactMinResultBytes = 200

// microcompactPlaceholder is what the model sees in place of a cleared
// result. It names the escape hatch: the tool can simply be re-run.
const microcompactPlaceholder = "[old tool result cleared to save context — re-run the tool if needed]"

// microcompactPlaceholderJSON is the placeholder as a tool-result body
// (results are raw JSON; a JSON string keeps every provider adapter happy).
var microcompactPlaceholderJSON = json.RawMessage(strconv.Quote(microcompactPlaceholder))

// microcompactAt resolves the trigger threshold (INC-13): explicit value
// wins, -1 disables, zero defaults to 3/4 of CompactAtTokens so micro fires
// before the LLM summary would.
func microcompactAt(spec *AgentSpec) int {
	switch v := spec.Model.MicrocompactAtTokens; {
	case v > 0:
		return v
	case v < 0:
		return 0
	default:
		return spec.Model.CompactAtTokens * 3 / 4
	}
}

// microcompactEligible reports whether one tool call's result would render
// as a placeholder behind a micro boundary. Shared by the trigger scan and
// assembly so both always agree: read-class (re-runnable) tools only, real
// results only (errors are short and are the model's feedback), and only
// results big enough to be worth clearing.
func microcompactEligible(s state.State, c provider.ToolCall) bool {
	res, ok := s.Conversation.ToolResults[c.CallID]
	if !ok || res.IsError {
		return false
	}
	if len(res.Result) <= microcompactMinResultBytes {
		return false
	}
	return toolClassIn(s, c.Name) == string(tool.ClassRead)
}

// microcompactDue reports whether advancing the micro boundary now would
// actually reclaim context (INC-13). Pure — fold plus spec — so resume
// reaches the same verdict. Requires the estimate over the threshold AND at
// least one eligible result between the current boundary and the candidate
// one; otherwise a read-free conversation would journal a no-op event at
// every safe boundary.
func microcompactDue(s state.State, spec *AgentSpec) bool {
	limit := microcompactAt(spec)
	if limit <= 0 {
		return false
	}
	newB := len(s.Conversation.Messages) - microcompactRecentGuard
	if newB <= s.Compaction.MicroBoundary {
		return false
	}
	if estimateContextTokens(s) <= limit {
		return false
	}
	return microcompactClears(s, newB) > 0
}

// microcompactClears counts the eligible results that a boundary at newB
// would clear beyond the current one (informational for the event, and the
// no-op guard for the trigger).
func microcompactClears(s state.State, newB int) int {
	msgs := s.Conversation.Messages
	from := s.Compaction.MicroBoundary
	if b := s.Compaction.Boundary; b > from {
		from = b // messages before the compaction boundary never assemble
	}
	if newB > len(msgs) {
		newB = len(msgs)
	}
	n := 0
	for i := from; i < newB; i++ {
		if msgs[i].Role != provider.RoleAssistant {
			continue
		}
		for _, c := range toolCallsOf(msgs[i]) {
			if microcompactEligible(s, c) {
				n++
			}
		}
	}
	return n
}

// microcompact journals the boundary advance (INC-13). No LLM, no activity —
// the appender folds the event into ds.s, so the caller falls through to
// compactionDue with the already-shrunken estimate (micro softens or avoids
// the summary), mirroring compaction's self-terminating shape.
func (l *Loop) microcompact(ds *driveState, appendE AppendFunc) error {
	newB := len(ds.s.Conversation.Messages) - microcompactRecentGuard
	cleared := microcompactClears(ds.s, newB)
	if _, err := appendE(event.TypeContextMicrocompacted, &event.ContextMicrocompacted{
		Boundary:        newB,
		EstimatedTokens: estimateContextTokens(ds.s),
		Cleared:         cleared,
	}); err != nil {
		return err
	}
	l.emit(protocol.Event{Kind: protocol.KindMessage,
		Text: fmt.Sprintf("context microcompacted: %d old tool result(s) cleared", cleared)})
	return nil
}

// compactContext runs the summarizer as a recorded LLM activity and journals
// ContextCompacted (S4.5 / DESIGN §context-assembly: compaction is itself a
// nondeterministic LLM call whose event changes later fold results). It is
// idempotent — re-summarizing on resume is safe — and does NOT pass the
// permission pipeline: it is a harness-internal maintenance call the model
// never directed. Its token usage still settles into the budget.
// It reports whether a compaction actually landed: a pre_compact hook veto
// or an empty summary returns (false, nil), and the AUTO caller must then
// proceed WITHOUT `continue` — retrying the same due-check in a loop with a
// standing veto would spin forever.
func (l *Loop) compactContext(ctx context.Context, ds *driveState, appendE AppendFunc,
	exec *ActivityExecutor, turn int, directive string, manual bool) (bool, error) {

	// PreCompact lifecycle hook (INC-15, G19): blockable — exit 2 skips this
	// compaction (auto or manual) and the context stays as is.
	trigger := "auto"
	if manual {
		trigger = "manual"
	}
	if res := l.fireLifecycle(ctx, hook.EventPreCompact,
		map[string]string{"trigger": trigger, "directive": directive}, true); res.Blocked {
		l.emit(protocol.Event{Kind: protocol.KindMessage,
			Text: "compact skipped by pre_compact hook: " + res.Reason})
		return false, nil
	}

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
	// The summarizer call needs blob bytes exactly like a turn's own call:
	// an image/file part left as a bare CAS ref makes the provider refuse
	// the whole request and the compact silently fails (QA Round1 F-A03).
	if err := l.inflateBlobs(msgs); err != nil {
		return err
	}
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
		return false, err
	}
	// Never journal a context-LOSING compaction: an empty summary would drop
	// the whole prefix (assembly reads only msgs[Boundary:] with no summary).
	// If the summarizer produced nothing, keep the context and skip — the user
	// can retry, and no history is silently lost.
	if strings.TrimSpace(summary) == "" {
		l.emit(protocol.Event{Kind: protocol.KindMessage,
			Text: "compact skipped: summarizer produced no summary; context unchanged"})
		return false, nil
	}

	// DroppedTurns is informational: turns wholly behind the new boundary.
	dropped := turn - 1 - ds.s.Compaction.UptoGenStep
	if dropped < 0 {
		dropped = 0
	}
	if _, err := appendE(event.TypeContextCompacted, &event.ContextCompacted{
		UptoGenStep: turn - 1, Summary: summary, DroppedTurns: dropped,
	}); err != nil {
		return false, err
	}
	l.emit(protocol.Event{Kind: protocol.KindDiscard, N: turn,
		Text: "context compacted"})
	l.fireLifecycle(ctx, hook.EventPostCompact,
		map[string]any{"trigger": trigger, "dropped_turns": dropped}, false)
	return true, nil
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
		ctlAppend := appendE
		if ctl.CommandID != "" {
			ctlAppend = l.commandAppender(ds, ctl.CommandID)
		}
		before := ds.lastID
		switch ctl.Kind {
		case protocol.ControlClose:
			copy := ctl
			ds.closeRequested = &copy
			continue
		case protocol.ControlGoalAttach, protocol.ControlGoalPause, protocol.ControlGoalResume,
			protocol.ControlGoalUpdate, protocol.ControlGoalCancel:
			// Goal controls (INC-D1) apply regardless of conversation size — a
			// goal can be attached to a fresh or freshly-cleared session.
			if err := l.applyGoalControl(ds, ctlAppend, ctl); err != nil {
				return err
			}
		case protocol.ControlClear, protocol.ControlCompact:
			// Nothing new since the last boundary → the op is a no-op (avoids a
			// degenerate empty compaction on an idle or freshly-cleared session).
			if len(ds.s.Conversation.Messages) <= ds.s.Compaction.Boundary {
				break
			}
			upto := ds.s.Session.GenStep
			if ctl.Kind == protocol.ControlClear {
				if err := l.clearContext(ds, ctlAppend, upto); err != nil {
					return err
				}
			} else if _, err := l.compactContext(ctx, ds, ctlAppend, exec, upto+1, ctl.Directive, true); err != nil {
				return err
			}
		case protocol.ControlRemember:
			if err := l.remember(ds, ctlAppend, ctl.Directive); err != nil {
				return err
			}
		}
		if ctl.CommandID != "" && ds.lastID == before {
			if _, err := ctlAppend(event.TypeCommandHandled, &event.CommandHandled{
				CommandID: ctl.CommandID, CommandSeq: ctl.CommandSeq,
				Kind: ctl.Kind, Result: "no_op",
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// remember writes a note to the workspace-root CLAUDE.md (G9, INC-14, INC-D4
// 取 A) and surfaces it as a program-source input so the CURRENT conversation
// honors it too. The file persists for the NEXT session's frozen prefix; the
// appended message is how "this run" sees it without rewriting the frozen
// prefix (DESIGN §4: environment changes enter as appended messages, never as
// a prefix rewrite). memory.Append is idempotent, so a replayed durable
// remember command never double-writes the file.
func (l *Loop) remember(ds *driveState, appendE AppendFunc, note string) error {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	var wsRoot string
	if l.Exec != nil && l.Exec.WS != nil {
		wsRoot = l.Exec.WS.Root()
	}
	if wsRoot == "" {
		return fmt.Errorf("remember: no workspace root")
	}
	if err := memory.Append(wsRoot, note); err != nil {
		return err
	}
	_, err := appendE(event.TypeInputReceived, &event.InputReceived{
		Text:   "[记忆] 已记入项目 CLAUDE.md（下次会话进入 prompt 前缀，本会话起即遵循）：" + note,
		Source: "program",
	})
	return err
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
