package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// TestCrashMatrix is the S2 exit gate: every named injection point and the
// counting predicates over a canonical run. Each row kills a subprocess at
// the armed point, then resumes in-process and requires either an exact
// continuation or an in-doubt refusal.
//
// S4.3 made same-turn tool calls execute CONCURRENTLY, which races the
// per-activity crash-point counter (two goroutines hit
// after_exec_before_journal in nondeterministic order). To keep the matrix
// a DETERMINISTIC sequential gate, the canonical run issues one tool per
// turn — read in t1, edit in t2, done in t3 — so every activity runs
// strictly in sequence. Concurrent multi-tool behavior is covered
// separately by TestParallelToolCalls.
//
// Canonical run event order (activity exec points in brackets):
//
//	session_started, input_received, generation_started(1) + snapshot,
//	activity_started(llm-t1) [exec hit 1] activity_completed, assistant_message(1),
//	activity_started(read) [exec hit 2] activity_completed,
//	generation_started(2) + snapshot,
//	activity_started(llm-t2) [exec hit 3] activity_completed, assistant_message(2),
//	activity_started(edit) [exec hit 4] activity_completed,
//	generation_started(3) + snapshot,
//	activity_started(llm-t3) [exec hit 5] activity_completed, assistant_message(3),
//	task_completed
func TestCrashMatrix(t *testing.T) {
	if os.Getenv("GO_CRASH_HELPER") == "1" {
		helperMatrixRun()
		return
	}

	rows := []struct {
		name          string
		predicate     string
		resumeFixture string // full | fromT2 | none
		wantInDoubt   bool
	}{
		{"run-started-only", "after:session_started:1", "full", false},
		{"input-journaled", "after:input_received:1", "full", false},
		{"input-point", "point:" + crash.PointAfterJournalInput, "full", false},
		{"llm-executed-unjournaled", "point:" + crash.PointAfterExecBeforeJournal + ":1", "full", false},
		{"llm-completed-unmessaged", "after:activity_completed:1", "full", false},
		{"assistant-journaled", "after:assistant_message:1", "fromT2", false},
		{"read-executed-unjournaled", "point:" + crash.PointAfterExecBeforeJournal + ":2", "fromT2", false},
		{"read-result-journaled", "after:activity_completed:2", "fromT2", false},
		// edit in-doubt SELF-HEALS (决策 #29): renders [interrupted by
		// crash], never re-runs; the loop continues into turn 3.
		{"edit-executed-unjournaled", "point:" + crash.PointAfterExecBeforeJournal + ":4", "fromT3", false},
		{"turn2-boundary", "after:generation_started:2", "fromT2", false},
		{"snapshot-written", "point:" + crash.PointAfterSnapshotWrite + ":2", "fromT2", false},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			base := t.TempDir()
			sessDir := filepath.Join(base, "sess")
			root := filepath.Join(base, "ws")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "greet.txt"), []byte("hello world"), 0o644); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(os.Args[0], "-test.run=TestCrashMatrix")
			cmd.Env = append(os.Environ(),
				"GO_CRASH_HELPER=1",
				"CRASH_SESS_DIR="+sessDir,
				"CRASH_WS="+root,
				crash.EnvVar+"="+row.predicate,
			)
			out, err := cmd.CombinedOutput()
			var ee *exec.ExitError
			if !errors.As(err, &ee) || ee.ExitCode() != crash.ExitCode {
				t.Fatalf("subprocess: err = %v, out = %s", err, out)
			}

			es, err := store.OpenEventStore(sessDir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = es.Close() }()
			ws, err := workspace.New(root)
			if err != nil {
				t.Fatal(err)
			}
			prov := scripted.New(matrixFixture(row.resumeFixture))
			l := &Loop{
				Spec:      matrixSpec(),
				Provider:  prov,
				Exec:      &tool.Executor{WS: ws},
				Store:     es,
				SessionID: "matrix",
			}
			res, err := l.Resume(context.Background())

			if row.wantInDoubt {
				var inDoubt *InDoubtError
				if !errors.As(err, &inDoubt) {
					t.Fatalf("err = %v, want InDoubtError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("resume: %v", err)
			}
			if res.Reason != "completed" || res.GenSteps != 3 {
				t.Fatalf("res = %+v", res)
			}
			if err := prov.Done(); err != nil {
				t.Errorf("fixture: %v", err)
			}

			// 分毫不差: the file is fixed, the log is gapless, the fold is
			// quiescent with nothing in flight (决策 #31: no terminal event).
			got, _ := os.ReadFile(filepath.Join(root, "greet.txt"))
			if string(got) != "HELLO WORLD" {
				t.Errorf("file = %q", got)
			}
			events, err := store.ReadEvents(sessDir)
			if err != nil {
				t.Fatal(err)
			}
			for i, e := range events {
				if e.Seq != int64(i+1) {
					t.Fatalf("seq gap at %d: %d", i, e.Seq)
				}
			}
			final, err := state.Fold(events)
			if err != nil {
				t.Fatal(err)
			}
			if q, reason := state.Quiescence(final); !q || reason != "completed" || len(final.Activities) != 0 {
				t.Errorf("final fold: quiescent=%v reason=%s in-flight=%v", q, reason, final.Activities)
			}
		})
	}
}

func matrixSpec() *AgentSpec {
	return &AgentSpec{
		Name:               "matrix",
		Model:              ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt:       "be precise",
		Tools:              []string{"read_file", "edit_file"},
		MaxGenerationSteps: 5,
	}
}

func matrixFixture(kind string) scripted.Fixture {
	// One tool per turn keeps every activity strictly sequential, so the
	// crash-point counter is deterministic even after S4.3's concurrent
	// same-turn execution (TestParallelToolCalls covers that path).
	turn1 := scripted.Step{Respond: []scripted.Event{
		{Text: "reading first"},
		{ToolCall: &scripted.ToolCallEvent{Name: "read_file", Args: map[string]any{"path": "greet.txt"}}},
		{Finish: "tool_use"},
	}}
	turn2 := scripted.Step{Respond: []scripted.Event{
		{Text: "now editing"},
		{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
			"path": "greet.txt", "old": "hello world", "new": "HELLO WORLD"}}},
		{Finish: "tool_use"},
	}}
	turn3 := scripted.Step{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}}
	switch kind {
	case "full":
		return scripted.Fixture{Steps: []scripted.Step{turn1, turn2, turn3}}
	case "fromT2":
		return scripted.Fixture{Steps: []scripted.Step{turn2, turn3}}
	case "fromT3":
		return scripted.Fixture{Steps: []scripted.Step{turn3}}
	default:
		return scripted.Fixture{}
	}
}

func helperMatrixRun() {
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
	l := &Loop{
		Spec:      matrixSpec(),
		Provider:  scripted.New(matrixFixture("full")),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		SessionID: "matrix",
	}
	_, _ = l.Run(context.Background(), "make it loud")
	fmt.Println("UNREACHABLE: predicate did not fire")
	os.Exit(0)
}
