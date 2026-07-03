package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// askEverything makes every edit/execute call require approval.
var askEverything = &pipeline.Pipeline{Gates: []pipeline.Gate{
	policyGate{name: "permission", check: func(eff pipeline.Effect) pipeline.Decision {
		if eff.Kind == "tool_call" && (eff.Class == "edit" || eff.Class == "execute") {
			return pipeline.Ask(eff.Class + " requires approval")
		}
		return pipeline.Allow
	}},
}}

func approvalFixture() scripted.Fixture {
	return scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
				"path": "note.txt", "old": "", "new": "hello"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
}

// Approve path: the full fact chain lands in order and the effect executes.
func TestApprovalApprovePath(t *testing.T) {
	t.Setenv("AGENTRUNNER_APPROVE", "always")
	root := t.TempDir()
	l := testLoop(t, approvalFixture(), root)
	l.Pipeline = askEverything

	res, err := l.Run(context.Background(), "write a note")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.Turns != 2 {
		t.Fatalf("res = %+v", res)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "note.txt")); string(got) != "hello" {
		t.Fatalf("file = %q — approved effect did not execute", got)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, e := range events {
		switch e.Type {
		case event.TypeApprovalRequested, event.TypeWaitingEntered,
			event.TypeApprovalResponded, event.TypeWaitingResolved:
			order = append(order, e.Type)
		case event.TypeEffectResolved:
			if strings.Contains(string(e.Payload), "eff-call_1_0") {
				order = append(order, e.Type)
			}
		case event.TypeActivityStarted:
			if strings.Contains(string(e.Payload), "tool-call_1_0") {
				order = append(order, e.Type)
			}
		}
	}
	want := []string{"approval_requested", "waiting_entered", "approval_responded",
		"waiting_resolved", "effect_resolved", "activity_started"}
	if !equal(order, want) {
		t.Fatalf("fact chain = %v, want %v", order, want)
	}
}

// slowApprover approves only after the clock reaches its release time.
type slowApprover struct {
	clk     clock.Clock
	release time.Time
}

func (a slowApprover) Resolve(ctx context.Context, _ ApprovalRequest) (ApprovalDecision, error) {
	if err := a.clk.WaitUntil(ctx, a.release); err != nil {
		return ApprovalDecision{}, err
	}
	return ApprovalDecision{Approve: true, Reason: "finally reviewed", Source: "tty"}, nil
}

// The PLAN scenario: an approval parked for two days (FakeClock) resumes
// in place and the run completes.
func TestApprovalHangsTwoDaysThenApproved(t *testing.T) {
	root := t.TempDir()
	l := testLoop(t, approvalFixture(), root)
	l.Pipeline = askEverything
	fake := clock.NewFake(time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC))
	l.Clock = fake
	l.Approvals = slowApprover{clk: fake, release: fake.Now().Add(48 * time.Hour)}

	done := make(chan error, 1)
	var res RunResult
	go func() {
		var err error
		res, err = l.Run(context.Background(), "write a note")
		done <- err
	}()
	for fake.Waiters() == 0 {
		runtime.Gosched()
	}
	fake.Advance(48 * time.Hour)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "note.txt")); string(got) != "hello" {
		t.Fatalf("file = %q", got)
	}
}

// blockingApprover never answers — the interrupt must win.
type blockingApprover struct{}

func (blockingApprover) Resolve(ctx context.Context, _ ApprovalRequest) (ApprovalDecision, error) {
	<-ctx.Done()
	return ApprovalDecision{}, ctx.Err()
}

// Denied-by-interrupt: the approval resolves as a denial, the call renders
// "[interrupted by user]", and the LOOP CONTINUES to completion.
func TestApprovalDeniedByInterrupt(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
				"path": "note.txt", "old": "", "new": "hello"}}},
			{Finish: "tool_use"},
		}},
		{
			Expect:  scripted.Expect{LastMessageContains: "[interrupted by user]"},
			Respond: []scripted.Event{{Text: "understood, stopping"}, {Finish: "end_turn"}},
		},
	}}
	root := t.TempDir()
	l := testLoop(t, fix, root)
	l.Pipeline = askEverything
	l.Approvals = blockingApprover{}
	interrupts := make(chan struct{})
	l.Interrupts = interrupts

	done := make(chan error, 1)
	var res RunResult
	go func() {
		var err error
		res, err = l.Run(context.Background(), "write a note")
		done <- err
	}()
	close(interrupts) // user hits Ctrl-C while the approval is pending
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.Turns != 2 {
		t.Fatalf("res = %+v (interrupt must not end the run)", res)
	}
	if _, err := os.Stat(filepath.Join(root, "note.txt")); !os.IsNotExist(err) {
		t.Fatal("denied effect must not execute")
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var sawInterruptInput, sawDeniedResolution bool
	for _, e := range events {
		if e.Type == event.TypeInputReceived && strings.Contains(string(e.Payload), "interrupt") {
			sawInterruptInput = true
		}
		if e.Type == event.TypeWaitingResolved && strings.Contains(string(e.Payload), "denied_by_interrupt") {
			sawDeniedResolution = true
		}
	}
	if !sawInterruptInput || !sawDeniedResolution {
		t.Fatalf("interrupt facts missing (input=%v resolution=%v)", sawInterruptInput, sawDeniedResolution)
	}
}

