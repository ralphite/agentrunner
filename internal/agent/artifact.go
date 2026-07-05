package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
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

// materializeInputs writes the run's artifact inputs into the workspace
// (S5.8) as ONE idempotent activity — Started/terminal journaled like any
// side effect, so resume sees whether it happened (fold Run.Materialized)
// and can safely re-run it when it did not (same refs → same bytes).
func (l *Loop) materializeInputs(ctx context.Context, ds *driveState, appendE AppendFunc) error {
	inputs := ds.s.Session.Inputs
	if len(inputs) == 0 || ds.s.Session.Materialized {
		return nil
	}
	if l.Artifacts == nil || l.Exec == nil || l.Exec.WS == nil {
		return fmt.Errorf("materialize: run has artifact inputs but no artifact store/workspace")
	}
	// Every input write is an EDIT-class effect through the pipeline (PLAN
	// S5.8 "过管线,可审计"; S5 review): a path-scoped deny rule must bind a
	// write that arrives via spawn inputs exactly like one via edit_file.
	// This is pre-turn harness work, so a denial fails the run closed.
	for i, in := range inputs {
		args, _ := json.Marshal(map[string]string{"path": in.Path, "ref": in.Ref})
		eff := pipeline.Effect{
			ID: fmt.Sprintf("eff-materialize-%d", i), Kind: "tool_call",
			ToolName: "materialize", Class: string(tool.ClassEdit),
			Args: args, Mode: ds.s.CurrentMode(),
			Budget: budgetView(ds.s),
		}
		outcome, allowed, err := l.adjudicate(ctx, ds, appendE, eff)
		if err != nil {
			return err
		}
		if !allowed {
			return fmt.Errorf("materialize %s: denied by %s", in.Path, denyingGate(outcome))
		}
	}
	exec := &ActivityExecutor{Append: appendE, Clock: l.Clock, Redact: redact.FromEnv()}
	return exec.Do(ctx, Activity{
		ID: "materialize", Kind: event.KindTool, Name: "materialize",
		Idempotent: true,
		Run: func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			var written []string
			for _, in := range inputs {
				content, err := l.Artifacts.Get(in.Ref)
				if err != nil {
					return nil, nil, false, fmt.Errorf("materialize %s: %w", in.Path, err)
				}
				dest, err := l.Exec.WS.Resolve(in.Path)
				if err != nil {
					return nil, nil, false, fmt.Errorf("materialize %s: %w", in.Path, err)
				}
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					return nil, nil, false, err
				}
				if err := os.WriteFile(dest, content, 0o644); err != nil {
					return nil, nil, false, err
				}
				written = append(written, in.Path)
			}
			payload, _ := json.Marshal(map[string]any{"materialized": written})
			return payload, nil, false, nil
		},
	})
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
		// Artifact blobs are a durable session-tier sink like the journal:
		// model output passes credential redaction before persisting (S5
		// review — the CAS must not be the one unredacted tier).
		v, err := l.Artifacts.Publish(args.Stream, []byte(redact.FromEnv().String(args.Content)))
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
