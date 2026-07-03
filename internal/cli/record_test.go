package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
)

// record-fixture through the real CLI path: record a session, then replay
// the written fixture through a second run.
func TestRecordFixtureCLIRoundTrip(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	fixtureOut := filepath.Join(dir, "recorded.yaml")

	source := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "recorded reply"}, {Finish: "end_turn"}}},
	}}

	var out, errOut bytes.Buffer
	code := runAgent(runOptions{
		specPath:   writeSpec(t, dir),
		task:       "record me",
		workspace:  t.TempDir(),
		fixtureOut: fixtureOut,
		version:    "test",
		factory:    scriptedFactory(source),
		stdout:     &out, stderr: &errOut,
	})
	if code != ExitOK {
		t.Fatalf("record run exit = %d, stderr: %s", code, errOut.String())
	}
	if _, err := os.Stat(fixtureOut); err != nil {
		t.Fatalf("fixture not written: %v", err)
	}

	// Replay: second runAgent consuming the recorded fixture.
	replayFactory := func(_ context.Context, _ string) (provider.Provider, error) {
		return scripted.Load(fixtureOut)
	}
	out.Reset()
	errOut.Reset()
	code = runAgent(runOptions{
		specPath:  writeSpec(t, dir),
		task:      "record me",
		workspace: t.TempDir(),
		version:   "test",
		factory:   replayFactory,
		stdout:    &out, stderr: &errOut,
	})
	if code != ExitOK {
		t.Fatalf("replay exit = %d, stderr: %s", code, errOut.String())
	}
}

func TestRecordFixtureWriteFailureExits1(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	code := runAgent(runOptions{
		specPath:   writeSpec(t, dir),
		task:       "x",
		workspace:  t.TempDir(),
		fixtureOut: filepath.Join(dir, "no-such-dir", "f.yaml"),
		version:    "test",
		factory: scriptedFactory(scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
		}}),
		stdout: &out, stderr: &errOut,
	})
	if code != ExitRun {
		t.Fatalf("exit = %d, want %d", code, ExitRun)
	}
}

func TestProviderFailureExitCodes(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()

	// Unknown provider name → usage error (2). The spec names a provider
	// the default factory does not know.
	spec := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(spec, []byte("name: t\nmodel: {provider: nope, id: x}\nsystem_prompt: s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := runAgent(runOptions{
		specPath: spec, task: "x", workspace: t.TempDir(), version: "test",
		factory: defaultProviderFactory,
		stdout:  &out, stderr: &errOut,
	})
	if code != ExitUsage {
		t.Fatalf("unknown provider exit = %d, want %d", code, ExitUsage)
	}

	// Construction failure (gemini without credentials) → run error (1).
	t.Setenv("GEMINI_API_KEY", "")
	spec2 := filepath.Join(dir, "gem.yaml")
	if err := os.WriteFile(spec2, []byte("name: t\nmodel: {provider: gemini, id: x}\nsystem_prompt: s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code = runAgent(runOptions{
		specPath: spec2, task: "x", workspace: t.TempDir(), version: "test",
		factory: defaultProviderFactory,
		stdout:  &out, stderr: &errOut,
	})
	if code != ExitRun {
		t.Fatalf("construction failure exit = %d, want %d", code, ExitRun)
	}
}
