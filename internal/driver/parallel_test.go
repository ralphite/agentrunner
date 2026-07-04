package driver_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/driver"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/snapshot"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// parallelHarness wires a best-of-N driver over a real shadow snapshot
// store. The shared scripted provider serves the attempts IN ORDER — each
// attempt writes a different answer into ITS OWN worktree.
func parallelHarness(t *testing.T, spec *driver.DriverSpec, fix scripted.Fixture) (*driver.Driver, *store.EventStore, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "base.txt"), []byte("from-base"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := snapshot.NewShadowRepo(filepath.Join(t.TempDir(), "shadow.git"), root)
	if err != nil {
		t.Fatal(err)
	}
	dStore, err := store.OpenEventStore(filepath.Join(root, "driver"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dStore.Close() })

	childSpec := &agent.AgentSpec{
		Name:         "worker",
		Model:        agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "answer",
		Tools:        []string{"bash"},
		MaxTurns:     5,
	}
	spec.Agent = childSpec
	clk := clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))
	prov := scripted.New(fix)

	d := &driver.Driver{
		Spec:      spec,
		Store:     dStore,
		Clock:     clk,
		DriverID:  "drv-1",
		Exec:      &tool.Executor{WS: ws},
		Snapshots: snaps,
		NewChild: func(cs *store.EventStore, session string, iter, budgetTokens int) *agent.Loop {
			t.Fatal("parallel must use the worktree factory")
			return nil
		},
		NewChildAt: func(cs *store.EventStore, session string, iter, budgetTokens int, worktree string) *agent.Loop {
			wtWS, werr := workspace.New(worktree)
			if werr != nil {
				t.Fatalf("worktree workspace: %v", werr)
			}
			return &agent.Loop{
				Spec:      childSpec,
				Provider:  prov,
				Exec:      &tool.Executor{WS: wtWS, Session: session},
				Store:     cs,
				Clock:     clk,
				SessionID: session,
			}
		},
	}
	return d, dStore, root
}

// Best-of-N e2e: two attempts from one base snapshot; attempt 1 writes a
// wrong answer, attempt 2 the right one. The command verifier judges each
// attempt IN ITS OWN worktree; the winner is attempt 2, the main workspace
// is never touched, and the round's facts land in the driver stream.
func TestDriverParallelBestOfN(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		// Attempt 1: wrong answer.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
				Args: map[string]any{"command": "echo wrong > answer.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
		// Attempt 2: right answer.
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "bash",
				Args: map[string]any{"command": "echo right > answer.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	d, dStore, root := parallelHarness(t, &driver.DriverSpec{
		Name: "pick", Schedule: driver.ScheduleParallel, N: 2,
		Task: "write the right answer",
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierCommand,
			Command: "test -f base.txt && grep -qx right answer.txt"}},
	}, fix)

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "satisfied" || res.BestIter != 2 || res.Iterations != 2 {
		t.Fatalf("res = %+v, want satisfied best=2 of 2", res)
	}

	// Isolation: each worktree has its own answer, materialized from base;
	// the MAIN workspace got neither an answer nor any edit.
	for n, want := range map[int]string{1: "wrong", 2: "right"} {
		wt := filepath.Join(dStore.Dir(), "wt", "att-"+string(rune('0'+n)))
		if got, rerr := os.ReadFile(filepath.Join(wt, "answer.txt")); rerr != nil || strings.TrimSpace(string(got)) != want {
			t.Errorf("attempt %d answer = %q err=%v, want %s", n, got, rerr, want)
		}
		if base, rerr := os.ReadFile(filepath.Join(wt, "base.txt")); rerr != nil || string(base) != "from-base" {
			t.Errorf("attempt %d base.txt = %q err=%v (not materialized from snapshot?)", n, base, rerr)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "answer.txt")); !os.IsNotExist(err) {
		t.Error("main workspace was touched by an attempt")
	}

	// Stream facts: both attempts scheduled with the SAME base ref; the
	// terminal records the selection.
	events, err := store.ReadEvents(dStore.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var refs []string
	var completed *event.DriverCompleted
	for _, e := range events {
		switch e.Type {
		case event.TypeIterationScheduled:
			dec, _ := event.DecodePayload(e)
			refs = append(refs, dec.(*event.IterationScheduled).BaseRef)
		case event.TypeDriverCompleted:
			dec, _ := event.DecodePayload(e)
			completed = dec.(*event.DriverCompleted)
		}
	}
	if len(refs) != 2 || refs[0] == "" || refs[0] != refs[1] {
		t.Errorf("base refs = %v, want two identical non-empty", refs)
	}
	if completed == nil || completed.BestIter != 2 || completed.Reason != "satisfied" {
		t.Errorf("driver_completed = %+v", completed)
	}
}

// No attempt passes: the round ends stalled with the best score's attempt
// as BestIter — pass beats score, ties keep the earliest.
func TestDriverParallelAllFailStalls(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "attempt 1 no-op"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "attempt 2 no-op"}, {Finish: "end_turn"}}},
	}}
	d, _, _ := parallelHarness(t, &driver.DriverSpec{
		Name: "pick", Schedule: driver.ScheduleParallel, N: 2,
		Task: "write the right answer",
		Verifiers: []driver.VerifierSpec{{Kind: driver.VerifierCommand,
			Command: "grep -qx right answer.txt"}},
	}, fix)

	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "stalled" || res.Iterations != 2 {
		t.Fatalf("res = %+v, want stalled after the full round", res)
	}
	if res.BestIter != 1 {
		t.Errorf("BestIter = %d, want earliest on all-equal scores", res.BestIter)
	}
}

// Spec validation: parallel without n / verifiers / stores refuses to run.
func TestDriverParallelValidation(t *testing.T) {
	verifiers := []driver.VerifierSpec{{Kind: driver.VerifierCommand, Command: "true"}}
	cases := []struct {
		name   string
		mutate func(*driver.Driver)
	}{
		{"n too small", func(d *driver.Driver) { d.Spec.N = 1 }},
		{"no verifiers", func(d *driver.Driver) { d.Spec.Verifiers = nil }},
		{"no snapshot store", func(d *driver.Driver) { d.Snapshots = nil }},
		{"no worktree factory", func(d *driver.Driver) { d.NewChildAt = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, _, _ := parallelHarness(t, &driver.DriverSpec{
				Name: "pick", Schedule: driver.ScheduleParallel, N: 2,
				Task: "t", Verifiers: verifiers,
			}, scripted.Fixture{})
			tc.mutate(d)
			if _, err := d.Run(context.Background()); err == nil {
				t.Fatal("want validation error")
			}
		})
	}
}
