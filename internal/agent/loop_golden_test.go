package agent

import (
	"context"
	"encoding/json"
	"iter"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// capturingProvider records every CompleteRequest it serves — the request
// assembly golden pins the exact provider-visible shape (roles, part kinds,
// call ids, result placement) before S2.10 rewrites the loop orchestration.
type capturingProvider struct {
	inner    provider.Provider
	requests []provider.CompleteRequest
}

func (c *capturingProvider) Capabilities() provider.Capabilities { return c.inner.Capabilities() }

func (c *capturingProvider) Complete(ctx context.Context, req provider.CompleteRequest) iter.Seq2[provider.StreamEvent, error] {
	c.requests = append(c.requests, req)
	return c.inner.Complete(ctx, req)
}

func TestLoopRequestAssemblyGolden(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "greet.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	// S4.3 runs a turn's tool calls concurrently, so the read and the edit
	// must target DIFFERENT files — otherwise the read's result races the
	// edit and the golden is nondeterministic. This still pins the assembly
	// shape: two calls, two results, correct roles and ordering.
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("just notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "reading then editing"},
			{ToolCall: &scripted.ToolCallEvent{Name: "read_file", Args: map[string]any{"path": "notes.txt"}}},
			{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
				"path": "greet.txt", "old": "hello world", "new": "HELLO WORLD"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}

	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()

	cap := &capturingProvider{inner: scripted.New(fix)}
	loop := &Loop{
		Spec: &AgentSpec{
			Name:               "golden",
			Model:              ModelSpec{Provider: "scripted", ID: "model-x", MaxTokens: 256},
			SystemPrompt:       "be precise",
			Tools:              []string{"read_file", "edit_file"},
			MaxGenerationSteps: 5,
		},
		Provider:  cap,
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)),
		SessionID: "golden-sess",
	}
	if _, err := loop.Run(context.Background(), "make it loud"); err != nil {
		t.Fatal(err)
	}

	got, err := json.MarshalIndent(normalizeRequests(cap.requests), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	golden := filepath.Join("testdata", "request_assembly.golden")
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("missing golden (run with -update): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("request assembly drifted from golden.\n got:\n%s\nwant:\n%s", got, want)
	}
}

// envBlockRe matches the frozen env block whose contents (cwd = a random
// temp dir) are environment-specific; the golden pins shape, not the host.
var envBlockRe = regexp.MustCompile(`(?s)<env>.*?</env>`)

// normalizeRequests strips tool schemas (owned by the registry, not the
// loop) and normalizes the volatile env block so the golden pins
// orchestration shape only.
func normalizeRequests(reqs []provider.CompleteRequest) []map[string]any {
	out := make([]map[string]any, 0, len(reqs))
	for _, r := range reqs {
		toolNames := make([]string, 0, len(r.Tools))
		for _, td := range r.Tools {
			toolNames = append(toolNames, td.Name)
		}
		out = append(out, map[string]any{
			"turn":       r.GenStep,
			"model":      r.Model,
			"max_tokens": r.MaxTokens,
			"system":     envBlockRe.ReplaceAllString(r.System, "<env>NORMALIZED</env>"),
			"tools":      toolNames,
			"messages":   r.Messages,
		})
	}
	return out
}
