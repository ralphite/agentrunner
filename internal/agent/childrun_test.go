package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// childRunLoop builds a minimal child Loop over the childRun's store — the
// caller-builds-the-Loop contract of the substrate.
func childRunLoop(t *testing.T, cr *ChildRun, fix scripted.Fixture) *Loop {
	t.Helper()
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &Loop{
		Spec: &AgentSpec{
			Name:               "child",
			Model:              ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			MaxGenerationSteps: 5,
		},
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     cr.Store(),
		Clock:     clock.NewFake(time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)),
		SessionID: "childrun-sess",
	}
}

// The three-way decision (INC-76): a fresh journal runs, a completed
// journal settles from the quiescent shape WITHOUT touching the provider,
// and a non-empty non-quiescent journal resumes.
func TestChildRunThreeWayDecision(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "child")

	// (a) fresh journal → Run.
	cr, err := OpenChildRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	res, spent, rerr := cr.Run(context.Background(), childRunLoop(t, cr, fix), "do the thing")
	cr.Close()
	if rerr != nil || res.Reason != "completed" {
		t.Fatalf("fresh run: res=%+v err=%v, want completed", res, rerr)
	}
	if spent != childFoldUsage(dir) {
		t.Fatalf("spent %+v != fold usage %+v", spent, childFoldUsage(dir))
	}
	events, _ := store.ReadEvents(dir)
	seqAfterRun := int64(0)
	if len(events) > 0 {
		seqAfterRun = events[len(events)-1].Seq
	}
	if seqAfterRun == 0 {
		t.Fatal("fresh run journaled nothing")
	}

	// (c) already-quiescent journal → settle from the shape, no provider
	// call and no new events. The empty fixture would ERROR if consulted —
	// its absence of steps is the guard.
	cr2, err := OpenChildRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	res2, spent2, rerr2 := cr2.Run(context.Background(), childRunLoop(t, cr2, scripted.Fixture{}), "ignored")
	cr2.Close()
	if rerr2 != nil || res2.Reason != "completed" {
		t.Fatalf("settled run: res=%+v err=%v, want completed from the shape", res2, rerr2)
	}
	if spent2 != childFoldUsage(dir) {
		t.Fatalf("settled spent %+v != fold usage", spent2)
	}
	events, _ = store.ReadEvents(dir)
	if events[len(events)-1].Seq != seqAfterRun {
		t.Fatalf("settled path appended events (%d → %d): it must never re-run",
			seqAfterRun, events[len(events)-1].Seq)
	}

	// (b) non-empty, NOT quiescent journal → Resume. A journal holding only
	// SessionStarted (crash before the opening turn) resumes and runs it.
	dir2 := filepath.Join(t.TempDir(), "child2")
	seed, err := store.OpenEventStore(dir2)
	if err != nil {
		t.Fatal(err)
	}
	env, err := event.New(event.TypeSessionStarted, &event.SessionStarted{
		SpecName: "child", Model: "m", Prompt: "resume me",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Append(env); err != nil {
		t.Fatal(err)
	}
	_ = seed.Close()
	cr3, err := OpenChildRun(dir2)
	if err != nil {
		t.Fatal(err)
	}
	fix3 := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "resumed"}, {Finish: "end_turn"}}},
	}}
	res3, _, rerr3 := cr3.Run(context.Background(), childRunLoop(t, cr3, fix3), "ignored — resume path")
	cr3.Close()
	if rerr3 != nil || res3.Reason != "completed" {
		t.Fatalf("resume run: res=%+v err=%v, want completed via Resume", res3, rerr3)
	}
	if n := countEvents(t, dir2, event.TypeAssistantMessage); n != 1 {
		t.Fatalf("resume path assistant messages = %d, want 1 (the crashed opening turn ran)", n)
	}
}

// The error path settles spend from the child fold — never from the
// (zeroed) RunResult (S5/S6 review; the substrate's single settle point).
func TestChildRunSettlesUsageOnError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "child")
	cr, err := OpenChildRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer cr.Close()
	// An empty fixture makes the very first provider call fail: the run
	// errors after journaling its opening facts.
	_, spent, rerr := cr.Run(context.Background(), childRunLoop(t, cr, scripted.Fixture{}), "will fail")
	if rerr == nil {
		t.Fatal("run with an empty fixture should error")
	}
	if spent != childFoldUsage(dir) {
		t.Fatalf("error-path spent %+v != fold usage %+v", spent, childFoldUsage(dir))
	}
}
