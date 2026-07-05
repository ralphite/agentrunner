package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/mcp"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// fakeMCP implements MCPManager with the same allowed-narrowing semantics as
// mcp.Manager, without live transports.
type fakeMCP struct {
	tools   []mcp.DiscoveredTool
	allowed map[string]bool
	calls   []string
}

func (f *fakeMCP) SetAllowed(names []string) {
	if len(names) == 0 {
		f.allowed = nil
		return
	}
	f.allowed = map[string]bool{}
	for _, n := range names {
		f.allowed[n] = true
	}
}

func (f *fakeMCP) isAllowed(name string) bool { return f.allowed == nil || f.allowed[name] }

func (f *fakeMCP) Discover(context.Context) ([]mcp.DiscoveredTool, error) {
	var out []mcp.DiscoveredTool
	for _, t := range f.tools {
		if f.isAllowed(t.Name) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeMCP) Call(_ context.Context, qualified string, _ json.RawMessage) (json.RawMessage, bool, error) {
	if !f.isAllowed(qualified) {
		return nil, false, fmt.Errorf("mcp: tool %q not permitted", qualified)
	}
	f.calls = append(f.calls, qualified)
	return json.RawMessage(`{"content":"peeked"}`), false, nil
}

func demoTools() []mcp.DiscoveredTool {
	return []mcp.DiscoveredTool{
		{Server: "demo", Tool: "peek", Name: "mcp__demo__peek", Description: "read-only peek",
			Class: "read", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Server: "demo", Tool: "run", Name: "mcp__demo__run", Description: "untagged",
			Class: "execute", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
}

func mcpLoop(t *testing.T, fix scripted.Fixture, face *fakeMCP) (*Loop, *capturingProvider) {
	t.Helper()
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })
	cap := &capturingProvider{inner: scripted.New(fix)}
	return &Loop{
		Spec: &AgentSpec{
			Name:               "mcp-test",
			Model:              ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			Tools:              []string{"read_file"},
			MaxGenerationSteps: 5,
		},
		Provider:  cap,
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		Clock:     clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)),
		SessionID: "mcp-sess",
		MCP:       face,
	}, cap
}

// S5.1 e2e: discovery is journaled, the MCP tool is advertised alongside
// built-ins, a model call dispatches to the MCP face, and the result enters
// the fold like any tool result.
func TestMCPToolEndToEnd(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "m1", Name: "mcp__demo__peek",
				Args: map[string]any{}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	face := &fakeMCP{tools: demoTools()}
	l, cap := mcpLoop(t, fix, face)

	res, err := l.Run(context.Background(), "peek please")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}
	if len(face.calls) != 1 || face.calls[0] != "mcp__demo__peek" {
		t.Errorf("mcp calls = %v", face.calls)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var discovered *event.ToolsDiscovered
	for _, e := range events {
		if e.Type == event.TypeToolsDiscovered {
			dec, derr := event.DecodePayload(e)
			if derr != nil {
				t.Fatal(derr)
			}
			discovered = dec.(*event.ToolsDiscovered)
		}
	}
	if discovered == nil || discovered.Server != "demo" || len(discovered.Tools) != 2 {
		t.Fatalf("tools_discovered = %+v", discovered)
	}

	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	tr, ok := fold.Conversation.ToolResults["m1"]
	if !ok || tr.IsError || !strings.Contains(string(tr.Result), "peeked") {
		t.Errorf("tool result = %+v", tr)
	}

	// Both requests advertised the MCP tools next to the built-in.
	var names []string
	for _, td := range cap.requests[0].Tools {
		names = append(names, td.Name)
	}
	joined := strings.Join(names, ",")
	for _, want := range []string{"read_file", "mcp__demo__peek", "mcp__demo__run"} {
		if !strings.Contains(joined, want) {
			t.Errorf("advertised face missing %s: %v", want, names)
		}
	}
}

// S5.1 negative: allowed_tools narrows the face — the excluded tool is not
// advertised, not journaled, and a fabricated call to it fails (defense in
// depth at the manager) as a model-visible error while the run continues.
func TestMCPAllowedToolsNarrowing(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "m1", Name: "mcp__demo__run",
				Args: map[string]any{}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "understood"}, {Finish: "end_turn"}}},
	}}
	face := &fakeMCP{tools: demoTools()}
	l, cap := mcpLoop(t, fix, face)
	l.Spec.AllowedTools = []string{"mcp__demo__peek"}

	res, err := l.Run(context.Background(), "try the excluded tool")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v (a rejected MCP call is model-visible, not fatal)", res)
	}
	if len(face.calls) != 0 {
		t.Errorf("narrowed-out tool must never execute: %v", face.calls)
	}

	// Not advertised…
	for _, td := range cap.requests[0].Tools {
		if td.Name == "mcp__demo__run" {
			t.Errorf("excluded tool advertised: %v", td)
		}
	}
	// …and the fabricated call resolved as an error result in the fold.
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	tr, ok := fold.Conversation.ToolResults["m1"]
	if !ok || !tr.IsError {
		t.Errorf("fabricated call result = %+v, want model-visible error", tr)
	}
	if len(fold.Session.MCPTools) != 1 || fold.Session.MCPTools[0].Name != "mcp__demo__peek" {
		t.Errorf("journaled face = %+v, want only peek", fold.Session.MCPTools)
	}
}

