package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/state/statetest"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// After a completed multi-turn run: fold(snapshot + tail) must equal
// fold(all events) — the 2.13 equivalence property on real loop output.
func TestSnapshotTailEquivalence(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "greet.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "read_file", Args: map[string]any{"path": "greet.txt"}}},
			{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
				"path": "greet.txt", "old": "hello world", "new": "HELLO WORLD"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, root)
	if _, err := l.Run(context.Background(), "make it loud"); err != nil {
		t.Fatal(err)
	}

	dir := l.Store.Dir()
	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	full, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}

	snap, ok, err := store.LatestSnapshot(dir)
	if err != nil || !ok {
		t.Fatalf("no snapshot after multi-turn run (err=%v)", err)
	}
	if snap.UptoSeq <= 1 {
		t.Fatalf("snapshot upto_seq = %d", snap.UptoSeq)
	}
	if snap.JournalOffset <= 0 || len(snap.JournalHash) != 64 {
		t.Fatalf("snapshot has no indexed journal cursor: %+v", snap)
	}
	tail, err := store.ReadEventsAfter(dir, snap.UptoSeq, snap.JournalOffset, snap.JournalHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) >= len(events) {
		t.Fatalf("indexed tail read %d events, full journal has %d", len(tail), len(events))
	}
	var fromSnap state.State
	if err := json.Unmarshal(snap.State, &fromSnap); err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Seq <= snap.UptoSeq {
			continue
		}
		if fromSnap, err = state.Apply(fromSnap, e); err != nil {
			t.Fatal(err)
		}
	}
	statetest.AssertFoldEqual(t, fromSnap, full)
}

func TestResumeRefusesVersionMismatch(t *testing.T) {
	l := testLoop(t, scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}, t.TempDir())
	// Seed a session that is NOT ended so Resume reaches the version check:
	// journal only SessionStarted.
	env, err := event.New(event.TypeSessionStarted, &event.SessionStarted{SpecName: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Store.Append(env); err != nil {
		t.Fatal(err)
	}
	bad := state.SubStateVersions()
	bad["conversation"] = 99
	if err := store.WriteSnapshot(l.Store.Dir(), 1, bad, state.New()); err != nil {
		t.Fatal(err)
	}

	_, err = l.Resume(context.Background())
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("err = %v, want version mismatch refusal", err)
	}
}

func TestSchemaGuardAcceptsOlderNamespaceSubset(t *testing.T) {
	legacy := state.SubStateVersions()
	delete(legacy, "team") // binary added this optional namespace later
	if err := checkVersions(legacy); err != nil {
		t.Fatalf("compatible legacy namespace subset refused: %v", err)
	}
	legacy["future_unknown"] = 1
	if err := checkVersions(legacy); err == nil {
		t.Fatal("unknown namespace must still fail closed")
	}
}

func TestResumeFullFoldsLegacySnapshotMissingNewProjection(t *testing.T) {
	l := testLoop(t, scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}, t.TempDir())
	if _, err := l.Run(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	legacyState, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	legacyState.Session.Usage.InputTokens = 999999 // poison proves cache was ignored
	legacyVersions := state.SubStateVersions()
	delete(legacyVersions, "team")
	if err := store.WriteSnapshot(l.Store.Dir(), events[len(events)-1].Seq,
		legacyVersions, legacyState); err != nil {
		t.Fatal(err)
	}
	res, err := l.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Usage.InputTokens == 999999 {
		t.Fatal("legacy snapshot missing a projection was tail-replayed instead of full-folded")
	}
}

// 决策 #30/#31: there is no "already ended" — resuming a quiescent session
// is lawful and idles right back out (no input source), re-running nothing.
func TestResumeQuiescentIsLawful(t *testing.T) {
	l := testLoop(t, scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
	}}, t.TempDir())
	if _, err := l.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	before, _ := store.ReadEvents(l.Store.Dir())
	res, err := l.Resume(context.Background())
	if err != nil {
		t.Fatalf("resume of a quiescent session must be lawful: %v", err)
	}
	if res.Reason != "completed" || res.GenSteps != 1 {
		t.Errorf("res = %+v", res)
	}
	after, _ := store.ReadEvents(l.Store.Dir())
	for _, e := range after[len(before):] {
		if e.Type == event.TypeGenerationStarted {
			t.Fatalf("resume re-ran a generation step on a quiescent session")
		}
	}
}

