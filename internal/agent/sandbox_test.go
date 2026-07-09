package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

func TestSandboxCapabilityMissingDeniesBeforeActivity(t *testing.T) {
	l := testLoop(t, scripted.Fixture{}, t.TempDir())
	l.Exec.ProbeSandbox = func(bool) error { return errors.New("backend disabled") }
	ds := &driveState{s: state.State{}}
	outcome, allowed, err := l.adjudicate(context.Background(), ds, l.appender(ds), pipeline.Effect{
		ID: "eff-bash", Kind: "tool_call", ToolName: "bash", Class: "execute",
	})
	if err != nil {
		t.Fatal(err)
	}
	if allowed || denyingGate(outcome) != "containment" {
		t.Fatalf("outcome = %+v allowed=%v", outcome, allowed)
	}
	for _, env := range readEvents(t, l.Store.Dir()) {
		if env.Type == event.TypeActivityStarted {
			t.Fatal("activity started without an OS sandbox")
		}
	}
}

// INC-11.3 e2e: sandbox.network=none contains bash in the platform OS sandbox, and the
// journal's EffectResolved records the containment actually in force.
func TestSandboxNetworkNoneEndToEnd(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "b1", Name: "bash",
				Args: map[string]any{"command": "echo contained"}}},
			{Finish: "tool_use"},
		}},
		{
			Expect:  scripted.Expect{LastMessageContains: "contained"},
			Respond: []scripted.Event{{Text: "contained"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.Sandbox.Network = "none"
	l.applySandbox()
	if _, err := l.Exec.SandboxInfo(); err != nil {
		t.Skipf("no OS sandbox backend here: %v", err)
	}

	res, err := l.Run(context.Background(), "check the interfaces")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}
	if !l.Exec.NetworkContained() {
		t.Error("executor not ratcheted")
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var resolved *event.EffectResolved
	for _, e := range events {
		if e.Type != event.TypeEffectResolved {
			continue
		}
		dec, _ := event.DecodePayload(e)
		if er := dec.(*event.EffectResolved); er.CallID == "b1" {
			resolved = er
		}
	}
	if resolved == nil {
		t.Fatal("no EffectResolved for the bash call")
	}
	if resolved.Containment == nil || resolved.Containment.Filesystem != "workspace" ||
		resolved.Containment.Network != "none" || resolved.Containment.Backend == "" {
		t.Errorf("containment = %+v, want workspace + network none", resolved.Containment)
	}

	// The command really ran behind the recorded boundary.
	fold, _ := store.ReadEvents(l.Store.Dir())
	_ = fold
	var sawContained bool
	for _, e := range events {
		if e.Type == event.TypeActivityCompleted && strings.Contains(string(e.Payload), "tool-b1") {
			payload := string(e.Payload)
			if strings.Contains(payload, "contained") {
				sawContained = true
			}
		}
	}
	if !sawContained {
		t.Error("bash did not complete behind the sandbox")
	}
}

// A network rule gates only executions that WOULD have egress: the same
// rule set denies uncontained bash and lets contained bash through.
func TestNetworkRuleDeniesOnlyUncontained(t *testing.T) {
	rules := []pipeline.PermissionRule{
		{Tool: "bash", Network: "*", Action: "deny"},
		{Action: "allow"},
	}
	runOnce := func(t *testing.T, contain bool) (string, bool) {
		fix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "b1", Name: "bash",
					Args: map[string]any{"command": "echo ran"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
		}}
		l := testLoop(t, fix, t.TempDir())
		l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{
			&pipeline.PermissionGate{Rules: rules, WS: l.Exec.WS}}}
		if contain {
			l.Spec.Sandbox.Network = "none"
			l.applySandbox()
			if _, err := l.Exec.SandboxInfo(); err != nil {
				t.Skipf("no OS sandbox backend here: %v", err)
			}
		}
		if _, err := l.Run(context.Background(), "go"); err != nil {
			t.Fatal(err)
		}
		events, err := store.ReadEvents(l.Store.Dir())
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events {
			if e.Type == event.TypeEffectResolved && strings.Contains(string(e.Payload), "eff-tool-b1") {
				dec, _ := event.DecodePayload(e)
				return dec.(*event.EffectResolved).Verdict, true
			}
		}
		return "", false
	}

	if verdict, ok := runOnce(t, false); !ok || verdict != event.VerdictDeny {
		t.Errorf("uncontained bash verdict = %q, want deny by network rule", verdict)
	}
	if verdict, ok := runOnce(t, true); !ok || verdict != event.VerdictAllow {
		t.Errorf("contained bash verdict = %q, want allow (rule does not fire)", verdict)
	}
}

