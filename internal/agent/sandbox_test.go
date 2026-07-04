package agent

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
)

// S7 模块 5 e2e: sandbox.network=none contains bash in a netns, and the
// journal's EffectResolved records the containment actually in force.
func TestSandboxNetworkNoneEndToEnd(t *testing.T) {
	if err := exec.Command("unshare", "-r", "-n", "true").Run(); err != nil {
		t.Skipf("no unprivileged netns here: %v", err)
	}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "b1", Name: "bash",
				Args: map[string]any{"command": "tail -n +3 /proc/net/dev | cut -d: -f1 | tr -d ' '"}}},
			{Finish: "tool_use"},
		}},
		{
			Expect:  scripted.Expect{LastMessageContains: "lo"},
			Respond: []scripted.Event{{Text: "contained"}, {Finish: "end_turn"}},
		},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.Sandbox.Network = "none"

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
	if resolved.Containment == nil || resolved.Containment.Network != "none" ||
		resolved.Containment.Backend != "netns" {
		t.Errorf("containment = %+v, want {none netns}", resolved.Containment)
	}

	// The command really ran inside the namespace: only loopback visible.
	fold, _ := store.ReadEvents(l.Store.Dir())
	_ = fold
	var sawLoOnly bool
	for _, e := range events {
		if e.Type == event.TypeActivityCompleted && strings.Contains(string(e.Payload), "tool-b1") {
			payload := string(e.Payload)
			if strings.Contains(payload, "lo") && !strings.Contains(payload, "eth") {
				sawLoOnly = true
			}
		}
	}
	if !sawLoOnly {
		t.Error("bash output does not show netns-only interfaces")
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
			if err := exec.Command("unshare", "-r", "-n", "true").Run(); err != nil {
				t.Skipf("no unprivileged netns here: %v", err)
			}
			l.Spec.Sandbox.Network = "none"
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
