package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// S5.5 e2e: publish_artifact writes the blob, journals ArtifactPublished,
// versions accumulate per stream, and the fold tracks the latest version.
func TestPublishArtifactEndToEnd(t *testing.T) {
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "a1", Name: "publish_artifact",
				Args: map[string]any{"stream": "report", "content": "draft body"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "a2", Name: "publish_artifact",
				Args: map[string]any{"stream": "report", "content": "final body"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	l.Spec.Tools = append(l.Spec.Tools, "publish_artifact")

	res, err := l.Run(context.Background(), "publish twice")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" {
		t.Fatalf("res = %+v", res)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var published []*event.ArtifactPublished
	for _, e := range events {
		if e.Type == event.TypeArtifactPublished {
			dec, _ := event.DecodePayload(e)
			published = append(published, dec.(*event.ArtifactPublished))
		}
	}
	if len(published) != 2 || published[0].Version != 1 || published[1].Version != 2 {
		t.Fatalf("published = %+v", published)
	}
	// Every journaled ref RESOLVES (blob-before-event invariant, read side).
	for _, p := range published {
		content, err := l.Artifacts.Get(p.Ref)
		if err != nil {
			t.Fatalf("journaled ref %s does not resolve: %v", p.Ref, err)
		}
		if p.Version == 2 && string(content) != "final body" {
			t.Errorf("v2 content = %q", content)
		}
	}
	// The fold tracks the stream's latest version (S5.6's contract input).
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if fold.Session.Published["report"] != 2 {
		t.Errorf("fold published = %+v", fold.Session.Published)
	}
}

// S5.5 hard line: a crash between blob write and event append leaves an
// orphan blob and a CLEAN journal — never a journaled ref without a blob.
// Subprocess arms the injection point; the parent asserts both sides.
func TestPublishArtifactCrashBetweenBlobAndEvent(t *testing.T) {
	if os.Getenv("GO_ARTIFACT_CRASH_HELPER") == "1" {
		helperArtifactCrashRun()
		return
	}
	base := t.TempDir()
	sessDir := filepath.Join(base, "sess")
	wsDir := filepath.Join(base, "ws")
	if err := os.Mkdir(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestPublishArtifactCrashBetweenBlobAndEvent")
	cmd.Env = append(os.Environ(),
		"GO_ARTIFACT_CRASH_HELPER=1",
		"ARTIFACT_CRASH_SESS="+sessDir,
		"ARTIFACT_CRASH_WS="+wsDir,
		crash.EnvVar+"=point:"+crash.PointAfterBlobBeforeEvent,
	)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != crash.ExitCode {
		t.Fatalf("subprocess: err = %v, out = %s", err, out)
	}

	// The blob IS durable (orphan)…
	arts, err := store.OpenArtifactStore(filepath.Join(sessDir, "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	latest, ok, err := arts.Latest("report")
	if err != nil || !ok {
		t.Fatalf("blob/manifest must be durable before the crash: %+v, %v, %v", latest, ok, err)
	}
	if content, err := arts.Get(latest.Ref); err != nil || string(content) != "crash payload" {
		t.Fatalf("orphan blob unreadable: %q, %v", content, err)
	}
	// …and the journal has NO ArtifactPublished (the fact never landed).
	events, err := store.ReadEvents(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Type == event.TypeArtifactPublished {
			t.Fatalf("journal has artifact_published despite crashing before the append")
		}
	}
	// The fold is clean: no published stream — orphan, not dangling.
	fold, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if len(fold.Session.Published) != 0 {
		t.Fatalf("fold published = %+v, want none", fold.Session.Published)
	}
}

func helperArtifactCrashRun() {
	es, err := store.OpenEventStore(os.Getenv("ARTIFACT_CRASH_SESS"))
	if err != nil {
		os.Exit(1)
	}
	ws, err := workspace.New(os.Getenv("ARTIFACT_CRASH_WS"))
	if err != nil {
		os.Exit(1)
	}
	fix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "a1", Name: "publish_artifact",
				Args: map[string]any{"stream": "report", "content": "crash payload"}}},
			{Finish: "tool_use"},
		}},
	}}
	l := &Loop{
		Spec: &AgentSpec{Name: "crash",
			Model: ModelSpec{Provider: "scripted", ID: "m", MaxTokens: 100},
			Tools: []string{"read_file", "publish_artifact"}, MaxGenerationSteps: 2},
		Provider:  scripted.New(fix),
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		SessionID: "crash",
	}
	_, _ = l.Run(context.Background(), "publish then die")
	os.Exit(0) // unreachable when the point fires
}

// S5.5: children publish into the ROOT's store — refs resolve tree-wide.
func TestArtifactStoreTreeShared(t *testing.T) {
	parentFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "s1", Name: "spawn_agent",
				Args: map[string]any{"agent": "summarizer", "task": "PUBLISH-JOB now"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "waiting"}, {Finish: "end_turn"}}},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	// Child publishes.
	childFix := scripted.Fixture{Steps: []scripted.Step{
		{Respond: []scripted.Event{
			{ToolCall: &scripted.ToolCallEvent{CallID: "c1", Name: "publish_artifact",
				Args: map[string]any{"stream": "child-report", "content": "from the child"}}},
			{Finish: "tool_use"},
		}},
		{Respond: []scripted.Event{{Text: "published"}, {Finish: "end_turn"}}},
	}}
	l, _ := routedSpawnLoop(t, parentFix, t.TempDir(),
		scripted.RoutePair{Key: "PUBLISH-JOB", Fixture: childFix})
	child := summarizerSpec()
	child.Tools = append(child.Tools, "publish_artifact")
	l.SubSpecs = staticResolver(map[string]*AgentSpec{"summarizer": child})

	if _, err := l.Run(context.Background(), "delegate"); err != nil {
		t.Fatal(err)
	}
	// The PARENT's store resolves the child's publication.
	latest, ok, err := l.Artifacts.Latest("child-report")
	if err != nil || !ok {
		t.Fatalf("child publication missing from root store: %v, %v", ok, err)
	}
	content, err := l.Artifacts.Get(latest.Ref)
	if err != nil || string(content) != "from the child" {
		t.Fatalf("content = %q, %v", content, err)
	}
	// The fact is journaled in the CHILD's log (its run published it).
	childEvents, err := store.ReadEvents(filepath.Join(l.Store.Dir(), "sub", "s1-a1"))
	if err != nil {
		t.Fatal(err)
	}
	saw := false
	for _, e := range childEvents {
		if e.Type == event.TypeArtifactPublished && strings.Contains(string(e.Payload), "child-report") {
			saw = true
		}
	}
	if !saw {
		t.Error("child journal missing artifact_published")
	}
}
