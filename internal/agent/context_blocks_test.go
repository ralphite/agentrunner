package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// S5.2: the system prompt lays out env → memory → skills → spec prompt →
// mode suffix, in that fixed order.
func TestAssembleSystemOrder(t *testing.T) {
	run := state.Session{
		Env:    "<env>\ncwd: /w\ndate: 2026-07-04\n</env>",
		Memory: "<memory>\nrules\n</memory>",
		Skills: "<skills>\n- s: d (p)\n</skills>",
	}
	sys := assembleSystem(run, "be precise", "plan")
	order := []string{"<env>", "<memory>", "<skills>", "be precise", "plan"}
	last := -1
	for _, marker := range order {
		i := strings.Index(sys, marker)
		if i < 0 {
			t.Fatalf("system missing %q:\n%s", marker, sys)
		}
		if i < last {
			t.Fatalf("%q out of order:\n%s", marker, sys)
		}
		last = i
	}
}

// S5.2 e2e: a workspace with a CLAUDE.md and a skill gets both frozen into
// SessionStarted and injected into the request prefix; the skill BODY stays out
// (on-demand loading), and editing the files mid-run does not change the
// prefix (frozen at session start).
func TestContextBlocksFrozenIntoRun(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("always use tabs"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(root, ".claude", "skills", "deploy")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: deploy\ndescription: ship it\n---\nSECRET BODY STEPS\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			// A tool call forces a second turn, whose request must carry the
			// SAME prefix even though the files change in between (frozen).
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
				Args: map[string]any{"command": "echo 'always use spaces' > CLAUDE.md"}}},
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
	t.Cleanup(func() { _ = es.Close() })
	cap := &capturingProvider{inner: scripted.New(fix)}
	l := &Loop{
		Spec: &AgentSpec{Name: "ctx",
			Model: ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			Tools: []string{"bash"}, MaxGenerationSteps: 3},
		Provider:  cap,
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)),
		SessionID: "ctx-sess",
	}
	if _, err := l.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}

	requests := cap.Requests()
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	sys := requests[0].System
	if !strings.Contains(sys, "always use tabs") {
		t.Errorf("memory missing from prefix:\n%s", sys)
	}
	if !strings.Contains(sys, "deploy: ship it") {
		t.Errorf("skills directory missing from prefix:\n%s", sys)
	}
	if strings.Contains(sys, "SECRET BODY STEPS") {
		t.Errorf("skill body leaked into the prefix:\n%s", sys)
	}
	// Frozen: turn 2's prefix is byte-identical despite the mid-run edit.
	if requests[1].System != sys {
		t.Errorf("prefix drifted after a mid-run memory edit:\n t1: %q\n t2: %q",
			sys, requests[1].System)
	}
}