// S7/INC-12.5 security review: MCP tools execute out-of-process — the
// subprocess sandbox never bounds them, so under the ratchet they keep
// egress scope "all", carry no false containment claim, and are hard-denied
// before activity. A child escalation cannot punch through network:none.
func TestMCPToolsStayOutsideContainment(t *testing.T) {
	l := testLoop(t, scripted.Fixture{}, t.TempDir())
	l.Spec.Sandbox.Network = "none"
	l.applySandbox()
	if !l.Exec.NetworkContained() {
		t.Fatal("ratchet not applied")
	}
	if got := l.networkScope("execute", "mcp__srv__deploy"); got != "all" {
		t.Errorf("mcp scope under ratchet = %q, want all (out-of-process egress)", got)
	}
	if got := l.networkScope("execute", "bash"); got != "" {
		t.Errorf("bash scope under ratchet = %q, want contained", got)
	}
	if c := l.containment(pipeline.Effect{Kind: "tool_call", Class: "execute",
		ToolName: "mcp__srv__deploy"}); c != nil {
		t.Errorf("mcp containment = %+v, want nil (journal must not over-claim)", c)
	}
	if c := l.containment(pipeline.Effect{Kind: "tool_call", Class: "execute",
		ToolName: "bash"}); c == nil || c.Filesystem != "workspace" || c.Backend == "" {
		t.Errorf("bash containment = %+v, want workspace OS sandbox", c)
	}
	ds := &driveState{s: state.New()}
	outcome, allowed, err := l.adjudicate(context.Background(), ds, l.appender(ds), pipeline.Effect{
		ID: "eff-mcp-contained", Kind: "tool_call", ToolName: "mcp__srv__deploy",
		Class: "execute", Network: "all",
	})
	if err != nil {
		t.Fatal(err)
	}
	if allowed || denyingGate(outcome) != "containment" {
		t.Fatalf("contained MCP outcome = %+v allowed=%v", outcome, allowed)
	}
}

// INC-5: web_fetch declares egress as def DATA (network: "all"). It is
// execute-class (review M1) but the egress comes from the data slot, not the
// class — scope is "all" uncontained, empties under the ratchet (executor
// fails closed), and containment is nil either way because self-refusal is
// NOT a netns and the journal must not over-claim it.
func TestWebFetchNetworkScope(t *testing.T) {
	l := testLoop(t, scripted.Fixture{}, t.TempDir())
	if got := l.networkScope("execute", "web_fetch"); got != "all" {
		t.Errorf("web_fetch scope = %q, want all (def network slot)", got)
	}
	if got := l.networkScope("read", "read_file"); got != "" {
		t.Errorf("read_file scope = %q, want empty", got)
	}
	if c := l.containment(pipeline.Effect{Kind: "tool_call", Class: "execute",
		ToolName: "web_fetch"}); c != nil {
		t.Errorf("web_fetch containment (uncontained) = %+v, want nil", c)
	}
	l.Spec.Sandbox.Network = "none"
	l.applySandbox()
	if got := l.networkScope("execute", "web_fetch"); got != "" {
		t.Errorf("web_fetch scope under ratchet = %q, want empty (fail closed)", got)
	}
	if c := l.containment(pipeline.Effect{Kind: "tool_call", Class: "execute",
		ToolName: "web_fetch"}); c != nil {
		t.Errorf("web_fetch containment under ratchet = %+v, want nil (self-refusal, not netns)", c)
	}
}