// S5.1: plan mode hides the execute-class MCP tool from the advertised face
// while the read-class one stays visible (same mode filter as built-ins).
func TestMCPAdvertisedFaceRespectsMode(t *testing.T) {
	s := state.New()
	s = mustApply(t, s, event.TypeSessionStarted, &event.SessionStarted{SubStateVersions: state.SubStateVersions()})
	s = mustApply(t, s, event.TypeToolsDiscovered, &event.ToolsDiscovered{
		Server: "demo", Tools: []event.MCPToolDef{
			{Server: "demo", Name: "mcp__demo__peek", Class: "read"},
			{Server: "demo", Name: "mcp__demo__run", Class: "execute"},
		}})
	s = mustApply(t, s, event.TypeModeChanged, &event.ModeChanged{To: pipeline.ModePlan, Cause: "test"})

	defs := []provider.ToolDef{{Name: "mcp__demo__peek"}, {Name: "mcp__demo__run"}}
	out := advertisedTools(s, defs, s.CurrentMode())
	var names []string
	for _, d := range out {
		names = append(names, d.Name)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "mcp__demo__peek") {
		t.Errorf("read-class MCP tool should stay advertised in plan mode: %v", names)
	}
	if strings.Contains(joined, "mcp__demo__run") {
		t.Errorf("execute-class MCP tool must be hidden in plan mode: %v", names)
	}
}

// S5.1 resume: the journaled face is the truth — a live server whose schema
// drifted, a missing tool, or no manager at all refuses the resume.
func TestMCPResumeReconcile(t *testing.T) {
	journal := func(t *testing.T) *store.EventStore {
		t.Helper()
		es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = es.Close() })
		for _, e := range []struct {
			typ     string
			payload any
		}{
			{event.TypeSessionStarted, &event.SessionStarted{SpecName: "mcp-test", Task: "go",
				SubStateVersions: state.SubStateVersions()}},
			{event.TypeInputReceived, &event.InputReceived{Text: "go", Source: "cli"}},
			{event.TypeToolsDiscovered, &event.ToolsDiscovered{Server: "demo",
				Tools: []event.MCPToolDef{{Server: "demo", Name: "mcp__demo__peek",
					Class: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}}}},
		} {
			env, err := event.New(e.typ, e.payload)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := es.Append(env); err != nil {
				t.Fatal(err)
			}
		}
		return es
	}

	// face is the interface, not *fakeMCP: a typed-nil pointer would make
	// l.MCP != nil and dodge the no-manager branch.
	resume := func(t *testing.T, face MCPManager) error {
		t.Helper()
		es := journal(t)
		ws, err := workspace.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		fix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "hello"}, {Finish: "end_turn"}}},
		}}
		l := &Loop{
			Spec: &AgentSpec{Name: "mcp-test",
				Model: ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100}, MaxGenerationSteps: 3},
			Provider:  scripted.New(fix),
			Exec:      &tool.Executor{WS: ws},
			Store:     es,
			Clock:     clock.NewFake(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)),
			SessionID: "mcp-sess",
			MCP:       face,
		}
		_, err = l.Resume(context.Background())
		return err
	}

	t.Run("matching face resumes", func(t *testing.T) {
		face := &fakeMCP{tools: []mcp.DiscoveredTool{{Server: "demo", Tool: "peek",
			Name: "mcp__demo__peek", Class: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}}}
		if err := resume(t, face); err != nil {
			t.Fatalf("matching face must resume: %v", err)
		}
	})
	t.Run("schema drift refused", func(t *testing.T) {
		face := &fakeMCP{tools: []mcp.DiscoveredTool{{Server: "demo", Tool: "peek",
			Name: "mcp__demo__peek", Class: "read", InputSchema: json.RawMessage(`{"type":"object","properties":{"x":{}}}`)}}}
		err := resume(t, face)
		if err == nil || !strings.Contains(err.Error(), "schema drifted") {
			t.Fatalf("err = %v, want schema drift refusal", err)
		}
	})
	t.Run("missing tool refused", func(t *testing.T) {
		err := resume(t, &fakeMCP{})
		if err == nil || !strings.Contains(err.Error(), "not offered") {
			t.Fatalf("err = %v, want missing-tool refusal", err)
		}
	})
	t.Run("no manager refused", func(t *testing.T) {
		err := resume(t, nil)
		if err == nil || !strings.Contains(err.Error(), "no MCP servers") {
			t.Fatalf("err = %v, want no-manager refusal", err)
		}
	})
}