// The S2 exit gate's owed scenario: killed while parked in
// WAITING_APPROVAL, resumed, approved, and the run continues in place.
func TestApprovalSurvivesCrashThenApproved(t *testing.T) {
	if os.Getenv("GO_CRASH_HELPER") == "1" {
		helperApprovalRun()
		return
	}

	base := t.TempDir()
	sessDir := filepath.Join(base, "sess")
	root := filepath.Join(base, "ws")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestApprovalSurvivesCrashThenApproved")
	cmd.Env = append(os.Environ(),
		"GO_CRASH_HELPER=1",
		"CRASH_SESS_DIR="+sessDir,
		"CRASH_WS="+root,
		crash.EnvVar+"=after:waiting_entered:1", // die parked
	)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != crash.ExitCode {
		t.Fatalf("subprocess: err = %v, out = %s", err, out)
	}

	// Resume: the parked approval is re-prompted; approve and finish.
	t.Setenv("AGENTRUNNER_APPROVE", "always")
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
		Spec:      approvalSpec(),
		Provider:  scripted.New(scripted.Fixture{Steps: approvalFixture().Steps[1:]}),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		SessionID: "apr-crash",
		Pipeline:  askEverything,
	}
	res, err := l.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.Turns != 2 {
		t.Fatalf("res = %+v", res)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "note.txt")); string(got) != "hello" {
		t.Fatalf("file = %q — approved-after-resume effect did not execute", got)
	}
}

func approvalSpec() *AgentSpec {
	return &AgentSpec{
		Name:         "asker",
		Model:        ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt: "s",
		Tools:        []string{"edit_file"},
		MaxTurns:     5,
	}
}

func helperApprovalRun() {
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
		Spec:      approvalSpec(),
		Provider:  scripted.New(approvalFixture()),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		SessionID: "apr-crash",
		Pipeline:  askEverything,
		Approvals: blockingApprover{},
	}
	_, _ = l.Run(context.Background(), "write a note")
	fmt.Println("UNREACHABLE: predicate did not fire")
	os.Exit(0)
}

// The resolution's effect on the transcript: verify via decode, not text
// matching — the denied call renders exactly "[interrupted by user]".
func TestInterruptDenialRendering(t *testing.T) {
	m := &memAppend{}
	s := stateWithPendingApproval(t, m)
	_ = s
	var resolved event.EffectResolved
	for _, e := range m.events {
		if e.Type == event.TypeEffectResolved {
			if err := json.Unmarshal(e.Payload, &resolved); err != nil {
				t.Fatal(err)
			}
		}
	}
	if resolved.Verdict != event.VerdictDeny {
		t.Fatalf("resolved = %+v", resolved)
	}
	last := resolved.GateResults[len(resolved.GateResults)-1]
	if last.Gate != "approval" || last.Reason != "[interrupted by user]" {
		t.Fatalf("approval gate result = %+v", last)
	}
}

// stateWithPendingApproval drives awaitApproval through the interrupt arm
// against an in-memory journal.
func stateWithPendingApproval(t *testing.T, m *memAppend) *driveState {
	t.Helper()
	ds := &driveState{s: state.New()}
	interrupts := make(chan struct{})
	close(interrupts)
	l := &Loop{Approvals: blockingApprover{}, Interrupts: interrupts}
	appendE := func(typ string, payload any) (event.Envelope, error) {
		env, err := m.append(typ, payload)
		if err != nil {
			return env, err
		}
		ds.s, err = state.Apply(ds.s, env)
		return env, err
	}
	req := event.ApprovalRequested{ApprovalID: "apr-eff-call_1_0", EffectID: "eff-call_1_0", CallID: "call_1_0"}
	allowed, err := l.awaitApproval(context.Background(), ds, appendE, req)
	if err != nil || allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	return ds
}
