package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/store"
)

// ReplayJournal renders a session's journal as the output-event stream a
// live watcher would have seen (attach 补读). The journal is the durable
// truth; this is a pure projection of it — ephemeral kinds that never reach
// the journal (text deltas, discarded partial streams) are simply absent.
func ReplayJournal(sessionDir string, sink protocol.Sink) error {
	events, err := store.ReadEvents(sessionDir)
	if err != nil {
		return err
	}
	turn := 0
	// Tool-call metadata for pairing results back to their names.
	toolByActivity := map[string]event.ActivityStarted{}
	// Tool-call metadata by call id, so an approval_request (which only
	// carries the call id) can name the tool + args it is gating (UX-02).
	type callMeta struct{ tool, args string }
	callByID := map[string]callMeta{}
	for _, env := range events {
		decoded, err := event.DecodePayload(env)
		if err != nil {
			return err
		}
		switch p := decoded.(type) {
		case *event.SessionStarted:
			sink.Emit(protocol.Event{Kind: protocol.KindSessionStart})
		case *event.InputReceived:
			// The user's half of the conversation. Without this, replay shows
			// only assistant/tool output and a rejoining watcher can't see what
			// was asked (QA Wave1 cli-life-02). Prefer the Text projection; fall
			// back to the typed Content parts for journals that only carry those.
			text := p.Text
			if text == "" {
				text = textFromParts(p.Content)
			}
			// Always surface attachments — a message with both a caption and an
			// image otherwise hid the attachment entirely (QA Wave3 frank-01).
			if n := len(p.Images) + len(p.Files); n > 0 {
				tag := fmt.Sprintf("[+%d image(s), %d file(s)]", len(p.Images), len(p.Files))
				if text == "" {
					text = tag
				} else {
					text += " " + tag
				}
			}
			// Tag non-user (machine/hook) inputs by source; the renderer uses
			// the Tool field as that tag.
			src := ""
			switch p.Source {
			case "", "cli", "user", "local-user":
			default:
				src = p.Source
			}
			sink.Emit(protocol.Event{Kind: protocol.KindUserInput, N: turn, Text: text, Tool: src})
		case *event.GenerationStarted:
			turn = p.GenStep
			sink.Emit(protocol.Event{Kind: protocol.KindGenerationStart, N: p.GenStep})
		case *event.AssistantMessage:
			for _, part := range p.Message.Parts {
				switch part.Kind {
				case provider.PartText:
					if part.Text != "" {
						sink.Emit(protocol.Event{Kind: protocol.KindMessage, N: p.GenStep, Text: part.Text})
					}
				case provider.PartToolCall:
					args := compactJSON(part.Args)
					callByID[part.CallID] = callMeta{tool: part.ToolName, args: args}
					sink.Emit(protocol.Event{Kind: protocol.KindToolCall, N: p.GenStep,
						Tool: part.ToolName, CallID: part.CallID, Args: args})
				}
			}
		case *event.WaitingEntered:
			// The standby idle is part of the story (决策 #31): attach
			// replay renders it exactly like the live stream, so followers
			// keyed on idle (INC-2) see the same shape either way.
			if p.Kind == event.WaitInput {
				sink.Emit(protocol.Event{Kind: protocol.KindIdle, N: turn})
			}
		case *event.ActivityStarted:
			if p.Kind == event.KindTool {
				toolByActivity[p.ActivityID] = *p
			}
		case *event.ActivityCompleted:
			if started, ok := toolByActivity[p.ActivityID]; ok {
				sink.Emit(protocol.Event{Kind: protocol.KindToolResult, N: turn,
					Tool: started.Name, CallID: started.CallID,
					Result: compactJSON(p.Result), IsError: p.IsError})
			}
		case *event.ActivityFailed:
			if started, ok := toolByActivity[p.ActivityID]; ok && p.Final {
				sink.Emit(protocol.Event{Kind: protocol.KindToolResult, N: turn,
					Tool: started.Name, CallID: started.CallID,
					Result: p.Error.Message, IsError: true})
			} else if p.Final {
				// A final NON-tool failure (e.g. the LLM call after retries
				// exhaust) otherwise replays as silence — attach shows only
				// generation_start and a reconnecting watcher never learns the
				// turn failed (QA Wave1 carol-04, Wave2 carol-07/grace-03).
				// Surface it as an error event, mirroring the live stream.
				msg := p.Error.Message
				if p.Error.Class != "" {
					msg = string(p.Error.Class) + ": " + msg
				}
				sink.Emit(protocol.Event{Kind: protocol.KindError, N: turn, Text: msg})
			}
		case *event.ActivityCancelled:
			if started, ok := toolByActivity[p.ActivityID]; ok {
				sink.Emit(protocol.Event{Kind: protocol.KindToolResult, N: turn,
					Tool: started.Name, CallID: started.CallID,
					Result: "canceled", IsError: true})
			}
		case *event.ModeChanged:
			sink.Emit(protocol.Event{Kind: protocol.KindModeChanged, Mode: p.To})
		case *event.ApprovalRequested:
			meta := callByID[p.CallID]
			sink.Emit(protocol.Event{Kind: protocol.KindApprovalRequest, N: turn,
				CallID: p.CallID, ApprovalID: p.ApprovalID,
				Tool: meta.tool, Args: meta.args, Text: askReason(p.GateResults)})
		case *event.LimitExceeded:
			// An interrupt is journaled as a LimitExceeded of kind "interrupted"
			// (a limit of 0), NOT a budget failure — render it exactly like the
			// live stream's "↺ interrupted by user", not the budget-exhaustion
			// template (QA Wave6 mia-01, a regression from the heidi-05 fix).
			if p.Kind == "interrupted" {
				sink.Emit(protocol.Event{Kind: protocol.KindDiscard, N: turn, Text: "interrupted by user"})
				break
			}
			// A budget-truncated turn otherwise replays as an empty
			// [gen-step N] with no explanation (QA Wave2 heidi-05 / carol-06):
			// surface the reason so a rejoining watcher understands why the
			// turn produced nothing.
			sink.Emit(protocol.Event{Kind: protocol.KindError, N: turn,
				Text: fmt.Sprintf("%s budget exhausted; turn truncated (limit %d, used %d)",
					p.Kind, p.Limit, p.Used)})
		case *event.GenerationDiscarded:
			sink.Emit(protocol.Event{Kind: protocol.KindDiscard, N: p.GenStep, Text: p.Reason})
		case *event.SpecChanged:
			sink.Emit(protocol.Event{Kind: protocol.KindMessage,
				Text: "[agent switched to " + p.SpecName + "]"})
		case *event.SessionClosed:
			sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, N: p.GenSteps, Reason: p.Reason})
		// Driver streams (S6): iteration terminals and the series ending
		// project the same way the live tee emits them.
		case *event.IterationCompleted:
			sink.Emit(protocol.Event{Kind: protocol.KindIteration, N: p.Iter, Reason: p.ChildReason,
				Text: fmt.Sprintf("iteration %d %s (pass=%v score=%g)",
					p.Iter, p.ChildReason, p.Verdict.Pass, p.Verdict.Score)})
		case *event.DriverCompleted:
			sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, N: p.Iterations, Reason: p.Reason})
		}
	}
	return nil
}

// textFromParts joins the text parts of a typed content slice (used when an
// InputReceived carries Content but no flattened Text projection).
func textFromParts(parts []provider.Part) string {
	var b []string
	for _, part := range parts {
		if part.Kind == provider.PartText && part.Text != "" {
			b = append(b, part.Text)
		}
	}
	return strings.Join(b, "\n")
}

// askReason picks the human-facing reason to show with a pending approval:
// the gate that returned "ask" (falling back to any non-allow gate that
// carries a reason). Empty when no gate offered one.
func askReason(gates []event.GateResult) string {
	fallback := ""
	for _, g := range gates {
		if g.Decision == event.VerdictAsk && g.Reason != "" {
			return g.Reason
		}
		if g.Decision != event.VerdictAllow && g.Reason != "" && fallback == "" {
			fallback = g.Reason
		}
	}
	return fallback
}

// compactJSON renders raw JSON on one line (mirrors the live emit path).
func compactJSON(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}
