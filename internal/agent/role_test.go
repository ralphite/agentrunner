package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
)

// INC-12.4 (工作纸闸门 A-5): a lead with agents_dynamic (and NO static
// whitelist) drafts a role inline; the child runs under the constructed
// spec, the parent journal freezes it (SpawnRequested.RoleSpec), and a later
// message REVIVES the role from its own journaled spec — dynamic members are
// as durable as static ones.
func TestSpawnDynamicRoleAndRevive(t *testing.T) {
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "pm", Name: "spawn_agent",
					Args: map[string]any{
						"role": map[string]any{
							"name":         "pm",
							"description":  "product manager",
							"instructions": "you are the PM; write crisp acceptance criteria",
						},
						"task": "draft acceptance criteria for project OMEGA",
					}}},
				{Finish: "tool_use"},
			}},
			// Receipt 1 → follow up by handle (revive path).
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "m1", Name: "send_message",
					Args: map[string]any{"to": "pm", "text": "add a rollback criterion"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "OMEGA", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "criteria v1: ship works"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "criteria v2: with rollback"}, {Finish: "end_turn"}}},
		}}},
	)
	l := bgSpawnLoop(t, router, nil) // NO static whitelist
	l.Spec.AgentsDynamic = true
	inputs := make(chan protocol.UserInput)
	l.UserInputs = inputs
	closeWhen(l, inputs, func(evs []event.Envelope) bool {
		return countType(evs, event.TypeSubagentCompleted) >= 2
	})

	if _, err := l.Run(context.Background(), "orchestrate project OMEGA with a drafted team"); err != nil {
		t.Fatal(err)
	}

	evs, _ := store.ReadEvents(l.Store.Dir())
	// The dynamic role's constructed spec is frozen in the parent journal.
	var sawRoleSpec bool
	for _, e := range evs {
		if e.Type == event.TypeSpawnRequested {
			dec, _ := event.DecodePayload(e)
			sr := dec.(*event.SpawnRequested)
			if sr.Agent != "pm" {
				t.Errorf("spawned agent = %q, want pm", sr.Agent)
			}
			if strings.Contains(string(sr.RoleSpec), "acceptance criteria") ||
				strings.Contains(string(sr.RoleSpec), `"pm"`) {
				sawRoleSpec = true
			}
		}
	}
	if !sawRoleSpec {
		t.Fatal("SpawnRequested lacks the frozen RoleSpec")
	}
	if n := countType(evs, event.TypeChildRevived); n != 1 {
		t.Fatalf("ChildRevived = %d, want 1 (dynamic role revived from its journaled spec)", n)
	}
	if n := countType(evs, event.TypeSubagentCompleted); n != 2 {
		t.Fatalf("SubagentCompleted = %d, want 2", n)
	}
	// The child journal froze the constructed spec (SessionStarted.Spec) and
	// continued in ONE session across the revive.
	childEvs, err := store.ReadEvents(l.Store.Dir() + "/sub/pm-a1")
	if err != nil {
		t.Fatal(err)
	}
	if n := countType(childEvs, event.TypeSessionStarted); n != 1 {
		t.Fatalf("child SessionStarted = %d, want 1", n)
	}
	spec, err := childSpecFromJournal(l.Store.Dir() + "/sub/pm-a1")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "pm" || !strings.Contains(spec.SystemPrompt, "acceptance criteria") {
		t.Errorf("journaled dynamic spec = %+v", spec)
	}
	if len(spec.Tools) == 0 {
		t.Error("dynamic role inherited no tools")
	}
}

