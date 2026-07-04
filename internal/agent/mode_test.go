package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// 3.6a: the tool-face filter table, every mode × class.
func TestAdvertisedToolsByMode(t *testing.T) {
	defs := []provider.ToolDef{
		{Name: "read_file"}, {Name: "edit_file"}, {Name: "bash"}, {Name: "exit_plan_mode"},
	}
	names := func(ds []provider.ToolDef) []string {
		var out []string
		for _, d := range ds {
			out = append(out, d.Name)
		}
		return out
	}
	cases := []struct {
		mode string
		want []string
	}{
		{pipeline.ModePlan, []string{"read_file", "exit_plan_mode"}},
		{pipeline.ModeDefault, []string{"read_file", "edit_file", "bash", "exit_plan_mode"}},
		{pipeline.ModeAcceptEdits, []string{"read_file", "edit_file", "bash", "exit_plan_mode"}},
		{pipeline.ModeBypass, []string{"read_file", "edit_file", "bash", "exit_plan_mode"}},
	}
	for _, tc := range cases {
		if got := names(advertisedTools(state.New(), defs, tc.mode)); !equal(got, tc.want) {
			t.Errorf("%s: advertised = %v, want %v", tc.mode, got, tc.want)
		}
	}
}

// 3.6c: the transition rule table.
func TestModeTransitionTable(t *testing.T) {
	allowed := [][2]string{
		{pipeline.ModePlan, pipeline.ModeDefault},
		{pipeline.ModeDefault, pipeline.ModeAcceptEdits},
		{pipeline.ModeAcceptEdits, pipeline.ModeDefault},
	}
	denied := [][2]string{
		{pipeline.ModeDefault, pipeline.ModePlan},
		{pipeline.ModeDefault, pipeline.ModeBypass},
		{pipeline.ModePlan, pipeline.ModeBypass},
		{pipeline.ModeBypass, pipeline.ModeDefault},
	}
	for _, tr := range allowed {
		if !pipeline.ValidTransition(tr[0], tr[1]) {
			t.Errorf("%s → %s must be allowed", tr[0], tr[1])
		}
	}
	for _, tr := range denied {
		if pipeline.ValidTransition(tr[0], tr[1]) {
			t.Errorf("%s → %s must be denied", tr[0], tr[1])
		}
	}
}

// 3.6b + 3.6c integration: a plan-mode run sees the filtered face and the
// injected prompt; an approved exit_plan_mode switches to default and the
// next turn sees the full face without the suffix.
func TestPlanModeFullFlow(t *testing.T) {
	t.Setenv("AGENTRUNNER_APPROVE", "always")
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{Text: "plan ready"},
			{ToolCall: &scripted.ToolCallEvent{Name: "exit_plan_mode",
				Args: map[string]any{"plan": "edit greet.txt"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}

	inner := scripted.New(fix)
	cap := &capturingProvider{inner: inner}
	l := testLoop(t, fix, t.TempDir())
	l.Provider = cap
	l.Spec.Tools = []string{"read_file", "edit_file", "bash", "exit_plan_mode"}
	l.Mode = pipeline.ModePlan
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.PermissionGate{}, // mode flows in via the effect
	}}

	res, err := l.Run(context.Background(), "figure out a plan")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.Turns != 2 {
		t.Fatalf("res = %+v", res)
	}

	// Turn 1 (plan): filtered face + injected suffix.
	req1 := cap.requests[0]
	if !strings.Contains(req1.System, "PLAN MODE") {
		t.Errorf("turn 1 system prompt missing plan suffix: %q", req1.System)
	}
	for _, td := range req1.Tools {
		if td.Name == "edit_file" || td.Name == "bash" {
			t.Errorf("turn 1 advertises %s in plan mode", td.Name)
		}
	}

	// Turn 2 (default after approved transition): full face, no suffix.
	req2 := cap.requests[1]
	if strings.Contains(req2.System, "PLAN MODE") {
		t.Errorf("turn 2 still carries plan suffix")
	}
	var sawEdit bool
	for _, td := range req2.Tools {
		if td.Name == "edit_file" {
			sawEdit = true
		}
	}
	if !sawEdit {
		t.Errorf("turn 2 face still filtered: %+v", req2.Tools)
	}

	// The transition is durable and ATOMIC: it is folded from
	// exit_plan_mode's OWN completion (no separate mode_changed event that
	// could be lost in a crash between the two). Fold the log independently
	// and confirm the mode landed at default.
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	final, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if final.CurrentMode() != "default" {
		t.Fatalf("mode after exit_plan_mode = %q, want default", final.CurrentMode())
	}
}

// Denied exit_plan_mode keeps the run in plan mode.
func TestExitPlanModeDeniedStaysInPlan(t *testing.T) {
	t.Setenv("AGENTRUNNER_APPROVE", "never")
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "exit_plan_mode", Args: map[string]any{"plan": "x"}}},
			{Finish: "tool_use"},
		}},
		{
			Expect:  scripted.Expect{LastMessageContains: "denied"},
			Respond: []scripted.Event{{Text: "staying in plan"}, {Finish: "end_turn"}},
		},
	}}
	cap := &capturingProvider{inner: scripted.New(fix)}
	l := testLoop(t, fix, t.TempDir())
	l.Provider = cap
	l.Spec.Tools = []string{"read_file", "exit_plan_mode"}
	l.Mode = pipeline.ModePlan
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{&pipeline.PermissionGate{}}}

	if _, err := l.Run(context.Background(), "plan"); err != nil {
		t.Fatal(err)
	}
	// Turn 2 still in plan mode: suffix present.
	if !strings.Contains(cap.requests[1].System, "PLAN MODE") {
		t.Fatal("denied exit must keep plan mode")
	}
}

// 3.6d: bypass skips permission restrictions but hooks STILL run.
func TestBypassRunsHooksButSkipsPermission(t *testing.T) {
	hookRan := false
	recordingHook := policyGate{name: "hooks", check: func(pipeline.Effect) pipeline.Decision {
		hookRan = true
		return pipeline.Allow
	}}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
				"path": "a.txt", "old": "", "new": "x"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "ok"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Mode = pipeline.ModeBypass
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
		recordingHook,
		&pipeline.PermissionGate{}, // default mode would ask for edit; bypass allows
	}}

	res, err := l.Run(context.Background(), "edit without asking")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}
	if !hookRan {
		t.Fatal("bypass must NOT skip hooks")
	}
}
