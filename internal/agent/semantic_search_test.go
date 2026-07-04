package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
)

// S7 模块 4 e2e: semantic_search is a read-class tool — auto-allowed in
// default mode, idempotent on resume — and its ranked hits reach the model
// as a normal tool result.
func TestSemanticSearchToolEndToEnd(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "auth.go"),
		[]byte("package auth\nfunc CheckToken(t string) bool { return t != \"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "q1", Name: "semantic_search",
				Args: map[string]any{"query": "where is the token checked"}}},
			{Finish: "tool_use"},
		}},
		{
			Expect:  scripted.Expect{LastMessageContains: "auth.go"},
			Respond: []scripted.Event{{Text: "found it in auth.go"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, root)
	l.Spec.Tools = []string{"semantic_search", "read_file"}

	res, err := l.Run(context.Background(), "find the token check")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var started *event.ActivityStarted
	for _, e := range events {
		if e.Type == event.TypeActivityStarted && strings.Contains(string(e.Payload), "semantic_search") {
			dec, _ := event.DecodePayload(e)
			started = dec.(*event.ActivityStarted)
		}
	}
	if started == nil {
		t.Fatal("no semantic_search activity journaled")
	}
	if !started.Idempotent {
		t.Error("read-class search must be idempotent for resume")
	}
}
