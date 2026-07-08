package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/store"
)

// seedSession writes a small event log under a fake XDG data dir.
func seedSession(t *testing.T, id string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_DATA_HOME", dir)
	seedSessionIn(t, id)
}

func seedSessionIn(t *testing.T, id string) {
	t.Helper()
	sess := filepath.Join(mustDataDir(t), "sessions", id)
	s, err := store.OpenEventStore(sess)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for _, pair := range []struct {
		typ     string
		payload any
	}{
		{event.TypeSessionStarted, &event.SessionStarted{SpecName: "hello", Model: "m", Task: "t", Version: "dev"}},
		{event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}},
		{event.TypeTaskCompleted, &event.TaskCompleted{Reason: "completed", GenSteps: 1}},
	} {
		env, err := event.New(pair.typ, pair.payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Append(env); err != nil {
			t.Fatal(err)
		}
	}
}

// seedChildSession writes a child journal under <parent>/sub/<leaf> and
// returns the child's addressable full id (INC-1).
func seedChildSession(t *testing.T, parentID, leaf string) string {
	t.Helper()
	sess := filepath.Join(mustDataDir(t), "sessions", parentID, "sub", leaf)
	s, err := store.OpenEventStore(sess)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	env, err := event.New(event.TypeSessionStarted,
		&event.SessionStarted{SpecName: "worker", Model: "m", Task: "child task", Version: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(env); err != nil {
		t.Fatal(err)
	}
	return parentID + "-sub-" + leaf
}

func TestResolveChildSessionDir(t *testing.T) {
	seedSession(t, "20260708-010101-parent-aaaa")
	childID := seedChildSession(t, "20260708-010101-parent-aaaa", "call_2_0-a1")

	dir, err := resolveSessionDir(childID)
	if err != nil {
		t.Fatalf("child resolve: %v", err)
	}
	want := filepath.Join(mustDataDir(t), "sessions", "20260708-010101-parent-aaaa", "sub", "call_2_0-a1")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}

	// Grandchild nesting: each -sub- segment steps one directory deeper.
	grandID := seedChildSession(t, "20260708-010101-parent-aaaa/sub/call_2_0-a1", "call_1_0-a1")
	_ = grandID // the addressable id is built from segments, not the seed path
	gdir, err := resolveSessionDir(childID + "-sub-call_1_0-a1")
	if err != nil {
		t.Fatalf("grandchild resolve: %v", err)
	}
	if !strings.HasSuffix(gdir, filepath.Join("sub", "call_2_0-a1", "sub", "call_1_0-a1")) {
		t.Fatalf("grandchild dir = %q", gdir)
	}

	if _, err := resolveSessionDir("20260708-010101-parent-aaaa-sub-call_9_9-a1"); err == nil {
		t.Fatal("missing child must not resolve")
	}
}

func TestEventsChildSession(t *testing.T) {
	seedSession(t, "20260708-020202-parent-bbbb")
	childID := seedChildSession(t, "20260708-020202-parent-bbbb", "call_3_1-a1")

	var out, errOut bytes.Buffer
	if code := Run([]string{"events", childID}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"spec_name":"worker"`) {
		t.Errorf("child journal not rendered:\n%s", out.String())
	}
}

func mustDataDir(t *testing.T) string {
	t.Helper()
	dir, err := runtime.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEventsPrettyPrint(t *testing.T) {
	seedSession(t, "20260703-010203-fix-abcd")
	var out, errOut bytes.Buffer
	code := Run([]string{"events", "20260703-010203-fix-abcd"}, "dev", &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"session_started", "generation_started", "task_completed", `"spec_name":"hello"`} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
}

func TestEventsUniquePrefixAndState(t *testing.T) {
	seedSession(t, "20260703-010203-fix-abcd")
	var out, errOut bytes.Buffer
	code := Run([]string{"events", "20260703", "--state"}, "dev", &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	var folded struct {
		Session struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"session"`
	}
	if err := json.Unmarshal(out.Bytes(), &folded); err != nil {
		t.Fatalf("--state output not JSON: %v\n%s", err, out.String())
	}
	if folded.Session.Status != "completed" || folded.Session.Reason != "completed" {
		t.Errorf("folded run = %+v", folded.Session)
	}
}

func TestEventsJSONMode(t *testing.T) {
	seedSession(t, "20260703-010203-fix-abcd")
	var out, errOut bytes.Buffer
	if code := Run([]string{"events", "20260703", "--json"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	var env event.Envelope
	if err := json.Unmarshal([]byte(lines[0]), &env); err != nil || env.Seq != 1 {
		t.Errorf("line 0: %v %+v", err, env)
	}
}

func TestEventsAmbiguousPrefix(t *testing.T) {
	seedSession(t, "20260703-010203-fix-abcd")
	seedSessionIn(t, "20260703-020304-other-ef01")
	var out, errOut bytes.Buffer
	code := Run([]string{"events", "20260703"}, "dev", &out, &errOut)
	if code != ExitUsage || !strings.Contains(errOut.String(), "ambiguous") {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
}

func TestEventsUnknownSession(t *testing.T) {
	seedSession(t, "20260703-010203-fix-abcd")
	var out, errOut bytes.Buffer
	code := Run([]string{"events", "nope"}, "dev", &out, &errOut)
	if code != ExitUsage || !strings.Contains(errOut.String(), "no session matches") {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
}
