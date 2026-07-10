package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
)

// Assemble threads a spec's output_schema into the request's ResponseSchema.
func TestAssembleSetsOutputSchema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	spec := &AgentSpec{Name: "x", Model: ModelSpec{ID: "m"}, OutputSchema: SchemaJSON(schema)}
	req := Assemble(state.New(), spec, nil, 0)
	if string(req.ResponseSchema) != string(schema) {
		t.Fatalf("ResponseSchema = %q, want the spec's output_schema", req.ResponseSchema)
	}
	// A spec without output_schema leaves it empty (default unchanged).
	req = Assemble(state.New(), &AgentSpec{Name: "y", Model: ModelSpec{ID: "m"}}, nil, 0)
	if len(req.ResponseSchema) != 0 {
		t.Errorf("ResponseSchema = %q, want empty for a schema-less spec", req.ResponseSchema)
	}
}

// schemaLoop builds a loop whose provider reports the given capabilities, so
// the INC-35 downgrade path can be exercised (reuses capsProvider from
// thinking_test.go).
func schemaLoop(t *testing.T, caps provider.Capabilities) (*Loop, *capturingProvider) {
	t.Helper()
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: `{"ok":true}`}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	cap := &capturingProvider{inner: scripted.New(fix)}
	l.Provider = &capsProvider{capturingProvider: cap, caps: caps}
	l.Spec.OutputSchema = SchemaJSON(`{"type":"object"}`)
	l.Spec.Tools = nil // a tool-less turn — the only place native mode applies
	return l, cap
}

// A provider WITHOUT StructuredOutput must never receive the schema: the loop
// clears it, so the request the provider sees is unconstrained (the CLI
// validate/retry path is the fallback, not a silent fake constraint).
func TestOutputSchemaDowngradedWhenUnsupported(t *testing.T) {
	l, cap := schemaLoop(t, provider.Capabilities{}) // StructuredOutput false
	if _, err := l.Run(context.Background(), "produce json"); err != nil {
		t.Fatal(err)
	}
	for _, r := range cap.Requests() {
		if len(r.ResponseSchema) != 0 {
			t.Fatalf("provider saw a ResponseSchema despite lacking StructuredOutput: %q", r.ResponseSchema)
		}
	}
}

// A provider WITH StructuredOutput keeps the schema on the request.
func TestOutputSchemaKeptWhenSupported(t *testing.T) {
	l, cap := schemaLoop(t, provider.Capabilities{StructuredOutput: true})
	if _, err := l.Run(context.Background(), "produce json"); err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, r := range cap.Requests() {
		if len(r.ResponseSchema) > 0 {
			saw = true
		}
	}
	if !saw {
		t.Error("provider with StructuredOutput never received the schema")
	}
}

// A spec authored with a YAML output_schema loads it as valid JSON (yaml.v3
// cannot put a !!map into json.RawMessage, so SchemaJSON bridges it).
func TestSpecLoadsYAMLOutputSchema(t *testing.T) {
	dir := t.TempDir()
	body := "name: extract\n" +
		"model: { provider: gemini, id: gemini-flash-latest }\n" +
		"system_prompt: extract\n" +
		"tools: []\n" +
		"output_schema:\n" +
		"  type: object\n" +
		"  properties:\n" +
		"    name: { type: string }\n" +
		"  required: [name]\n"
	path := filepath.Join(dir, "extract.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := LoadSpec(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !json.Valid(spec.OutputSchema) {
		t.Fatalf("output_schema did not load as valid JSON: %q", spec.OutputSchema)
	}
	var m map[string]any
	if err := json.Unmarshal(spec.OutputSchema, &m); err != nil || m["type"] != "object" {
		t.Fatalf("output_schema = %q, want the object schema", spec.OutputSchema)
	}
}

// An output_schema spec is a pure producer: the auto-added tool face
// (spawn_agent/send_message/goal/…) is suppressed so the turn is tool-less and
// native JSON mode can actually apply (INC-35). Without this, a daemon-hosted
// run always advertises send_message and the native path is unreachable.
func TestStructuredOnlySuppressesAutoTools(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: `{"ok":true}`}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	cap := &capturingProvider{inner: scripted.New(fix)}
	l.Provider = cap
	// A spec that WOULD normally advertise spawn_agent (agents_dynamic) —
	// but with output_schema it must advertise nothing.
	l.Spec.Tools = nil
	l.Spec.AgentsDynamic = true
	l.Spec.OutputSchema = SchemaJSON(`{"type":"object"}`)
	if _, err := l.Run(context.Background(), "produce json"); err != nil {
		t.Fatal(err)
	}
	for _, r := range cap.Requests() {
		if len(r.Tools) != 0 {
			var names []string
			for _, td := range r.Tools {
				names = append(names, td.Name)
			}
			t.Fatalf("output_schema spec advertised tools %v — must be tool-less for native JSON mode", names)
		}
	}
}
