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
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// The 2.15 crash-matrix scenario: killed at after_exec_before_journal —
// bash ran (marker line written) but no terminal event landed. Resume must
// surface in-doubt and must NOT re-run the command.
func TestInDoubtSurfacesAndDoesNotRerun(t *testing.T) {
	if os.Getenv("GO_CRASH_HELPER") == "1" {
		helperInDoubtRun(t)
		return
	}

	base := t.TempDir()
	sessDir := filepath.Join(base, "sess")
	root := filepath.Join(base, "ws")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestInDoubtSurfacesAndDoesNotRerun")
	cmd.Env = append(os.Environ(),
		"GO_CRASH_HELPER=1",
		"CRASH_SESS_DIR="+sessDir,
		"CRASH_WS="+root,
		crash.EnvVar+"=point:"+crash.PointAfterExecBeforeJournal+":2", // hit 1 = llm-t1, hit 2 = the bash tool
	)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != crash.ExitCode {
		t.Fatalf("subprocess: err = %v, out = %s", err, out)
	}

	// The effect happened exactly once before the crash.
	marker, err := os.ReadFile(filepath.Join(root, "marker.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(marker), "ran\n"); got != 1 {
		t.Fatalf("marker lines = %d, want 1 (pre-crash effect)", got)
	}

	// Resume: refuses with the in-doubt activity named.
	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	l := &Loop{
		Spec:      inDoubtSpec(),
		Provider:  scripted.New(scripted.Fixture{}), // must never be called
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		SessionID: "in-doubt",
	}
	_, err = l.Resume(context.Background())
	var inDoubt *InDoubtError
	if !errors.As(err, &inDoubt) {
		t.Fatalf("err = %v, want InDoubtError", err)
	}
	if len(inDoubt.Activities) != 1 || inDoubt.Activities[0].Name != "bash" {
		t.Fatalf("in-doubt = %+v", inDoubt.Activities)
	}
	if !strings.Contains(err.Error(), "refusing to re-run") {
		t.Errorf("message = %q", err)
	}

	// And it did NOT re-run.
	marker, _ = os.ReadFile(filepath.Join(root, "marker.txt"))
	if got := strings.Count(string(marker), "ran\n"); got != 1 {
		t.Fatalf("marker lines after resume = %d — in-doubt activity was re-run", got)
	}
}

func inDoubtSpec() *AgentSpec {
	return &AgentSpec{
		Name:               "doubter",
		Model:              ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt:       "s",
		Tools:              []string{"bash", "read_file"},
		MaxGenerationSteps: 5,
	}
}

func helperInDoubtRun(t *testing.T) {
	es, err := store.OpenEventStore(os.Getenv("CRASH_SESS_DIR"))
	if err != nil {
		fmt.Println("helper:", err)
		os.Exit(1)
	}
	ws, err := workspace.New(os.Getenv("CRASH_WS"))
	if err != nil {
		fmt.Println("helper:", err)
		os.Exit(1)
	}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "bash",
				Args: map[string]any{"command": "echo ran >> marker.txt"}}},
			{Finish: "tool_use"},
		}},
	}}
	l := &Loop{
		Spec:      inDoubtSpec(),
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		SessionID: "in-doubt",
	}
	_, _ = l.Run(context.Background(), "leave a mark")
	fmt.Println("UNREACHABLE: point did not fire")
	os.Exit(0)
}

// Idempotent in-flight activities are NOT in doubt: resume re-runs them.
// Synthetic log: turn 1 assistant called read_file, Started journaled
// (idempotent), no terminal — then the process died.
func TestIdempotentInFlightRerunsOnResume(t *testing.T) {
	base := t.TempDir()
	sessDir := filepath.Join(base, "sess")
	root := filepath.Join(base, "ws")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "greet.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	asst := event.AssistantMessage{GenStep: 1, Message: providerAssistantToolCall("call_1_0", "read_file", `{"path":"greet.txt"}`)}
	for _, pair := range []struct {
		typ     string
		payload any
	}{
		{event.TypeSessionStarted, &event.SessionStarted{SpecName: "t", SubStateVersions: state.SubStateVersions()}},
		{event.TypeInputReceived, &event.InputReceived{Text: "read it", Source: "cli"}},
		{event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}},
		{event.TypeAssistantMessage, &asst},
		{event.TypeActivityStarted, &event.ActivityStarted{ActivityID: "tool-call_1_0",
			Kind: event.KindTool, Name: "read_file", CallID: "call_1_0", Idempotent: true, Attempt: 1}},
	} {
		env, err := event.New(pair.typ, pair.payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := es.Append(env); err != nil {
			t.Fatal(err)
		}
	}
	_ = es.Close() // crash

	es2, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es2.Close() }()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	prov := scripted.New(scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}})
	l := &Loop{
		Spec:      inDoubtSpec(),
		Provider:  prov,
		Exec:      &tool.Executor{WS: ws},
		Store:     es2,
		SessionID: "idem",
	}
	res, err := l.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.GenSteps != 2 {
		t.Fatalf("res = %+v", res)
	}
	if err := prov.Done(); err != nil {
		t.Error(err)
	}

	// The re-run produced a real result and drained the in-flight set.
	events, err := store.ReadEvents(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	final, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if len(final.Activities) != 0 {
		t.Errorf("in-flight not drained: %+v", final.Activities)
	}
	tr, ok := final.Conversation.ToolResults["call_1_0"]
	if !ok || tr.IsError {
		t.Errorf("re-run result = %+v (ok=%v)", tr, ok)
	}
}

func providerAssistantToolCall(callID, name, args string) provider.Message {
	return provider.Message{
		Role: provider.RoleAssistant,
		Parts: []provider.Part{{
			Kind: provider.PartToolCall, CallID: callID, ToolName: name, Args: json.RawMessage(args),
		}},
	}
}