// The 2.13 crash-matrix scenario: a subprocess is killed right after turn
// 1's tool results land (counting predicate on the second generation_started —
// i.e. mid-run at a turn boundary); the parent resumes the SAME session
// dir and the run finishes turn 2 without re-running turn 1.
func TestCrashThenResumeContinuesRun(t *testing.T) {
	if os.Getenv("GO_CRASH_HELPER") == "1" {
		helperCrashRun(t)
		return
	}

	base := t.TempDir()
	sessDir := filepath.Join(base, "sess")
	root := filepath.Join(base, "ws")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "greet.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCrashThenResumeContinuesRun")
	cmd.Env = append(os.Environ(),
		"GO_CRASH_HELPER=1",
		"CRASH_SESS_DIR="+sessDir,
		"CRASH_WS="+root,
		crash.EnvVar+"=after:generation_started:2", // die at the turn-2 boundary
	)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != crash.ExitCode {
		t.Fatalf("subprocess: err = %v, out = %s", err, out)
	}

	// The file was edited before the crash; the run is not ended.
	if got, _ := os.ReadFile(filepath.Join(root, "greet.txt")); string(got) != "HELLO WORLD" {
		t.Fatalf("pre-crash work lost: %q", got)
	}

	// Resume with a fixture containing ONLY the remaining turn: any attempt
	// to re-run turn 1 would drift (expects the edited transcript) or
	// exhaust the fixture.
	es, err := store.OpenEventStore(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	prov := scripted.New(scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{{Text: "all done"}, {Finish: "end_turn"}}},
	}})
	l := &Loop{
		Spec:      crashSpec(),
		Provider:  prov,
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		SessionID: "crash-resume",
	}
	res, err := l.Resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.GenSteps != 2 {
		t.Fatalf("res = %+v", res)
	}
	if err := prov.Done(); err != nil {
		t.Errorf("resume fixture: %v", err)
	}

	// The log is one coherent story: exactly one turn-1 LLM activity.
	events, err := store.ReadEvents(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	llmT1 := 0
	for _, e := range events {
		if e.Type != event.TypeActivityStarted {
			continue
		}
		var started event.ActivityStarted
		if err := json.Unmarshal(e.Payload, &started); err != nil {
			t.Fatal(err)
		}
		if started.ActivityID == "llm-t1" {
			llmT1++
		}
	}
	if llmT1 != 1 {
		t.Errorf("llm-t1 started %d times, want 1 (turn 1 must not re-run)", llmT1)
	}
	if fold, ferr := state.Fold(events); ferr != nil {
		t.Fatal(ferr)
	} else if q, _ := state.Quiescence(fold); !q {
		t.Errorf("journal not quiescent after resume completion")
	}
}

func crashSpec() *AgentSpec {
	return &AgentSpec{
		Name:               "crashy",
		Model:              ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 100},
		SystemPrompt:       "be helpful",
		Tools:              []string{"read_file", "edit_file"},
		MaxGenerationSteps: 5,
	}
}

// helperCrashRun executes turn 1 (read + edit) and is killed by the
// counting predicate when GenerationStarted{2} is appended.
func helperCrashRun(t *testing.T) {
	es, err := store.OpenEventStore(os.Getenv("CRASH_SESS_DIR"))
	if err != nil {
		fmt.Println("helper:", err)
		os.Exit(1)
	}
	ws, err := workspace.New(os.Getenv("CRASH_WS"))
	if err != nil {
		fmt.Println("helper:", err)
		os.Exit(1)
	}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{Name: "read_file", Args: map[string]any{"path": "greet.txt"}}},
			{ToolCall: &scripted.ToolCallEvent{Name: "edit_file", Args: map[string]any{
				"path": "greet.txt", "old": "hello world", "new": "HELLO WORLD"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "unreachable"}, {Finish: "end_turn"}}},
	}}
	l := &Loop{
		Spec:      crashSpec(),
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		SessionID: "crash-resume",
	}
	_, _ = l.Run(context.Background(), "make it loud")
	fmt.Println("UNREACHABLE: predicate did not fire")
	os.Exit(0)
}

// 决策 #18 (INC-80 契约 review P1): an OLDER sub-state version is a
// compatible additive predecessor — the reader accepts it (new fields fold
// from zero values); only a NEWER-than-binary version refuses.
func TestSchemaGuardAcceptsOlderSubStateVersion(t *testing.T) {
	older := state.SubStateVersions()
	older["series"] = 1 // the pre-INC-80.2b③ window (07-18..07-19) wrote v1
	if err := checkVersions(older); err != nil {
		t.Fatalf("older additive version refused: %v", err)
	}
	newer := state.SubStateVersions()
	newer["series"]++
	if err := checkVersions(newer); err == nil {
		t.Fatal("a journal from a NEWER binary must fail closed")
	}
}
