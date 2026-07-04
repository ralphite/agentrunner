package agent

import (
	"encoding/json"

	"github.com/ralphite/agentrunner/internal/tool"
)

// runBlackboardTool executes publish_note / read_notes against the
// tree-shared board (S5.4). The publisher identity is the spec name — the
// collaboration-meaningful identity, stable across attempts. Every failure
// here is model-visible; the harness never fails on a blackboard call.
func (l *Loop) runBlackboardTool(name string, rawArgs json.RawMessage) tool.Result {
	if l.Board == nil {
		return errorResult(name + ": no blackboard available in this run")
	}
	switch name {
	case "publish_note":
		var args struct {
			Topic string `json:"topic"`
			Text  string `json:"text"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || args.Topic == "" || args.Text == "" {
			return errorResult("publish_note: invalid args: need {\"topic\", \"text\"}")
		}
		note := l.Board.Publish(args.Topic, l.Spec.Name, args.Text)
		payload, _ := json.Marshal(map[string]any{
			"output": "published", "topic": note.Topic, "seq": note.Seq,
		})
		return tool.Result{Payload: payload}

	case "read_notes":
		var args struct {
			Topic string `json:"topic"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil || args.Topic == "" {
			return errorResult("read_notes: invalid args: need {\"topic\"}")
		}
		payload, _ := json.Marshal(map[string]any{
			"topic": args.Topic, "notes": l.Board.Read(args.Topic),
		})
		return tool.Result{Payload: payload}

	default:
		return errorResult("unknown blackboard tool " + name)
	}
}
