package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

const resumeSpecYAML = `name: fixer
model: { provider: scripted, id: x }
system_prompt: fix things
tools: [read_file, edit_file]
permissions:
  - { action: allow }
`

const crashFixtureYAML = `steps:
  - respond:
      - tool_call: { name: read_file, args: { path: greet.txt } }
      - tool_call: { name: edit_file, args: { path: greet.txt, old: hello world, new: HELLO WORLD } }
      - finish: tool_use
  - respond:
      - text: unreachable before crash
      - finish: end_turn
`

const resumeFixtureYAML = `steps:
  - respond:
      - text: all done
      - finish: end_turn
`

// End-to-end CLI crash + resume: `run` dies at the turn-2 boundary in a
// subprocess; `resume <prefix>` reconstructs the loop from session_started
// (spec + workspace root journaled) and finishes the run.
func TestCLIResumeAfterCrash(t *testing.T) {
	if os.Getenv("GO_CRASH_HELPER") == "1" {
		os.Exit(Run([]string{"run", "--workspace", os.Getenv("CRASH_WS"),
			os.Getenv("CRASH_SPEC"), "make it loud"}, "dev", os.Stdout, os.Stderr))
	}

	base := t.TempDir()
	xdg := filepath.Join(base, "xdg")
	ws := filepath.Join(base, "ws")
	if err := os.Mkdir(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "greet.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(base, "spec.yaml")
	if err := os.WriteFile(specPath, []byte(resumeSpecYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	crashFix := filepath.Join(base, "crash.yaml")
	if err := os.WriteFile(crashFix, []byte(crashFixtureYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLIResumeAfterCrash")
	cmd.Env = append(os.Environ(),
		"GO_CRASH_HELPER=1",
		"XDG_DATA_HOME="+xdg,
		"CRASH_WS="+ws,
		"CRASH_SPEC="+specPath,
		"AGENTRUNNER_SCRIPTED_FIXTURE="+crashFix,
		crash.EnvVar+"=after:generation_started:2",
	)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != crash.ExitCode {
		t.Fatalf("run subprocess: err = %v, out = %s", err, out)
	}

	// Resume in-process through the CLI with the remaining fixture.
	t.Setenv("XDG_DATA_HOME", xdg)
	resumeFix := filepath.Join(base, "resume.yaml")
	if err := os.WriteFile(resumeFix, []byte(resumeFixtureYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTRUNNER_SCRIPTED_FIXTURE", resumeFix)
	t.Setenv(crash.EnvVar, "")

	var stdout, stderr bytes.Buffer
	// The session id begins with the date; a bare prefix that matches the
	// single session is enough.
	sessions, err := os.ReadDir(filepath.Join(xdg, "agentrunner", "sessions"))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %v (err %v)", sessions, err)
	}
	code := Run([]string{"resume", sessions[0].Name()[:8]}, "dev", &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("resume exit = %d\nstderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "run completed: 2 turns") {
		t.Errorf("stderr = %s", stderr.String())
	}
	got, _ := os.ReadFile(filepath.Join(ws, "greet.txt"))
	if string(got) != "HELLO WORLD" {
		t.Errorf("file = %q", got)
	}

	// sessions list shows the delivery receipt.
	stdout.Reset()
	if code := Run([]string{"sessions", "list"}, "dev", &stdout, &stderr); code != ExitOK {
		t.Fatalf("sessions list exit = %d", code)
	}
	if !strings.Contains(stdout.String(), "completed") {
		t.Errorf("sessions list:\n%s", stdout.String())
	}

	// The product surfaces need durable workspace/title metadata for every
	// session, not only sessions they created themselves.
	stdout.Reset()
	if code := Run([]string{"sessions", "list", "--json"}, "dev", &stdout, &stderr); code != ExitOK {
		t.Fatalf("sessions json exit = %d: %s", code, stderr.String())
	}
	var listed []struct {
		ID        string `json:"id"`
		Workspace string `json:"workspace"`
		Title     string `json:"title"`
		Kind      string `json:"kind"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatalf("decode sessions json: %v\n%s", err, stdout.String())
	}
	wantWorkspace, err := filepath.EvalSymlinks(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Workspace != wantWorkspace || listed[0].Title != "make it loud" || listed[0].Kind != "session" {
		t.Fatalf("sessions json = %+v, want workspace %q and journaled title", listed, wantWorkspace)
	}
}

func TestCLIResumeUnknownSession(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var out, errOut bytes.Buffer
	if code := Run([]string{"resume", "nope"}, "dev", &out, &errOut); code != ExitUsage {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
}

func TestCLISessionsListEmpty(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var out, errOut bytes.Buffer
	if code := Run([]string{"sessions", "list"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "no sessions") {
		t.Errorf("out = %q", out.String())
	}
	out.Reset()
	if code := Run([]string{"sessions", "--json"}, "dev", &out, &errOut); code != ExitOK || strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("empty json exit = %d, out = %q, err = %q", code, out.String(), errOut.String())
	}
}

func TestCLISessionsPaginationNewestFirst(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	root := filepath.Join(xdg, "agentrunner", "sessions")
	base := time.Unix(1_700_000_000, 0)
	for i, id := range []string{"old", "middle", "new"} {
		dir := filepath.Join(root, id)
		es, err := store.OpenEventStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := es.Append(mkEnv(t, event.TypeSessionStarted, &event.SessionStarted{})); err != nil {
			t.Fatal(err)
		}
		_ = es.Close()
		path := filepath.Join(dir, "events.jsonl")
		at := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"sessions", "list", "--json", "--limit", "1", "--offset", "1"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
	var rows []struct {
		ID        string `json:"id"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "middle" {
		t.Fatalf("rows=%+v, want middle page", rows)
	}
	if want := base.Add(time.Minute).UTC(); rows[0].UpdatedAt == "" {
		t.Fatal("middle row missing updated_at")
	} else if got, err := time.Parse(time.RFC3339Nano, rows[0].UpdatedAt); err != nil || !got.Equal(want) {
		t.Fatalf("updated_at=%q (%v), want %s", rows[0].UpdatedAt, err, want)
	}

	out.Reset()
	if code := Run([]string{"sessions", "--json"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("full exit=%d stderr=%s", code, errOut.String())
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil || len(rows) != 3 {
		t.Fatalf("full rows=%+v err=%v", rows, err)
	}
	if got := []string{rows[0].ID, rows[1].ID, rows[2].ID}; !reflect.DeepEqual(got, []string{"new", "middle", "old"}) {
		t.Fatalf("full order=%v, want journal update descending", got)
	}
	for _, args := range [][]string{{"sessions", "--limit", "-1"}, {"sessions", "--offset", "x"}, {"sessions", "--limit"}} {
		out.Reset()
		errOut.Reset()
		if code := Run(args, "dev", &out, &errOut); code != ExitUsage {
			t.Fatalf("args=%v exit=%d", args, code)
		}
	}
}

func TestCLISessionsJSONProjectsDriverMetadata(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	dir := filepath.Join(xdg, "agentrunner", "sessions", "driver-1")
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := json.Marshal(map[string]any{
		"Name":   "nightly",
		"Prompt": "Audit dependencies every night",
	})
	for _, item := range []struct {
		typ string
		v   any
	}{
		{event.TypeDriverStarted, &event.DriverStarted{DriverID: "driver-1", SpecName: "nightly", Spec: spec, WorkspaceRoot: "/tmp/project", FoldVersion: 1}},
		{event.TypeDriverCompleted, &event.DriverCompleted{DriverID: "driver-1", Reason: "satisfied", Iterations: 1}},
	} {
		if _, err := es.Append(mkEnv(t, item.typ, item.v)); err != nil {
			t.Fatal(err)
		}
	}
	if err := es.Close(); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"sessions", "--json"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("sessions exit=%d stderr=%s", code, errOut.String())
	}
	var rows []struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"`
		Schedule string `json:"schedule"`
		Title    string `json:"title"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "driver-1" || rows[0].Kind != "driver" || rows[0].Schedule != "immediate" || rows[0].Title != "Audit dependencies every night" {
		t.Fatalf("driver row = %+v", rows)
	}
}

func TestCLISessionsProjectsTypedHumanAttentionAcrossChildren(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	sessionsRoot := filepath.Join(xdg, "agentrunner", "sessions")
	parentDir := filepath.Join(sessionsRoot, "parent")
	type item struct {
		typ string
		v   any
	}
	write := func(dir string, items ...item) {
		t.Helper()
		es, err := store.OpenEventStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range items {
			if _, err := es.Append(mkEnv(t, entry.typ, entry.v)); err != nil {
				_ = es.Close()
				t.Fatal(err)
			}
		}
		if err := es.Close(); err != nil {
			t.Fatal(err)
		}
	}
	askDetail, err := json.Marshal(struct {
		Question string `json:"question"`
	}{Question: "Which release should I ship?"})
	if err != nil {
		t.Fatal(err)
	}
	write(parentDir,
		item{event.TypeSessionStarted, &event.SessionStarted{SpecName: "lead", Prompt: "coordinate release", SubStateVersions: state.SubStateVersions()}},
		item{event.TypeSpawnRequested, &event.SpawnRequested{CallID: "c1", Agent: "worker", ChildSession: "parent-sub-c1-a1", Depth: 1}},
		item{event.TypeSpawnRequested, &event.SpawnRequested{CallID: "c2", Agent: "worker", ChildSession: "parent-sub-c2-a1", Depth: 1}},
		item{event.TypeSpawnRequested, &event.SpawnRequested{CallID: "c3", Agent: "reviewer", ChildSession: "parent-sub-c3-a1", Depth: 1}},
		item{event.TypeSpawnRequested, &event.SpawnRequested{CallID: "c1", Agent: "worker", ChildSession: "parent-sub-c1-a1", Depth: 1}},
		item{event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitInput, Detail: askDetail}},
	)
	for _, callID := range []string{"c1", "c2"} {
		approvalID := "apr-" + callID
		detail, marshalErr := json.Marshal(&event.ApprovalRequested{ApprovalID: approvalID, ToolName: "bash"})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		write(filepath.Join(parentDir, "sub", callID+"-a1"),
			item{event.TypeSessionStarted, &event.SessionStarted{SpecName: "worker", SubStateVersions: state.SubStateVersions()}},
			item{event.TypeApprovalRequested, &event.ApprovalRequested{ApprovalID: approvalID, ToolName: "bash"}},
			item{event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitApproval, Detail: detail}},
		)
	}
	childAskDetail, err := json.Marshal(struct {
		Questions []event.AskQuestion `json:"questions"`
	}{Questions: []event.AskQuestion{{
		Question: "Choose a release channel",
		Options:  []event.AskOption{{Label: "Stable"}, {Label: "Beta"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(parentDir, "sub", "c3-a1"),
		item{event.TypeSessionStarted, &event.SessionStarted{SpecName: "reviewer", SubStateVersions: state.SubStateVersions()}},
		item{event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitInput, Detail: childAskDetail}},
	)
	write(filepath.Join(sessionsRoot, "ordinary-idle"),
		item{event.TypeSessionStarted, &event.SessionStarted{SpecName: "idle", Prompt: "ordinary idle", SubStateVersions: state.SubStateVersions()}},
		item{event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitInput}},
	)

	type row struct {
		ID        string            `json:"id"`
		Status    string            `json:"status"`
		Attention *sessionAttention `json:"attention"`
	}
	list := func() []row {
		t.Helper()
		var out, errOut bytes.Buffer
		if code := Run([]string{"sessions", "--json"}, "dev", &out, &errOut); code != ExitOK {
			t.Fatalf("sessions exit=%d stderr=%s", code, errOut.String())
		}
		var rows []row
		if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	find := func(rows []row, id string) row {
		t.Helper()
		for _, candidate := range rows {
			if candidate.ID == id {
				return candidate
			}
		}
		t.Fatalf("missing row %s in %+v", id, rows)
		return row{}
	}

	parent := find(list(), "parent")
	if parent.Status != "waiting:input" || parent.Attention == nil || parent.Attention.Answers != 2 || parent.Attention.Approvals != 2 {
		t.Fatalf("parent row = %+v", parent)
	}
	if idle := find(list(), "ordinary-idle"); idle.Attention != nil {
		t.Fatalf("ordinary idle attention = %+v, want nil", idle.Attention)
	}

	write(filepath.Join(parentDir, "sub", "c1-a1"),
		item{event.TypeApprovalResponded, &event.ApprovalResponded{ApprovalID: "apr-c1", Decision: "approve", Source: "test"}},
		item{event.TypeWaitingResolved, &event.WaitingResolved{Kind: event.WaitApproval, Resolution: "approved"}},
	)
	parent = find(list(), "parent")
	if parent.Attention == nil || parent.Attention.Approvals != 1 || parent.Attention.Answers != 2 {
		t.Fatalf("attention after child resolution = %+v", parent.Attention)
	}
	write(parentDir, item{event.TypeWaitingResolved, &event.WaitingResolved{Kind: event.WaitInput, Resolution: "answered"}})
	parent = find(list(), "parent")
	if parent.Attention == nil || parent.Attention.Approvals != 1 || parent.Attention.Answers != 1 {
		t.Fatalf("child answer attention after parent ask resolution = %+v", parent.Attention)
	}
	write(filepath.Join(parentDir, "sub", "c3-a1"),
		item{event.TypeWaitingResolved, &event.WaitingResolved{Kind: event.WaitInput, Resolution: "answered"}},
	)
	parent = find(list(), "parent")
	if parent.Attention == nil || parent.Attention.Approvals != 1 || parent.Attention.Answers != 0 {
		t.Fatalf("attention after child ask resolution = %+v", parent.Attention)
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"sessions"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("sessions exit=%d stderr=%s", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "parent") || !strings.Contains(got, "waiting:approval") {
		t.Fatalf("plain sessions output = %q", got)
	}
}

func TestCLISessionsRecentPageUsesFreshDescendantActivity(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	root := filepath.Join(xdg, "agentrunner", "sessions")
	appendEvents := func(dir string, values ...any) {
		t.Helper()
		es, err := store.OpenEventStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range values {
			switch value := value.(type) {
			case *event.SessionStarted:
				_, err = es.Append(mkEnv(t, event.TypeSessionStarted, value))
			case *event.SpawnRequested:
				_, err = es.Append(mkEnv(t, event.TypeSpawnRequested, value))
			case *event.WaitingEntered:
				_, err = es.Append(mkEnv(t, event.TypeWaitingEntered, value))
			default:
				t.Fatalf("unsupported event %T", value)
			}
			if err != nil {
				_ = es.Close()
				t.Fatal(err)
			}
		}
		if err := es.Close(); err != nil {
			t.Fatal(err)
		}
	}
	parentDir := filepath.Join(root, "old-parent")
	childSID := "old-parent-sub-ask-a1"
	appendEvents(parentDir,
		&event.SessionStarted{SpecName: "lead", Prompt: "old parent", SubStateVersions: state.SubStateVersions()},
		&event.SpawnRequested{CallID: "ask", Agent: "worker", ChildSession: childSID, Depth: 1},
	)
	childDetail, err := json.Marshal(struct {
		Questions []event.AskQuestion `json:"questions"`
	}{Questions: []event.AskQuestion{{
		Question: "Ship where?",
		Options:  []event.AskOption{{Label: "Stable"}, {Label: "Beta"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	childDir := filepath.Join(parentDir, "sub", "ask-a1")
	appendEvents(childDir,
		&event.SessionStarted{SpecName: "worker", SubStateVersions: state.SubStateVersions()},
		&event.WaitingEntered{Kind: event.WaitInput, Detail: childDetail},
	)

	base := time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(parentDir, "events.jsonl"), base, base); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		dir := filepath.Join(root, fmt.Sprintf("newer-%02d", i))
		appendEvents(dir, &event.SessionStarted{
			SpecName: "idle", Prompt: fmt.Sprintf("newer %02d", i), SubStateVersions: state.SubStateVersions(),
		})
		stamp := base.Add(time.Duration(i+1) * time.Second)
		if err := os.Chtimes(filepath.Join(dir, "events.jsonl"), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	childStamp := base.Add(2 * time.Minute)
	if err := os.Chtimes(filepath.Join(childDir, "events.jsonl"), childStamp, childStamp); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"sessions", "--json", "--limit", "40"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("sessions exit=%d stderr=%s", code, errOut.String())
	}
	var rows []struct {
		ID        string            `json:"id"`
		UpdatedAt string            `json:"updated_at"`
		Attention *sessionAttention `json:"attention"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID != "old-parent" {
			continue
		}
		if row.Attention == nil || row.Attention.Answers != 1 {
			t.Fatalf("old parent attention = %+v", row.Attention)
		}
		if row.UpdatedAt != childStamp.Format(time.RFC3339Nano) {
			t.Fatalf("updated_at = %q, want child activity %q", row.UpdatedAt, childStamp.Format(time.RFC3339Nano))
		}
		return
	}
	t.Fatalf("old parent with fresh child activity missing from recent page: %+v", rows)
}

func TestCLIResumeDriverReportsDomainError(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	dir := filepath.Join(xdg, "agentrunner", "sessions", "driver-stopped")
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		typ string
		v   any
	}{
		{event.TypeDriverStarted, &event.DriverStarted{DriverID: "driver-stopped", SpecName: "loop", FoldVersion: 1}},
		{event.TypeDriverCompleted, &event.DriverCompleted{DriverID: "driver-stopped", Reason: "stopped", Iterations: 2}},
	} {
		if _, err := es.Append(mkEnv(t, item.typ, item.v)); err != nil {
			t.Fatal(err)
		}
	}
	_ = es.Close()
	var out, errOut bytes.Buffer
	if code := Run([]string{"resume", "driver-stopped"}, "dev", &out, &errOut); code != ExitUsage {
		t.Fatalf("resume exit=%d stderr=%s", code, errOut.String())
	}
	if got := errOut.String(); !strings.Contains(got, "already ended (stopped)") || strings.Contains(got, "session_started") {
		t.Fatalf("driver resume error = %q", got)
	}
}

// INC-52 (HANDA-PARITY #14): `sessions list --json` surfaces the folded
// SessionTitled{auto} title in preference to the opening first line, so the
// webui shows the distilled title. A journal without the event falls back.
func TestCLISessionsJSONSurfacesAutoTitle(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	dir := filepath.Join(xdg, "agentrunner", "sessions", "sess-1")
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		typ string
		v   any
	}{
		{event.TypeSessionStarted, &event.SessionStarted{SpecName: "hello", Model: "m",
			Prompt:  "please refactor the auth boundary and add the missing regression tests",
			Version: "dev", SubStateVersions: state.SubStateVersions()}},
		{event.TypeInputReceived, &event.InputReceived{Text: "please refactor the auth boundary", Source: "user"}},
		{event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}},
		{event.TypeAssistantMessage, &event.AssistantMessage{GenStep: 1,
			Message: provider.Message{Role: provider.RoleAssistant,
				Parts: []provider.Part{{Kind: provider.PartText, Text: "on it"}}}}},
		{event.TypeSessionTitled, &event.SessionTitled{Title: "Refactor the auth boundary", Source: event.TitleSourceAuto}},
	} {
		if _, err := es.Append(mkEnv(t, item.typ, item.v)); err != nil {
			t.Fatal(err)
		}
	}
	if err := es.Close(); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"sessions", "--json"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("sessions exit=%d stderr=%s", code, errOut.String())
	}
	var rows []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Title != "Refactor the auth boundary" {
		t.Fatalf("sessions json = %+v, want auto title over the first line", rows)
	}
}

func TestCLITrust(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	ws := t.TempDir()
	var out, errOut bytes.Buffer
	if code := Run([]string{"trust", ws}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "trusted") {
		t.Errorf("out = %q", out.String())
	}
	if code := Run([]string{"trust"}, "dev", &out, &errOut); code != ExitUsage {
		t.Fatalf("no-arg exit = %d", code)
	}
}

// PLAN 3.1: `sessions --json` carries the ENGINE-computed cadence and next
// run — the webui consumes these fields verbatim instead of re-deriving
// them from the journal. A live interval driver gets both; a live series
// session (merged stream) gets the same projection; a terminal driver keeps
// its cadence but never a next run.
func TestCLISessionsJSONProjectsCadenceAndNextRun(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	// Live legacy interval driver.
	{
		dir := filepath.Join(xdg, "agentrunner", "sessions", "loop-live")
		es, err := store.OpenEventStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		spec, _ := json.Marshal(map[string]any{"Name": "loop", "Schedule": "interval", "Interval": "30m"})
		if _, err := es.Append(mkEnv(t, event.TypeDriverStarted, &event.DriverStarted{
			DriverID: "loop-live", SpecName: "loop", Spec: spec, FoldVersion: 1})); err != nil {
			t.Fatal(err)
		}
		if _, err := es.Append(mkEnv(t, event.TypeIterationScheduled, &event.IterationScheduled{
			DriverID: "loop-live", Iter: 1, Tick: time.Now().Add(-time.Minute)})); err != nil {
			t.Fatal(err)
		}
		_ = es.Close()
	}
	// Live merged-stream series session (cron).
	writeSeriesJournal(t, "series-live", "cron", false)
	// Terminal driver: cadence yes, next run never.
	{
		dir := filepath.Join(xdg, "agentrunner", "sessions", "loop-done")
		es, err := store.OpenEventStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		spec, _ := json.Marshal(map[string]any{"Name": "done", "Schedule": "interval", "Interval": "30m"})
		if _, err := es.Append(mkEnv(t, event.TypeDriverStarted, &event.DriverStarted{
			DriverID: "loop-done", SpecName: "done", Spec: spec, FoldVersion: 1})); err != nil {
			t.Fatal(err)
		}
		if _, err := es.Append(mkEnv(t, event.TypeDriverCompleted, &event.DriverCompleted{
			DriverID: "loop-done", Reason: "max_iterations", Iterations: 1})); err != nil {
			t.Fatal(err)
		}
		_ = es.Close()
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"sessions", "--json"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("sessions exit=%d stderr=%s", code, errOut.String())
	}
	var rows []struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		Schedule  string `json:"schedule"`
		Cadence   string `json:"cadence"`
		NextRunAt string `json:"next_run_at"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("decode: %v\n%s", err, out.String())
	}
	byID := map[string]int{}
	for i, r := range rows {
		byID[r.ID] = i
	}
	live := rows[byID["loop-live"]]
	if live.Cadence != "Every 30m" || live.NextRunAt == "" {
		t.Fatalf("live driver row = %+v, want cadence+next run", live)
	}
	series := rows[byID["series-live"]]
	if series.Kind != "driver" || series.Schedule != "cron" ||
		series.Cadence != "Hourly at :00" || series.NextRunAt == "" {
		t.Fatalf("series row = %+v, want driver kind + cron cadence + next run", series)
	}
	done := rows[byID["loop-done"]]
	if done.Cadence != "Every 30m" || done.NextRunAt != "" {
		t.Fatalf("terminal driver row = %+v, want cadence and NO next run", done)
	}
}