// INC-12.4: role guards — inline roles need the dynamic face; agent and role
// are mutually exclusive; a role tool outside the parent face is refused.
// All model-visible errors, never harness failures.
func TestSpawnRoleGuards(t *testing.T) {
	router := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "r1", Name: "spawn_agent",
					Args: map[string]any{
						"role": map[string]any{"name": "x", "instructions": "work"},
						"task": "t"}}},
				{ToolCall: &scripted.ToolCallEvent{CallID: "r2", Name: "spawn_agent",
					Args: map[string]any{"agent": "worker",
						"role": map[string]any{"name": "y", "instructions": "work"},
						"task": "t"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "saw the errors"}, {Finish: "end_turn"}}},
		}}},
	)
	// Static whitelist WITHOUT the dynamic face.
	l := bgSpawnLoop(t, router, []string{"worker"})
	if _, err := l.Run(context.Background(), "try bad role spawns"); err != nil {
		t.Fatal(err)
	}
	evs, _ := store.ReadEvents(l.Store.Dir())
	if n := countType(evs, event.TypeSpawnRequested); n != 0 {
		t.Fatalf("SpawnRequested = %d, want 0 (all guarded)", n)
	}
	var sawNotEnabled, sawExclusive bool
	for _, e := range evs {
		if e.Type != event.TypeActivityCompleted {
			continue
		}
		p := string(e.Payload)
		if strings.Contains(p, "disabled") {
			sawNotEnabled = true
		}
		if strings.Contains(p, "exactly one of") {
			sawExclusive = true
		}
	}
	if !sawNotEnabled || !sawExclusive {
		t.Errorf("guards missing: disabled=%v exclusive=%v", sawNotEnabled, sawExclusive)
	}

	// A dynamic-face lead still cannot hand a role tools beyond its own face.
	router2 := scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "r3", Name: "spawn_agent",
					Args: map[string]any{
						"role": map[string]any{"name": "z", "description": "d", "instructions": "work",
							"tools": []string{"bash"}}, // parent face: read_file only
						"task": "t"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "saw it"}, {Finish: "end_turn"}}},
		}}},
	)
	l2 := bgSpawnLoop(t, router2, nil)
	l2.Spec.AgentsDynamic = true
	if _, err := l2.Run(context.Background(), "try oversized role tools"); err != nil {
		t.Fatal(err)
	}
	evs2, _ := store.ReadEvents(l2.Store.Dir())
	var sawOutside bool
	for _, e := range evs2 {
		if e.Type == event.TypeActivityCompleted && strings.Contains(string(e.Payload), "not in the parent's tool face") {
			sawOutside = true
		}
	}
	if !sawOutside {
		t.Error("oversized role tool face was not refused")
	}
}

// approveOnce / denyAll are escalation-approval stubs.
type approveOnce struct{}

func (approveOnce) Resolve(context.Context, ApprovalRequest) (ApprovalDecision, error) {
	return ApprovalDecision{Approve: true, Reason: "user granted the escalation", Source: "tty"}, nil
}

type denyAll struct{}

func (denyAll) Resolve(context.Context, ApprovalRequest) (ApprovalDecision, error) {
	return ApprovalDecision{Approve: false, Reason: "no write access for drafted members", Source: "tty"}, nil
}

// escalationLoop builds a lead whose OWN rules deny write_file; the drafted
// role requests write_file via escalate.
func escalationLoop(t *testing.T, router *scripted.Router) *Loop {
	t.Helper()
	l := bgSpawnLoop(t, router, nil)
	l.Spec.AgentsDynamic = true
	l.Spec.Tools = []string{"read_file", "write_file"}
	l.Pipeline = &pipeline.Pipeline{Gates: []pipeline.Gate{&pipeline.PermissionGate{
		Rules: []pipeline.PermissionRule{
			{Tool: "write_file", Action: "deny"},
			{Action: "allow"},
		},
	}}}
	return l
}

func escalationRouter() *scripted.Router {
	return scripted.NewRouter(
		scripted.RoutePair{Key: "you orchestrate", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "swe", Name: "spawn_agent",
					Args: map[string]any{
						"role": map[string]any{
							"name": "swe", "description": "software engineer",
							"instructions": "you implement; write files as needed",
							"escalate":     true,
							"permissions": []map[string]any{
								{"tool": "write_file", "action": "allow"},
								{"action": "allow"},
							},
						},
						"task": "implement WIDGET: write widget.txt"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "receipt seen"}, {Finish: "end_turn"}}},
			{Respond: []scripted.Event{{Text: "ack"}, {Finish: "end_turn"}}},
		}}},
		scripted.RoutePair{Key: "WIDGET", Fixture: scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "w1", Name: "write_file",
					Args: map[string]any{"path": "widget.txt", "content": "made by swe"}}},
				{Finish: "tool_use"},
			}},
			{Respond: []scripted.Event{{Text: "WIDGET written"}, {Finish: "end_turn"}}},
		}}},
	)
}

