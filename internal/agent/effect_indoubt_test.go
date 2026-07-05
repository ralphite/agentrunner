package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// hookishGate declares side effects (the 3.8 hook gate shape).
type hookishGate struct{}

func (hookishGate) Name() string                                             { return "hooks" }
func (hookishGate) Check(context.Context, pipeline.Effect) pipeline.Decision { return pipeline.Allow }
func (hookishGate) SideEffecting() bool                                      { return true }

// pureGate has no side effects.
type pureGate struct{}

func (pureGate) Name() string                                             { return "permission" }
func (pureGate) Check(context.Context, pipeline.Effect) pipeline.Decision { return pipeline.Allow }

func effectFixture(kind string) scripted.Fixture {
	turn1 := scripted.Step{Respond: []scripted.Event{
		{ToolCall: &scripted.ToolCallEvent{Name: "bash", Args: map[string]any{"command": "true"}}},
		{Finish: "tool_use"},
	}}
	turn2 := scripted.Step{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}}
	if kind == "turn2" {
		return scripted.Fixture{Steps: []scripted.Step{turn2}}
	}
	return scripted.Fixture{Steps: []scripted.Step{turn1, turn2}}
}

func effectCrashLoop(sessDir, root string, gates []pipeline.Gate, fix scripted.Fixture) (*Loop, error) {
	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		return nil, err
	}
	ws, err := workspace.New(root)
	if err != nil {
		return nil, err
	}
	return &Loop{
		Spec: &AgentSpec{Name: "eff", Model: ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 50},
			SystemPrompt: "s", Tools: []string{"bash"}, MaxGenerationSteps: 5},
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		SessionID: "eff-crash",
		Pipeline:  &pipeline.Pipeline{Gates: gates},
	}, nil
}

// Crash between gates and resolution. With a side-effecting gate in the
// pipeline, resume surfaces in-doubt (hooks may have run); with only pure
// gates, resume silently re-adjudicates and finishes.
func TestEffectAdjudicationCrashWindow(t *testing.T) {
	if os.Getenv("GO_CRASH_HELPER") == "1" {
		gates := []pipeline.Gate{pureGate{}}
		if os.Getenv("CRASH_GATES") == "hooks" {
			gates = []pipeline.Gate{hookishGate{}}
		}
		l, err := effectCrashLoop(os.Getenv("CRASH_SESS_DIR"), os.Getenv("CRASH_WS"), gates, effectFixture("full"))
		if err != nil {
			fmt.Println("helper:", err)
			os.Exit(1)
		}
		_, _ = l.Run(context.Background(), "run true")
		fmt.Println("UNREACHABLE: point did not fire")
		os.Exit(0)
	}

	for _, tc := range []struct {
		name, gates string
		wantInDoubt bool
	}{
		{"side-effecting-gates-surface", "hooks", true},
		{"pure-gates-readjudicate", "pure", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			sessDir := filepath.Join(base, "sess")
			root := filepath.Join(base, "ws")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(os.Args[0], "-test.run=TestEffectAdjudicationCrashWindow")
			cmd.Env = append(os.Environ(),
				"GO_CRASH_HELPER=1",
				"CRASH_SESS_DIR="+sessDir,
				"CRASH_WS="+root,
				"CRASH_GATES="+tc.gates,
				// Hit 2: hit 1 is the llm effect's window, hit 2 the tool's.
				crash.EnvVar+"=point:"+crash.PointBetweenGateAndResolved+":2",
			)
			out, err := cmd.CombinedOutput()
			var ee *exec.ExitError
			if !errors.As(err, &ee) || ee.ExitCode() != crash.ExitCode {
				t.Fatalf("subprocess: err = %v, out = %s", err, out)
			}

			gates := []pipeline.Gate{pureGate{}}
			if tc.gates == "hooks" {
				gates = []pipeline.Gate{hookishGate{}}
			}
			// GenStep 1's assistant message is already journaled: resume only
			// needs the remaining turn.
			l, err := effectCrashLoop(sessDir, root, gates, effectFixture("turn2"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = l.Store.Close() }()
			res, err := l.Resume(context.Background())

			if tc.wantInDoubt {
				var inDoubt *InDoubtError
				if !errors.As(err, &inDoubt) {
					t.Fatalf("err = %v, want InDoubtError", err)
				}
				if len(inDoubt.Effects) != 1 || !strings.Contains(err.Error(), "mid-adjudication") {
					t.Fatalf("in-doubt = %+v (%v)", inDoubt.Effects, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("pure-gate resume: %v", err)
			}
			if res.Reason != "completed" || res.GenSteps != 2 {
				t.Fatalf("res = %+v", res)
			}
		})
	}
}
