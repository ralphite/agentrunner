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

// writeStructuredSpec writes a spec whose output_schema drives structured
// output (PLAN 5.7 single entry) on the scripted (non-native) provider.
func writeStructuredSpec(t *testing.T, dir string) string {
	t.Helper()
	useScriptedDefaultModel(t, dir)
	path := filepath.Join(dir, "spec.yaml")
	spec := `name: t
system_prompt: help
output_schema:
  type: object
  properties:
    name: { type: string }
    lines: { type: integer }
  required: [name, lines]
  additionalProperties: false
`
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestSpecSchemaFallbackRetriesThenValidates (PLAN 5.7): the spec declares
// output_schema, the provider (scripted) lacks native structured output, so
// `new` engages the internal validate-and-retry fallback automatically — no
// flag. The opening reply misses the schema, the correction re-prompt lands,
// and the canonical structured output is printed.
func TestSpecSchemaFallbackRetriesThenValidates(t *testing.T) {
	dir := t.TempDir()
	specPath := writeStructuredSpec(t, dir)
	// step 1 = opening run (non-conforming: missing "lines").
	// step 2 = after the CLI's correction send (conforming).
	fixture := `steps:
  - respond: [ { text: "Here you go: {\"name\":\"a.go\"}" }, { finish: end_turn } ]
  - respond: [ { text: "{\"name\":\"a.go\",\"lines\":42}" }, { finish: end_turn } ]
`
	startScriptedDaemon(t, dir, fixture)

	ws := t.TempDir()
	var out, errOut bytes.Buffer
	code := newCmd([]string{"--workspace", ws, specPath, "count the lines in a.go"}, &out, &errOut)
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

// TestSpecSchemaFallbackExhaustsRetries: every reply misses the schema, so
// after the bounded fallback retries (structuredFallbackRetries corrections)
// the run fails with a non-zero exit and a schema error.
func TestSpecSchemaFallbackExhaustsRetries(t *testing.T) {
	dir := t.TempDir()
	specPath := writeStructuredSpec(t, dir)
	// 3 attempts (1 opening + structuredFallbackRetries corrections), all
	// non-conforming.
	fixture := `steps:
  - respond: [ { text: "no json here at all" }, { finish: end_turn } ]
  - respond: [ { text: "{\"name\":\"a.go\"}" }, { finish: end_turn } ]
  - respond: [ { text: "still not conforming" }, { finish: end_turn } ]
`
	startScriptedDaemon(t, dir, fixture)

	ws := t.TempDir()
	var out, errOut bytes.Buffer
	code := newCmd([]string{"--workspace", ws, specPath, "count lines"}, &out, &errOut)
	if code == ExitOK {
		t.Fatalf("exit = ExitOK, want failure (retries exhausted)\nstdout: %s\nstderr: %s", out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "did not conform") {
		t.Errorf("stderr missing non-conformance report:\n%s", errOut.String())
	}
}

// TestSpecSchemaFallbackUsageErrors: the retired --json-schema flag is
// refused (single entry = spec output_schema), and a fallback-needing spec
// with --detach fails fast, before any session is minted.
func TestSpecSchemaFallbackUsageErrors(t *testing.T) {
	dir := t.TempDir()
	ws := t.TempDir()

	// The retired flag errors as unknown — the spec is the only entry.
	plainSpec := writeSpec(t, dir)
	var out, errOut bytes.Buffer
	if code := newCmd([]string{"--workspace", ws, "--json-schema", "x.json", plainSpec, "hi"}, &out, &errOut); code != ExitUsage {
		t.Errorf("retired --json-schema flag: exit = %d, want ExitUsage", code)
	}

	// Fallback (non-native provider) + --detach is incompatible.
	structuredSpec := writeStructuredSpec(t, dir)
	out.Reset()
	errOut.Reset()
	if code := newCmd([]string{"--workspace", ws, "--detach", structuredSpec, "hi"}, &out, &errOut); code != ExitUsage {
		t.Errorf("detach+fallback: exit = %d, want ExitUsage", code)
	}
	if !strings.Contains(errOut.String(), "detach") {
		t.Errorf("stderr should explain the --detach conflict:\n%s", errOut.String())
	}
}