// INC-12.5 (工作纸闸门 A-6, 决策 #20 修订): an APPROVED escalation replaces
// the frozen intersection — the drafted member writes a file its parent may
// not touch; the approval fact and the escalation gate result are journaled.
func TestEscalationApproved(t *testing.T) {
	l := escalationLoop(t, escalationRouter())
	l.Approvals = approveOnce{}
	inputs := make(chan protocol.UserInput)
	l.UserInputs = inputs
	closeWhen(l, inputs, func(evs []event.Envelope) bool {
		return countType(evs, event.TypeSubagentCompleted) >= 1
	})
	if _, err := l.Run(context.Background(), "build WIDGET with a drafted swe"); err != nil {
		t.Fatal(err)
	}
	evs, _ := store.ReadEvents(l.Store.Dir())
	var sawEscalationAsk, sawApproved, escalatedSpawn bool
	for _, e := range evs {
		switch e.Type {
		case event.TypeApprovalRequested:
			if strings.Contains(string(e.Payload), "escalation") || strings.Contains(string(e.Payload), "BEYOND") {
				sawEscalationAsk = true
			}
		case event.TypeApprovalResponded:
			if strings.Contains(string(e.Payload), `"approve"`) {
				sawApproved = true
			}
		case event.TypeSpawnRequested:
			dec, _ := event.DecodePayload(e)
			if dec.(*event.SpawnRequested).Escalated {
				escalatedSpawn = true
			}
		}
	}
	if !sawEscalationAsk || !sawApproved || !escalatedSpawn {
		t.Fatalf("escalation flow incomplete: ask=%v approved=%v spawnFact=%v",
			sawEscalationAsk, sawApproved, escalatedSpawn)
	}
	// The member ACTUALLY wrote the file the parent's own rules deny.
	if _, err := os.Stat(filepath.Join(l.Exec.WS.Root(), "widget.txt")); err != nil {
		t.Fatalf("escalated member could not write: %v", err)
	}
	// The child journal shows its write_file was ALLOWED by its own gate.
	childEvs, _ := store.ReadEvents(l.Store.Dir() + "/sub/swe-a1")
	var childWriteAllowed bool
	for _, e := range childEvs {
		if e.Type == event.TypeEffectResolved && strings.Contains(string(e.Payload), `"allow"`) &&
			strings.Contains(string(e.Payload), "tool-w1") {
			childWriteAllowed = true
		}
	}
	if !childWriteAllowed {
		t.Error("child write_file was not allow-resolved under the escalated gates")
	}
}

// INC-12.5: a DENIED escalation never spawns — the refusal (with the user's
// reason) is the model-visible result; the parent's intersection stands.
func TestEscalationDenied(t *testing.T) {
	l := escalationLoop(t, escalationRouter())
	l.Approvals = denyAll{}
	if _, err := l.Run(context.Background(), "build WIDGET with a drafted swe"); err != nil {
		t.Fatal(err)
	}
	evs, _ := store.ReadEvents(l.Store.Dir())
	if n := countType(evs, event.TypeSpawnRequested); n != 0 {
		t.Fatalf("SpawnRequested = %d, want 0 (denied escalation must not spawn)", n)
	}
	var deniedVisible bool
	for _, e := range evs {
		if e.Type == event.TypeEffectResolved && strings.Contains(string(e.Payload), "no write access") {
			deniedVisible = true
		}
	}
	if !deniedVisible {
		t.Error("denial reason did not reach the journal / model")
	}
	if _, err := os.Stat(filepath.Join(l.Exec.WS.Root(), "widget.txt")); err == nil {
		t.Error("file written despite denied escalation")
	}
	_ = time.Now // keep imports symmetric with the sibling tests
}
