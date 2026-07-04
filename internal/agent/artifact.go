package agent

import (
	"context"
	"encoding/json"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
)

// ensureArtifacts opens the tree-shared artifact store at the root session
// (S5.5); children inherit through childLoop, so refs resolve tree-wide.
func (l *Loop) ensureArtifacts() error {
	if l.Artifacts != nil || l.Store == nil {
		return nil
	}
	a, err := store.OpenArtifactStore(l.Store.Dir() + "/artifacts")
	if err != nil {
		return err
	}
	l.Artifacts = a
	return nil
}

// buildPublishRun is publish_artifact's Run closure (S5.5). Ordering is the
// invariant: the blob and manifest are DURABLE (fsynced) before the
// ArtifactPublished fact is journaled — a ref in the log always resolves; a
// crash in between leaves an orphan blob, never a dangling ref. appendE is
// the batch-serialized appender (the closure runs on an activity goroutine).
func (l *Loop) buildPublishRun(call provider.ToolCall, res *tool.Result,
	appendE AppendFunc) func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {

	return func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
		var args struct {
			Stream  string `json:"stream"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(call.Args, &args); err != nil || args.Stream == "" || args.Content == "" {
			*res = errorResult("publish_artifact: invalid args: need {\"stream\", \"content\"}")
			return res.Payload, nil, true, nil
		}
		if l.Artifacts == nil {
			*res = errorResult("publish_artifact: no artifact store in this run")
			return res.Payload, nil, true, nil
		}
		v, err := l.Artifacts.Publish(args.Stream, []byte(args.Content))
		if err != nil {
			return nil, nil, false, err // harness failure (disk), not model-visible
		}
		crash.Point(crash.PointAfterBlobBeforeEvent)
		if _, err := appendE(event.TypeArtifactPublished, &event.ArtifactPublished{
			Stream: v.Stream, Version: v.Version, Ref: v.Ref, Bytes: v.Bytes,
			Source: "tool",
		}); err != nil {
			return nil, nil, false, err
		}
		payload, _ := json.Marshal(map[string]any{
			"output": "published", "stream": v.Stream, "version": v.Version, "ref": v.Ref,
		})
		*res = tool.Result{Payload: payload}
		return res.Payload, nil, false, nil
	}
}
