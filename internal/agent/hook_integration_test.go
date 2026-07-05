package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// The 3.8 exit criterion: hooks with side effects must NOT re-run on the
// recovery path. A real pre hook appends to a marker file; the process is
// killed between the hook and the resolution; resume surfaces in-doubt
// (3.2 machinery + real hook), and the marker still has exactly one line.
func TestHooksDoNotRerunOnResume(t *testing.T) {
	if os.Getenv("GO_CRASH_HELPER") == "1" {
		helperHookRun()
		return
	}

	base := t.TempDir()
	sessDir := filepath.Join(base, "sess")
	root := filepath.Join(base, "ws")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestHooksDoNotRerunOnResume")
	cmd.Env = append(os.Environ(),
		"GO_CRASH_HELPER=1",
		"CRASH_SESS_DIR="+sessDir,
		"CRASH_WS="+root,
		// Hit 2 = the tool effect's window (hit 1 is the llm effect).
		crash.EnvVar+"=point:"+crash.PointBetweenGateAndResolved+":2",
	)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != crash.ExitCode {
		t.Fatalf("subprocess: err = %v, out = %s", err, out)
	}

	marker, err := os.ReadFile(filepath.Join(root, "hooks.log"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(marker), "pre\n"); got != 1 {
		t.Fatalf("pre hook ran %d times before crash, want 1", got)
	}

	l, err := hookLoop(sessDir, root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Store.Close() }()
	_, err = l.Resume(context.Background())
	var inDoubt *InDoubtError
	if !errors.As(err, &inDoubt) {
		t.Fatalf("err = %v, want InDoubtError (hooks ran, no resolution)", err)
	}

	marker, _ = os.ReadFile(filepath.Join(root, "hooks.log"))
	if got := strings.Count(string(marker), "pre\n"); got != 1 {
		t.Fatalf("pre hook re-ran on resume: %d lines", got)
	}
}

func hookLoop(sessDir, root string) (*Loop, error) {
	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		return nil, err
	}
	ws, err := workspace.New(root)
	if err != nil {
		return nil, err
	}
	runner := &hook.Runner{PreTool: []string{"echo pre >> hooks.log"}, Dir: root}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{"command": "true"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	return &Loop{
		Spec: &AgentSpec{Name: "hooked", Model: ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 50},
			SystemPrompt: "s", Tools: []string{"bash"}, MaxGenerationSteps: 5,
			Permissions: []pipeline.PermissionRule{{Action: "allow"}}},
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		SessionID: "hooked",
		Pipeline: &pipeline.Pipeline{Gates: []pipeline.Gate{
			&hook.Gate{Runner: runner},
			&pipeline.PermissionGate{Rules: []pipeline.PermissionRule{{Action: "allow"}}, WS: ws},
		}},
		Hooks: runner,
	}, nil
}

func helperHookRun() {
	l, err := hookLoop(os.Getenv("CRASH_SESS_DIR"), os.Getenv("CRASH_WS"))
	if err != nil {
		fmt.Println("helper:", err)
		os.Exit(1)
	}
	_, _ = l.Run(context.Background(), "run true")
	fmt.Println("UNREACHABLE: point did not fire")
	os.Exit(0)
}

// Post hooks attach their output to the completion fact.
func TestPostHookNoteJournaled(t *testing.T) {
	root := t.TempDir()
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{"command": "true"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, root)
	l.Hooks = &hook.Runner{PostTool: []string{`echo "checked by post hook"`}, Dir: root}

	if _, err := l.Run(context.Background(), "run true"); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var note string
	for _, e := range events {
		if e.Type == event.TypeActivityCompleted && strings.Contains(string(e.Payload), "tool-call_1_0") {
			var completed event.ActivityCompleted
			if err := json.Unmarshal(e.Payload, &completed); err != nil {
				t.Fatal(err)
			}
			note = completed.HookNote
		}
	}
	if note != "checked by post hook" {
		t.Fatalf("hook_note = %q", note)
	}
}
