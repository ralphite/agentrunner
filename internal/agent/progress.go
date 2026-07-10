package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/tool"
)

// The progress face (INC-37, HANDA #9): progress_update is a loop-handled
// internal tool like goal_status/goal_complete — it journals a fold-owned
// projection and never touches the workspace, so it bypasses the effect
// pipeline. Limits keep a runaway model from bloating the journal; statuses
// normalize to a closed enum so the fold and every consumer read one shape.
const (
	progressMaxItems = 50
	progressMaxID    = 64
	progressMaxTitle = 200
)

// progressStatusAliases maps common model phrasings onto the canonical
// pending|running|done|failed enum. Unknown statuses are a model-visible
// arg error, not a silent guess.
var progressStatusAliases = map[string]string{
	"pending": "pending", "todo": "pending", "open": "pending", "waiting": "pending",
	"running": "running", "in_progress": "running", "in-progress": "running",
	"active": "running", "working": "running", "started": "running",
	"done": "done", "completed": "done", "complete": "done", "finished": "done",
	"failed": "failed", "error": "failed", "failure": "failed", "cancelled": "failed",
}

// runProgressTool validates and normalizes the wholesale checklist, journals
// ProgressUpdated, and answers with a count — never the table itself (the
// model already knows what it sent; echoing it back only burns context).
func runProgressTool(args json.RawMessage, appendE AppendFunc) tool.Result {
	errRes := func(msg string) tool.Result {
		p, _ := json.Marshal(map[string]string{"error": msg})
		return tool.Result{Payload: p, IsError: true}
	}
	var in struct {
		Items []event.ProgressItem `json:"items"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return errRes(`progress_update needs {"items":[{"id","title","status"}]}`)
	}
	if len(in.Items) > progressMaxItems {
		return errRes(fmt.Sprintf("progress_update accepts at most %d items (got %d)", progressMaxItems, len(in.Items)))
	}
	r := redact.FromEnv()
	items := make([]event.ProgressItem, 0, len(in.Items))
	seen := make(map[string]bool, len(in.Items))
	for i, it := range in.Items {
		id := strings.TrimSpace(it.ID)
		title := strings.TrimSpace(it.Title)
		if id == "" || title == "" {
			return errRes(fmt.Sprintf("item %d needs a non-empty id and title", i))
		}
		if seen[id] {
			return errRes(fmt.Sprintf("duplicate item id %q", id))
		}
		seen[id] = true
		status, ok := progressStatusAliases[strings.ToLower(strings.TrimSpace(it.Status))]
		if !ok {
			return errRes(fmt.Sprintf("item %q has unknown status %q (use pending|running|done|failed)", id, it.Status))
		}
		if len(id) > progressMaxID {
			id = id[:progressMaxID]
		}
		if len(title) > progressMaxTitle {
			title = title[:progressMaxTitle]
		}
		items = append(items, event.ProgressItem{ID: id, Title: r.String(title), Status: status})
	}
	if _, err := appendE(event.TypeProgressUpdated, &event.ProgressUpdated{Items: items}); err != nil {
		return errRes(fmt.Sprintf("recording progress failed: %v", err))
	}
	p, _ := json.Marshal(map[string]any{"output": "progress updated", "items": len(items)})
	return tool.Result{Payload: p}
}
