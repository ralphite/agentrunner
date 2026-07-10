package gemini

import (
	"encoding/json"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
)

var personSchema = json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"lines":{"type":"integer"}},"required":["name","lines"]}`)

// A tool-less turn with a ResponseSchema maps to Gemini's native JSON mode:
// responseMimeType=application/json + responseJsonSchema.
func TestToConfigResponseSchemaNoTools(t *testing.T) {
	cfg, err := toConfig(provider.CompleteRequest{
		Model: "gemini-flash-latest", MaxTokens: 512, ResponseSchema: personSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ResponseMIMEType != "application/json" {
		t.Errorf("ResponseMIMEType = %q, want application/json", cfg.ResponseMIMEType)
	}
	if cfg.ResponseJsonSchema == nil {
		t.Fatal("ResponseJsonSchema not set")
	}
	obj, ok := cfg.ResponseJsonSchema.(map[string]any)
	if !ok || obj["type"] != "object" {
		t.Fatalf("ResponseJsonSchema = %#v, want the unmarshaled schema", cfg.ResponseJsonSchema)
	}
}

// JSON mode and tool calls are mutually exclusive: a turn that offers tools
// must IGNORE the schema so the tool turn is not broken.
func TestToConfigSchemaIgnoredWithTools(t *testing.T) {
	cfg, err := toConfig(provider.CompleteRequest{
		Model: "gemini-flash-latest", MaxTokens: 512, ResponseSchema: personSchema,
		Tools: []provider.ToolDef{{Name: "read_file", Description: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ResponseMIMEType != "" || cfg.ResponseJsonSchema != nil {
		t.Errorf("schema applied on a tool turn (mime=%q schema=%v) — JSON mode must yield to tools",
			cfg.ResponseMIMEType, cfg.ResponseJsonSchema)
	}
	if len(cfg.Tools) == 0 {
		t.Error("tools dropped")
	}
}

// No schema → no JSON mode (default behavior unchanged).
func TestToConfigNoSchema(t *testing.T) {
	cfg, err := toConfig(provider.CompleteRequest{Model: "gemini-flash-latest", MaxTokens: 512})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ResponseMIMEType != "" || cfg.ResponseJsonSchema != nil {
		t.Error("JSON mode set without a ResponseSchema")
	}
}

// A malformed schema is a call-time error, not a silent drop.
func TestToConfigBadSchema(t *testing.T) {
	if _, err := toConfig(provider.CompleteRequest{
		Model: "gemini-flash-latest", MaxTokens: 512, ResponseSchema: json.RawMessage(`{not json`),
	}); err == nil {
		t.Error("malformed response schema accepted")
	}
}

func TestGeminiCapabilitiesStructuredOutput(t *testing.T) {
	if !(&Provider{}).Capabilities().StructuredOutput {
		t.Error("gemini should advertise StructuredOutput")
	}
}
