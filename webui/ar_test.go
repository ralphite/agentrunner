package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestParseSessionID(t *testing.T) {
	cases := []struct {
		name string
		res  arResult
		want string
	}{
		{
			// `ar new` announces the id on stderr; the reply is on stdout.
			name: "new: id on stderr",
			res: arResult{
				Stdout: "\n[gen-step 1]\n收到，请指示。\n",
				Stderr: "session 20260708-230920-task-5913\n(session 20260708-230920-task-5913 is waiting — continue: ...)\n",
			},
			want: "20260708-230920-task-5913",
		},
		{
			name: "fork: id on stdout",
			res:  arResult{Stdout: "session 20260708-231108-fork-ab12\n"},
			want: "20260708-231108-fork-ab12",
		},
		{
			// fork prints `forked <PARENT> @ <bar>` on stderr and
			// `session <NEW>` on stdout — the new id must win, not the parent.
			name: "fork: parent on stderr, new on stdout",
			res: arResult{
				Stdout: "session 20260709-024710-fork-bar-t1-df98\n",
				Stderr: "forked 20260708-224108-gin-gonic-gin-08e2 @ bar-t1\n",
			},
			want: "20260709-024710-fork-bar-t1-df98",
		},
		{
			name: "bare id anywhere",
			res:  arResult{Stderr: "created 20260708-010203-task-0001 ok"},
			want: "20260708-010203-task-0001",
		},
		{
			name: "none",
			res:  arResult{Stdout: "no session here"},
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseSessionID(c.res); got != c.want {
				t.Fatalf("parseSessionID = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDaemonUnreachable(t *testing.T) {
	unreachable := []string{
		"agentrunner: daemon dial: dial unix /x/daemon.sock: connect: no such file or directory",
		"error (no daemon running? start one with: agentrunner daemon --detach)",
		"failed: is the daemon running?",
	}
	for _, s := range unreachable {
		if !daemonUnreachable(s) {
			t.Errorf("expected unreachable for %q", s)
		}
	}
	reachable := []string{
		"no session matches \"__arwebui_probe__\"",
		"unknown session",
		"",
	}
	for _, s := range reachable {
		if daemonUnreachable(s) {
			t.Errorf("expected reachable for %q", s)
		}
	}
}

func TestValidID(t *testing.T) {
	// The "#<n>" collision-suffixed approval id must pass — a worker's
	// suffixed ask was un-answerable from the UI otherwise (QA Round4 F-K1).
	ok := []string{"20260708-230920-task-5913", "call_1_0", "bar-final", "a.b_c-1", "apr-eff-tool-call_1_0#2"}
	for _, s := range ok {
		if !validID(s) {
			t.Errorf("expected valid: %q", s)
		}
	}
	bad := []string{"", "a b", "x;y", "$(rm)", "a/b"}
	for _, s := range bad {
		if validID(s) {
			t.Errorf("expected invalid: %q", s)
		}
	}
}

func TestParseBarrierID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"barrier bar-m37\nsnapshot 1a2b3c4\n", "bar-m37"},
		{"snapshot 1a2b3c4\n", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := parseBarrierID(c.in); got != c.want {
			t.Errorf("parseBarrierID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMetaStoreMergeHydratesJournalMetadataWithoutReplacingTitle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	store := newMetaStore(path)
	store.set("s1", "", "My renamed task")
	store.merge(map[string]sessionMeta{
		"s1": {Workspace: "/tmp/project", Title: "Journal opening task"},
		"s2": {Workspace: "/tmp/other", Title: "External task"},
	})

	if got := store.get("s1"); got.Workspace != "/tmp/project" || got.Title != "My renamed task" {
		t.Fatalf("s1 metadata = %+v", got)
	}
	if got := store.get("s2"); got.Workspace != "/tmp/other" || got.Title != "External task" {
		t.Fatalf("s2 metadata = %+v", got)
	}

	reloaded := newMetaStore(path)
	if got := reloaded.get("s2"); got.Workspace != "/tmp/other" || got.Title != "External task" {
		t.Fatalf("reloaded metadata = %+v", got)
	}
}

// TestHandleDiffNestedWorkspace pins W1: a workspace that merely sits inside
// some parent repository (the shape of runtime/ws/* under a checkout) must
// report nested:true instead of a silent empty "no changes" diff.
func TestHandleDiffNestedWorkspace(t *testing.T) {
	parent := t.TempDir()
	mustGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mustGit(parent, "init", "-q")
	ws := filepath.Join(parent, "runtime", "ws", "ws-x")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}

	s := &server{meta: newMetaStore(filepath.Join(t.TempDir(), "meta.json"))}
	s.meta.set("20260710-000000-task-0001", ws, "t")

	req := httptest.NewRequest("GET", "/api/sessions/x/diff", nil)
	req.SetPathValue("sid", "20260710-000000-task-0001")
	rec := httptest.NewRecorder()
	s.handleDiff(rec, req)

	var resp struct {
		IsRepo   bool   `json:"isRepo"`
		Nested   bool   `json:"nested"`
		RepoRoot string `json:"repoRoot"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v\n%s", err, rec.Body.String())
	}
	if resp.IsRepo || !resp.Nested || resp.RepoRoot == "" {
		t.Fatalf("want nested workspace verdict, got %+v (body %s)", resp, rec.Body.String())
	}

	// After `git init` in the workspace itself it must count as a repo root,
	// and a file the agent wrote must surface as an untracked/new-file diff.
	mustGit(ws, "init", "-q")
	if err := os.WriteFile(filepath.Join(ws, "proof.txt"), []byte("UXR1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/api/sessions/x/diff", nil)
	req2.SetPathValue("sid", "20260710-000000-task-0001")
	s.handleDiff(rec2, req2)
	var resp2 struct {
		IsRepo bool   `json:"isRepo"`
		Nested bool   `json:"nested"`
		Diff   string `json:"diff"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatal(err)
	}
	if !resp2.IsRepo || resp2.Nested || !strings.Contains(resp2.Diff, "proof.txt") {
		t.Fatalf("want repo-root diff containing proof.txt, got %+v", resp2)
	}
}

// TestNewRuntimeDirNaming pins W2: auto-created workspaces get readable,
// sortable names (ws-YYYYMMDD-HHMMSS), not raw nanosecond stamps, and a
// same-second collision picks a -2 suffix instead of clobbering.
func TestNewRuntimeDirNaming(t *testing.T) {
	s := &server{runtimeDir: t.TempDir()}
	first := s.newRuntimeDir("ws", "ws")
	base := filepath.Base(first)
	if !regexp.MustCompile(`^ws-\d{8}-\d{6}$`).MatchString(base) {
		t.Fatalf("unexpected workspace name %q", base)
	}
	if err := os.MkdirAll(first, 0o755); err != nil {
		t.Fatal(err)
	}
	second := s.newRuntimeDir("ws", "ws")
	if second == first {
		t.Fatalf("collision not avoided: %q", second)
	}
	if !strings.HasPrefix(filepath.Base(second), base) {
		t.Fatalf("collision suffix should extend %q, got %q", base, second)
	}
}

func TestHandleGitBranchesDetachedDoesNotExposeHEADAsBranch(t *testing.T) {
	repo := t.TempDir()
	mustGit := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	mustGit(repo, "init", "-q", "-b", "main")
	mustGit(repo, "config", "user.name", "QA")
	mustGit(repo, "config", "user.email", "qa@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "proof.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(repo, "add", "proof.txt")
	mustGit(repo, "commit", "-q", "-m", "base")
	worktree := filepath.Join(t.TempDir(), "detached")
	mustGit(repo, "worktree", "add", "-q", "--detach", worktree, "main")

	s := &server{}
	req := httptest.NewRequest("GET", "/api/git/branches?dir="+worktree, nil)
	rec := httptest.NewRecorder()
	s.handleGitBranches(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Current  string   `json:"current"`
		Branches []string `json:"branches"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Current != "" || len(resp.Branches) == 0 || resp.Branches[0] != "main" {
		t.Fatalf("detached branch response = %+v", resp)
	}
}

func TestHandleWorktreeStartsAtSelectedRef(t *testing.T) {
	repo := t.TempDir()
	mustGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	mustGit("init", "-q", "-b", "main")
	mustGit("config", "user.name", "QA")
	mustGit("config", "user.email", "qa@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "proof.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit("add", "proof.txt")
	mustGit("commit", "-q", "-m", "main")
	mainHash := mustGit("rev-parse", "main")
	mustGit("checkout", "-q", "-b", "other")
	if err := os.WriteFile(filepath.Join(repo, "proof.txt"), []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit("commit", "-q", "-am", "other")

	s := &server{runtimeDir: t.TempDir()}
	body := bytes.NewBufferString(`{"repo":` + strconv.Quote(repo) + `,"ref":"main"}`)
	req := httptest.NewRequest("POST", "/api/worktree", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleWorktree(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", resp.Path, "rev-parse", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("worktree head: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != mainHash {
		t.Fatalf("worktree HEAD = %s, want main %s", got, mainHash)
	}
}
