package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

const forkSkillMD = `---
description: checks a deploy plan
context: fork
allowed-tools: [read_file]
---
FORK-BODY: you are the deploy checker.
`

const inlineSkillMD = `---
description: plain inline skill
---
INLINE-BODY: follow me inline.
`

func writeSkill(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// transformLoop builds the minimal Loop the transform needs: a spec (gate +
// tool face) and a workspace-rooted executor.
func transformLoop(t *testing.T, root string, dynamic bool) *Loop {
	t.Helper()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	return &Loop{
		Spec: &AgentSpec{Name: "lead", Tools: []string{"read_file", "skill"},
			Model: ModelSpec{Provider: "scripted", ID: "m"}, AgentsDynamic: dynamic},
		Exec: &tool.Executor{WS: ws},
	}
}

func skillCallTurn(name, prompt string) provider.GenStep {
	args, _ := json.Marshal(map[string]string{"name": name, "prompt": prompt})
	return provider.GenStep{
		Message: provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{
			{Kind: provider.PartText, Text: "invoking"},
			{Kind: provider.PartToolCall, CallID: "c1", ToolName: "skill", Args: args},
		}},
		ToolCalls: []provider.ToolCall{{CallID: "c1", Name: "skill", Args: args}},
	}
}

// TestForkSkillExpansion: a context:fork skill call is rewritten into
// spawn_agent{role} — message part and collected call together — while every
// non-expanding case stays byte-identical to the inline path.
func TestForkSkillExpansion(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "deploy-check", forkSkillMD)
	writeSkill(t, root, "plain", inlineSkillMD)
	l := transformLoop(t, root, true)

	turn := skillCallTurn("deploy-check", "check service X")
	l.expandForkSkills(&turn)

	part := turn.Message.Parts[1]
	if part.ToolName != "spawn_agent" || turn.ToolCalls[0].Name != "spawn_agent" {
		t.Fatalf("fork skill not expanded: part=%s call=%s", part.ToolName, turn.ToolCalls[0].Name)
	}
	if string(part.Args) != string(turn.ToolCalls[0].Args) {
		t.Fatal("message part and collected call diverged after expansion")
	}
	var spawn struct {
		Role   InlineRole `json:"role"`
		Prompt string     `json:"prompt"`
	}
	if err := json.Unmarshal(part.Args, &spawn); err != nil {
		t.Fatal(err)
	}
	if spawn.Role.Name != "deploy-check" || spawn.Role.Description != "checks a deploy plan" {
		t.Errorf("role identity = %+v", spawn.Role)
	}
	if !strings.Contains(spawn.Role.Instructions, "FORK-BODY") {
		t.Errorf("role instructions missing skill body: %q", spawn.Role.Instructions)
	}
	if len(spawn.Role.Tools) != 1 || spawn.Role.Tools[0] != "read_file" {
		t.Errorf("allowed-tools not mapped: %v", spawn.Role.Tools)
	}
	if spawn.Prompt != "check service X" {
		t.Errorf("prompt = %q", spawn.Prompt)
	}

	// Non-expanding cases: each stays an untouched skill call.
	for name, tc := range map[string]struct {
		loop *Loop
		turn provider.GenStep
	}{
		"inline skill":   {l, skillCallTurn("plain", "")},
		"unknown skill":  {l, skillCallTurn("nonexistent", "")},
		"traversal name": {l, skillCallTurn("../evil", "")},
		"gate off":       {transformLoop(t, root, false), skillCallTurn("deploy-check", "x")},
	} {
		tc.loop.expandForkSkills(&tc.turn)
		if got := tc.turn.ToolCalls[0].Name; got != "skill" {
			t.Errorf("%s: expanded to %q, want untouched skill call", name, got)
		}
	}
}

// TestForkSkillDefaultPrompt: a fork invocation without a prompt still expands,
// with the default prompt filled in (role instructions carry the skill body).
func TestForkSkillDefaultPrompt(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "deploy-check", forkSkillMD)
	l := transformLoop(t, root, true)
	turn := skillCallTurn("deploy-check", "")
	l.expandForkSkills(&turn)
	var spawn struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(turn.ToolCalls[0].Args, &spawn); err != nil {
		t.Fatal(err)
	}
	if spawn.Prompt == "" {
		t.Fatal("expanded fork spawn has an empty prompt")
	}
}

// TestForkSkillSpawnsChild is the full-chain twin (mirrors
// TestSpawnDynamicRole): the model invokes skill(name, prompt) on a fork skill;
// the journal records an ordinary dynamic-role spawn whose frozen RoleSpec
// carries the skill body as the child's system prompt, and the child runs to
// completion in its own journal.
func TestForkSkillSpawnsChild(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "deploy-check", forkSkillMD)

	parentFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "fk", Name: "skill", Args: map[string]any{
				"name": "deploy-check", "prompt": "FORK-WORK: check the release",
			}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	childFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "fork skill run complete"}, {Finish: "end_turn"}}},
	}}
	l, _ := routedSpawnLoop(t, parentFix, root,
		scripted.RoutePair{Key: "FORK-BODY", Fixture: childFix})
	l.Spec.Agents = nil
	l.Spec.AgentsDynamic = true
	l.Spec.Tools = []string{"read_file", "skill"}
	if _, err := l.Run(context.Background(), "run the deploy-check skill"); err != nil {
		t.Fatal(err)
	}

	events, _ := store.ReadEvents(l.Store.Dir())
	var spawned *event.SpawnRequested
	for _, env := range events {
		if env.Type == event.TypeSpawnRequested {
			decoded, _ := event.DecodePayload(env)
			spawned = decoded.(*event.SpawnRequested)
		}
	}
	if spawned == nil || spawned.Agent != "deploy-check" || len(spawned.RoleSpec) == 0 {
		t.Fatalf("fork skill did not spawn a dynamic role: %+v", spawned)
	}
	var frozen AgentSpec
	if err := json.Unmarshal(spawned.RoleSpec, &frozen); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(frozen.SystemPrompt, "FORK-BODY") {
		t.Fatalf("frozen role system prompt missing skill body: %q", frozen.SystemPrompt)
	}
	if len(frozen.Tools) != 1 || frozen.Tools[0] != "read_file" {
		t.Fatalf("frozen role tools = %v, want allowed-tools [read_file]", frozen.Tools)
	}

	childEvents, err := store.ReadEvents(filepath.Join(l.Store.Dir(), "sub", "fk-a1"))
	if err != nil {
		t.Fatal(err)
	}
	childFold, err := state.Fold(childEvents)
	if err != nil {
		t.Fatal(err)
	}
	if childFold.Session.SpecName != "deploy-check" {
		t.Fatalf("child did not start from the fork skill role: %+v", childFold.Session)
	}
}
