package daemon

import (
	"bytes"
	"encoding/json"

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
	for _, env := range events {
		decoded, err := event.DecodePayload(env)
		if err != nil {
			return err
		}
		switch p := decoded.(type) {
		case *event.RunStarted:
			sink.Emit(protocol.Event{Kind: protocol.KindRunStart})
		case *event.TurnStarted:
			turn = p.Turn
			sink.Emit(protocol.Event{Kind: protocol.KindTurnStart, Turn: p.Turn})
		case *event.AssistantMessage:
			for _, part := range p.Message.Parts {
				switch part.Kind {
				case provider.PartText:
					if part.Text != "" {
						sink.Emit(protocol.Event{Kind: protocol.KindMessage, Turn: p.Turn, Text: part.Text})
					}
				case provider.PartToolCall:
					sink.Emit(protocol.Event{Kind: protocol.KindToolCall, Turn: p.Turn,
						Tool: part.ToolName, CallID: part.CallID, Args: compactJSON(part.Args)})
				}
			}
		case *event.ActivityStarted:
			if p.Kind == event.KindTool {
				toolByActivity[p.ActivityID] = *p
			}
		case *event.ActivityCompleted:
			if started, ok := toolByActivity[p.ActivityID]; ok {
				sink.Emit(protocol.Event{Kind: protocol.KindToolResult, Turn: turn,
					Tool: started.Name, CallID: started.CallID,
					Result: compactJSON(p.Result), IsError: p.IsError})
			}
		case *event.ActivityFailed:
			if started, ok := toolByActivity[p.ActivityID]; ok && p.Final {
				sink.Emit(protocol.Event{Kind: protocol.KindToolResult, Turn: turn,
					Tool: started.Name, CallID: started.CallID,
					Result: p.Error.Message, IsError: true})
			}
		case *event.ActivityCancelled:
			if started, ok := toolByActivity[p.ActivityID]; ok {
				sink.Emit(protocol.Event{Kind: protocol.KindToolResult, Turn: turn,
					Tool: started.Name, CallID: started.CallID,
					Result: "canceled", IsError: true})
			}
		case *event.ModeChanged:
			sink.Emit(protocol.Event{Kind: protocol.KindModeChanged, Mode: p.To})
		case *event.ApprovalRequested:
			sink.Emit(protocol.Event{Kind: protocol.KindApprovalRequest, Turn: turn,
				CallID: p.CallID})
		case *event.TurnDiscarded:
			sink.Emit(protocol.Event{Kind: protocol.KindDiscard, Turn: p.Turn, Text: p.Reason})
		case *event.RunEnded:
			sink.Emit(protocol.Event{Kind: protocol.KindRunEnd, Turn: p.Turns, Reason: p.Reason})
		}
	}
	return nil
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
