package driver_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/driver"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// harness wires a driver whose child, each iteration, appends one line to
// progress.txt (a fresh scripted run over the shared workspace). The command
// verifier is supplied by the caller.
func harness(t *testing.T, spec *driver.DriverSpec) (*driver.Driver, *store.EventStore) {
	t.Helper()
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	exec := &tool.Executor{WS: ws}
	dStore, err := store.OpenEventStore(filepath.Join(root, "driver"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dStore.Close() })

	childSpec := &agent.AgentSpec{
		Name:         "worker",
		Model:        agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "make progress",
		Tools:        []string{"bash"},
		MaxTurns:     5,
	}
	spec.Agent = childSpec
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
				Args: map[string]any{"command": "echo tick >> progress.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "appended a line"}, {Finish: "end_turn"}}},
	}}
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))

	d := &driver.Driver{
		Spec:     spec,
		Store:    dStore,
		Clock:    clk,
		DriverID: "drv-1",
		Exec:     exec,
		NewChild: func(cs *store.EventStore, session string, iter int) *agent.Loop {
			return &agent.Loop{
				Spec:      childSpec,
				Provider:  scripted.New(fix),
				Exec:      exec,
				Store:     cs,
				Clock:     clk,
				SessionID: session,
			}
		},
	}
	return d, dStore
}

// Goal mode: a metric-free command verifier that passes once the workspace
// has accumulated enough lines. The child adds one line per iteration, so the
// goal is satisfied on exactly the third iteration.
func TestDriverGoalSatisfied(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 5,
		Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierCommand, Command: "test $(wc -l < progress.txt) -ge 3",
		}},
	})

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.Iterations != 3 {
		t.Fatalf("res = %+v, want satisfied at iteration 3", res)
	}

	events, err := store.ReadEvents(dStore.Dir())
	if err != nil {
		t.Fatal(err)
	}
	st, err := driver.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != driver.StatusEnded || st.Reason != "satisfied" {
		t.Fatalf("fold status=%s reason=%s", st.Status, st.Reason)
	}
	if len(st.Iterations) != 3 {
		t.Fatalf("iterations = %d, want 3", len(st.Iterations))
	}
	if !st.Iterations[2].Completed || !st.Iterations[2].Verdict.Pass {
		t.Errorf("iteration 3 = %+v, want completed+pass", st.Iterations[2])
	}
	if st.Iterations[0].Verdict.Pass || st.Iterations[1].Verdict.Pass {
		t.Errorf("iterations 1-2 should have failed the verifier")
	}
	// The terminal DriverCompleted is the last fact.
	last := events[len(events)-1]
	if last.Type != event.TypeDriverCompleted {
		t.Fatalf("last event = %s, want driver_completed", last.Type)
	}
}

// Goal mode: a verifier that never passes ends the run at the iteration cap.
func TestDriverGoalMaxIterations(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 2,
		Verifiers: []driver.VerifierSpec{{
			Kind: driver.VerifierCommand, Command: "test -f never-created",
		}},
	})

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "max_iterations" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want max_iterations at 2", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if len(st.Iterations) != 2 {
		t.Fatalf("iterations = %d, want 2", len(st.Iterations))
	}
	// Each iteration DID run a child (fresh journal under sub/iter-N).
	for i := 1; i <= 2; i++ {
		sub := filepath.Join(dStore.Dir(), "sub", "iter-"+itoa(i))
		cev, err := store.ReadEvents(sub)
		if err != nil || len(cev) == 0 {
			t.Fatalf("child iter-%d journal missing", i)
		}
	}
}

// Goal mode with a scored metric verifier: the score is the current line
// count, threshold 3. Score climbs 1→2→3 and passes on the third.
func TestDriverMetricVerifier(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 5,
		Verifiers: []driver.VerifierSpec{{
			Kind:        driver.VerifierCommand,
			Command:     "wc -l < progress.txt",
			MetricRegex: `(\d+)`,
			Threshold:   3,
		}},
	})

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.Iterations != 3 {
		t.Fatalf("res = %+v, want satisfied at 3", res)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if got := st.Iterations[2].Verdict.Score; got != 3 {
		t.Errorf("iteration 3 score = %v, want 3", got)
	}
	if st.BestIter != 3 {
		t.Errorf("best iter = %d, want 3 (highest score)", st.BestIter)
	}
}

// Goal mode with patience: a metric verifier whose threshold is unreachable
// (the score plateaus once it stops climbing). Here the child appends one
// line per iteration so the score keeps rising — to force a plateau we cap
// the metric with a threshold above any reachable value AND a patience that
// trips once no NEW best appears. We assert the stalled path by making the
// verifier score constant (a fixed metric) so no iteration improves.
func TestDriverGoalStalled(t *testing.T) {
	d, dStore := harness(t, &driver.DriverSpec{
		Name: "goal", Task: "add a line", MaxIterations: 10, Patience: 2,
		Verifiers: []driver.VerifierSpec{{
			Kind:        driver.VerifierCommand,
			Command:     "echo 1", // constant score 1, threshold 5 → never passes, never improves
			MetricRegex: `(\d+)`,
			Threshold:   5,
		}},
	})

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "stalled" {
		t.Fatalf("res = %+v, want stalled", res)
	}
	// Best is iteration 1 (first to reach the constant score); patience 2 means
	// two more non-improving iterations end it — iteration 3.
	if res.Iterations != 3 {
		t.Fatalf("stalled at iteration %d, want 3", res.Iterations)
	}
	events, _ := store.ReadEvents(dStore.Dir())
	st, _ := driver.Fold(events)
	if st.BestIter != 1 {
		t.Errorf("best iter = %d, want 1", st.BestIter)
	}
}

// Every event in event.DriverStream must fold into driver state — the mirror
// of the run fold's TestApplyCoversRegistry, so no driver-stream type is left
// unhandled anywhere.
func TestFoldCoversDriverStream(t *testing.T) {
	for typ := range event.DriverStream {
		e, err := event.New(typ, event.Registry[typ]())
		if err != nil {
			t.Fatalf("new %s: %v", typ, err)
		}
		if _, err := driver.Fold([]event.Envelope{e}); err != nil {
			t.Errorf("driver fold rejects %q: %v", typ, err)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
