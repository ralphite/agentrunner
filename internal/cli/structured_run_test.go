package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/runtime"
)

const structuredTestSchema = `{
  "type": "object",
  "properties": {"name": {"type": "string"}, "lines": {"type": "integer"}},
  "required": ["name", "lines"],
  "additionalProperties": false
}`

// startScriptedDaemon boots the in-process daemon backed by the scripted
// provider fixture, exactly as the conversational twins do, and returns once
// it is accepting connections.
func startScriptedDaemon(t *testing.T, dir, fixture string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	fixPath := filepath.Join(dir, "fix.yaml")
	if err := os.WriteFile(fixPath, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTRUNNER_SCRIPTED_FIXTURE", fixPath)
	sock, err := socketPath()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var errLog bytes.Buffer
	broker := daemon.NewApprovalBroker()
	srv := &daemon.Server{
		SocketPath:   sock,
		NewID:        func(prompt string) string { return runtime.NewSessionID(time.Now(), prompt) },
		Run:          hostRunFunc("test", &errLog, broker),
		Approvals:    broker,
		PersistInput: persistInputFunc(),
	}
	go func() { _ = srv.ListenAndServe(ctx) }()
	waitDaemon(t, sock)
}

// TestNewJSONSchemaRetriesThenValidates: the opening reply misses the schema
// (no "lines"), the CLI re-prompts, the second reply conforms, and the
// canonical structured output is printed. Exercises runStructured +
// captureFinal + the re-send retry with a scripted provider (deterministic).
func TestNewJSONSchemaRetriesThenValidates(t *testing.T) {
	dir := t.TempDir()
	specPath := writeSpec(t, dir)
	schemaPath := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(schemaPath, []byte(structuredTestSchema), 0o644); err != nil {
		t.Fatal(err)
	}
	// step 1 = opening run (non-conforming: missing "lines").
	// step 2 = after the CLI's correction send (conforming).
	fixture := `steps:
  - respond: [ { text: "Here you go: {\"name\":\"a.go\"}" }, { finish: end_turn } ]
  - respond: [ { text: "{\"name\":\"a.go\",\"lines\":42}" }, { finish: end_turn } ]
`
	startScriptedDaemon(t, dir, fixture)

	ws := t.TempDir()
	var out, errOut bytes.Buffer
	code := newCmd([]string{"--workspace", ws, "--json-schema", schemaPath, specPath, "count the lines in a.go"}, &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit = %d, want ExitOK\nstdout: %s\nstderr: %s", code, out.String(), errOut.String())
	}
	// stdout carries the canonical (compact, key-sorted) structured output.
	if got := strings.TrimSpace(out.String()); got != `{"lines":42,"name":"a.go"}` {
		t.Fatalf("structured output = %q, want canonical JSON", got)
	}
	if !strings.Contains(errOut.String(), "validated") {
		t.Errorf("stderr missing validation confirmation:\n%s", errOut.String())
	}
}

// TestNewJSONSchemaExhaustsRetries: every reply misses the schema, so after the
// bounded retries the run fails with a non-zero exit and a schema error.
func TestNewJSONSchemaExhaustsRetries(t *testing.T) {
	dir := t.TempDir()
	specPath := writeSpec(t, dir)
	schemaPath := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(schemaPath, []byte(structuredTestSchema), 0o644); err != nil {
		t.Fatal(err)
	}
	// 2 attempts (1 opening + 1 retry), both non-conforming.
	fixture := `steps:
  - respond: [ { text: "no json here at all" }, { finish: end_turn } ]
  - respond: [ { text: "{\"name\":\"a.go\"}" }, { finish: end_turn } ]
`
	startScriptedDaemon(t, dir, fixture)

	ws := t.TempDir()
	var out, errOut bytes.Buffer
	code := newCmd([]string{"--workspace", ws, "--json-schema", schemaPath, "--json-schema-max-retries", "1", specPath, "count lines"}, &out, &errOut)
	if code == ExitOK {
		t.Fatalf("exit = ExitOK, want failure (retries exhausted)\nstdout: %s\nstderr: %s", out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "did not conform") {
		t.Errorf("stderr missing non-conformance report:\n%s", errOut.String())
	}
}

// TestNewJSONSchemaUsageErrors: bad/missing schema and --detach incompatibility
// fail fast, before any session is minted.
func TestNewJSONSchemaUsageErrors(t *testing.T) {
	dir := t.TempDir()
	specPath := writeSpec(t, dir)
	ws := t.TempDir()

	// Missing schema file.
	var out, errOut bytes.Buffer
	if code := newCmd([]string{"--workspace", ws, "--json-schema", filepath.Join(dir, "nope.json"), specPath, "hi"}, &out, &errOut); code != ExitUsage {
		t.Errorf("missing schema: exit = %d, want ExitUsage", code)
	}

	// Malformed schema.
	badPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := newCmd([]string{"--workspace", ws, "--json-schema", badPath, specPath, "hi"}, &out, &errOut); code != ExitUsage {
		t.Errorf("malformed schema: exit = %d, want ExitUsage", code)
	}

	// --json-schema + --detach is incompatible.
	goodPath := filepath.Join(dir, "ok.json")
	if err := os.WriteFile(goodPath, []byte(structuredTestSchema), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := newCmd([]string{"--workspace", ws, "--detach", "--json-schema", goodPath, specPath, "hi"}, &out, &errOut); code != ExitUsage {
		t.Errorf("detach+json-schema: exit = %d, want ExitUsage", code)
	}
	if !strings.Contains(errOut.String(), "detach") {
		t.Errorf("stderr should explain the --detach conflict:\n%s", errOut.String())
	}
}
