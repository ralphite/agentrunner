package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
				Stderr: "session 20260708-230920-delegation-5913\n(session 20260708-230920-delegation-5913 is waiting — continue: ...)\n",
			},
			want: "20260708-230920-delegation-5913",
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
			res:  arResult{Stderr: "created 20260708-010203-delegation-0001 ok"},
			want: "20260708-010203-delegation-0001",
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
	ok := []string{"20260708-230920-delegation-5913", "call_1_0", "bar-final", "a.b_c-1", "apr-eff-tool-call_1_0#2"}
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

func TestHandleSessionsPaginationForwardsBoundedCLIArgs(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	arPath := filepath.Join(dir, "ar")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + strconv.Quote(argsPath) + "\nprintf '[]\\n'\n"
	if err := os.WriteFile(arPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &server{arPath: arPath, meta: newMetaStore(filepath.Join(dir, "meta.json"))}
	req := httptest.NewRequest("GET", "/api/sessions?limit=40&offset=80", nil)
	rr := httptest.NewRecorder()
	s.handleSessions(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	got, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "sessions\nlist\n--json\n--limit\n40\n--offset\n80\n" {
		t.Fatalf("args=%q", got)
	}

	for _, target := range []string{"/api/sessions?limit=-1", "/api/sessions?limit=501", "/api/sessions?offset=nope"} {
		rr = httptest.NewRecorder()
		s.handleSessions(rr, httptest.NewRequest("GET", target, nil))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("target=%s status=%d", target, rr.Code)
		}
	}
}

func TestHandlePSEmptyBackgroundWork(t *testing.T) {
	dir := t.TempDir()
	arPath := filepath.Join(dir, "ar")
	if err := os.WriteFile(arPath, []byte("#!/bin/sh\nprintf 'no background work in flight\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &server{arPath: arPath}
	req := httptest.NewRequest("GET", "/api/sessions/sess-1/ps", nil)
	req.SetPathValue("sid", "sess-1")
	rec := httptest.NewRecorder()
	s.handlePS(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got []map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("background work = %#v, want empty", got)
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
	store.set("s1", "", "My renamed prompt")
	store.merge(map[string]sessionMeta{
		"s1": {Workspace: "/tmp/project", Title: "Journal opening prompt"},
		"s2": {Workspace: "/tmp/other", Title: "External prompt"},
	})

	if got := store.get("s1"); got.Workspace != "/tmp/project" || got.Title != "My renamed prompt" {
		t.Fatalf("s1 metadata = %+v", got)
	}
	if got := store.get("s2"); got.Workspace != "/tmp/other" || got.Title != "External prompt" {
		t.Fatalf("s2 metadata = %+v", got)
	}

	reloaded := newMetaStore(path)
	if got := reloaded.get("s2"); got.Workspace != "/tmp/other" || got.Title != "External prompt" {
		t.Fatalf("reloaded metadata = %+v", got)
	}
}

// TestMetaStoreProjectOverlayRoundTrip pins INC-53: the workspace-keyed overlay
// (custom name / folded / last_opened) persists and reloads, and does not
// disturb the session cache. An emptied overlay entry is dropped.
func TestMetaStoreProjectOverlayRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	store := newMetaStore(path)
	store.set("s1", "/repo/app", "Opening prompt") // session cache coexists

	name := "My App"
	folded := true
	store.setProject("/repo/app", &name, &folded)
	store.touchProject("/repo/app")

	reloaded := newMetaStore(path)
	if got := reloaded.get("s1"); got.Workspace != "/repo/app" || got.Title != "Opening prompt" {
		t.Fatalf("session cache disturbed by overlay: %+v", got)
	}
	p := reloaded.allProjects()["/repo/app"]
	if p.DisplayName != "My App" || !p.Folded || p.LastOpened == 0 {
		t.Fatalf("overlay did not round-trip: %+v", p)
	}

	// Clearing name + unfolding leaves an all-default entry with only
	// last_opened; then reset last_opened's siblings and confirm an entirely
	// empty overlay entry is dropped.
	empty := ""
	unfold := false
	store2 := newMetaStore(path)
	store2.setProject("/repo/app", &empty, &unfold)
	got := store2.allProjects()["/repo/app"]
	if got.DisplayName != "" || got.Folded {
		t.Fatalf("clear did not revert name/fold: %+v", got)
	}
	// last_opened remains (it is a separate concern); a brand-new key that is
	// set then cleared should not linger.
	blank := ""
	no := false
	store2.setProject("/never/set", &blank, &no)
	if _, ok := store2.allProjects()["/never/set"]; ok {
		t.Fatalf("all-default overlay entry should be dropped")
	}
}

// TestMetaStoreLoadsLegacyFlatFile pins INC-53 backward compatibility: an old
// webui-meta.json (a bare sid→sessionMeta map, no wrapper) is still read, and
// a project overlay can be layered on top and persisted in the new format.
func TestMetaStoreLoadsLegacyFlatFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	legacy := map[string]sessionMeta{
		"20260709-071306-find-fn-39bd": {Workspace: "/repo/app", Title: "Legacy title"},
	}
	b, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}

	store := newMetaStore(path)
	if got := store.get("20260709-071306-find-fn-39bd"); got.Workspace != "/repo/app" || got.Title != "Legacy title" {
		t.Fatalf("legacy flat file not read: %+v", got)
	}
	name := "Renamed"
	store.setProject("/repo/app", &name, nil) // triggers a wrapper re-persist

	reloaded := newMetaStore(path)
	if got := reloaded.get("20260709-071306-find-fn-39bd"); got.Title != "Legacy title" {
		t.Fatalf("session cache lost after wrapper upgrade: %+v", got)
	}
	if p := reloaded.allProjects()["/repo/app"]; p.DisplayName != "Renamed" {
		t.Fatalf("overlay lost after wrapper upgrade: %+v", p)
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
	s.meta.set("20260710-000000-delegation-0001", ws, "t")

	req := httptest.NewRequest("GET", "/api/sessions/x/diff", nil)
	req.SetPathValue("sid", "20260710-000000-delegation-0001")
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
	if err := os.MkdirAll(filepath.Join(ws, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "node_modules", "pkg", "index.js"), []byte("generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "packages", "ui", "node_modules", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "packages", "ui", "node_modules", "nested", "index.js"), []byte("generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/api/sessions/x/diff", nil)
	req2.SetPathValue("sid", "20260710-000000-delegation-0001")
	s.handleDiff(rec2, req2)
	var resp2 struct {
		IsRepo          bool   `json:"isRepo"`
		Nested          bool   `json:"nested"`
		Diff            string `json:"diff"`
		HiddenUntracked int    `json:"hiddenUntracked"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatal(err)
	}
	if !resp2.IsRepo || resp2.Nested || !strings.Contains(resp2.Diff, "proof.txt") {
		t.Fatalf("want repo-root diff containing proof.txt, got %+v", resp2)
	}
	if resp2.HiddenUntracked != 2 || strings.Contains(resp2.Diff, "node_modules") {
		t.Fatalf("generated dependency files must be counted but not inlined: %+v", resp2)
	}
}

func TestHandleDiffLastTurn(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "fake-ar")
	script := `#!/bin/sh
if [ "$1" = "diff" ] && [ "$3" = "--scope" ] && [ "$4" = "last-turn" ] && [ "$5" = "--json" ]; then
  printf '%s\n' '{"scope":"last-turn","available":true,"workspace":"/tmp/project","input_seq":4,"barrier_seq":6,"barrier_id":"bar-t2","diff":"diff --git a/a.txt b/a.txt\\n+a","numstat":"1\\t0\\ta.txt"}'
  exit 0
fi
exit 2
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &server{arPath: bin}
	req := httptest.NewRequest("GET", "/api/sessions/x/diff?scope=last-turn", nil)
	req.SetPathValue("sid", "20260711-000000-delegation-0001")
	rec := httptest.NewRecorder()
	s.handleDiff(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["scope"] != "last-turn" || resp["available"] != true || resp["known"] != true ||
		!strings.Contains(resp["diff"].(string), "a.txt") {
		t.Fatalf("unexpected Last turn response: %#v", resp)
	}
	if untracked, ok := resp["untracked"].([]any); !ok || len(untracked) != 0 {
		t.Fatalf("untracked must be a stable empty array: %#v", resp["untracked"])
	}

	bad := httptest.NewRequest("GET", "/api/sessions/x/diff?scope=commit", nil)
	bad.SetPathValue("sid", "20260711-000000-delegation-0001")
	badRec := httptest.NewRecorder()
	s.handleDiff(badRec, bad)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid scope status = %d, want 400", badRec.Code)
	}
}

func TestHandleSessionFileDownloadConfinesWorkspace(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "docs", "report.md"), []byte("QA45_DOWNLOAD_OK\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(ws, "escape.txt")); err != nil {
		t.Fatal(err)
	}

	const id = "20260710-000000-delegation-0001"
	s := &server{meta: newMetaStore("")}
	s.meta.set(id, ws, "download")

	request := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/sessions/x/file?path="+path, nil)
		req.SetPathValue("sid", id)
		rec := httptest.NewRecorder()
		s.handleSessionFile(rec, req)
		return rec
	}

	good := request("docs%2Freport.md")
	if good.Code != http.StatusOK || good.Body.String() != "QA45_DOWNLOAD_OK\n" {
		t.Fatalf("good download = %d %q", good.Code, good.Body.String())
	}
	if got := good.Header().Get("Content-Disposition"); !strings.Contains(got, "report.md") {
		t.Fatalf("missing attachment filename: %q", got)
	}
	for name, path := range map[string]string{
		"absolute":       filepath.ToSlash(filepath.Join(ws, "docs", "report.md")),
		"traversal":      "..%2Fsecret.txt",
		"directory":      "docs",
		"symlink escape": "escape.txt",
	} {
		t.Run(name, func(t *testing.T) {
			if rec := request(path); rec.Code < 400 {
				t.Fatalf("unsafe path %q returned %d", path, rec.Code)
			}
		})
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

	s := &server{runtimeDir: t.TempDir(), worktreeDir: t.TempDir()}
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

func TestHandleWorktreeAcceptsSlashNamedBranch(t *testing.T) {
	repo := t.TempDir()
	mustGit := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
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

	s := &server{runtimeDir: t.TempDir(), worktreeDir: t.TempDir()}
	body := bytes.NewBufferString(`{"repo":` + strconv.Quote(repo) + `,"branch":"feature/proof"}`)
	req := httptest.NewRequest("POST", "/api/worktree", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleWorktree(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	mustGit("show-ref", "--verify", "refs/heads/feature/proof")
}

func TestVersionMatch(t *testing.T) {
	// Same commit stamp on both binaries: ar's "agentrunner <stamp> (go...)"
	// contains webui's bare stamp.
	if !versionMatch("a1b2c3d", "agentrunner a1b2c3d (go1.26.4)") {
		t.Error("expected match when ar version contains webui stamp")
	}
	// Plain dev builds must never false-alarm.
	if !versionMatch("dev", "agentrunner dev (go1.26.4)") {
		t.Error("expected dev builds to match")
	}
	// The exact regression: new webui, stale ar (different commit).
	if versionMatch("d230a93", "agentrunner ar-inc30 (go1.26.4)") {
		t.Error("expected skew to be flagged")
	}
	// Unrunnable ar (empty) is a mismatch.
	if versionMatch("d230a93", "") {
		t.Error("expected empty ar version to be a mismatch")
	}
}

func TestArFailFlagsStaleBinary(t *testing.T) {
	// The INC-43 regression: webui sent --steer to a pre-INC-43 ar, Go's flag
	// package rejected it with exit 2. The toast must name the stale binary.
	rec := httptest.NewRecorder()
	res := arResult{
		Stderr: "flag provided but not defined: -steer\nUsage of send:",
		Err:    fmt.Errorf("exit status 2"),
	}
	arFail(rec, "ar send", res)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	var body struct{ Stderr string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Stderr, "out of date") || !strings.Contains(body.Stderr, "scripts/deploy.sh") {
		t.Errorf("stale-binary hint missing from toast: %q", body.Stderr)
	}

	// A normal failure (no flag-parse error) must NOT get the hint.
	rec2 := httptest.NewRecorder()
	arFail(rec2, "ar send", arResult{Stderr: "no session matches \"x\"", Err: fmt.Errorf("exit status 1")})
	var body2 struct{ Stderr string }
	_ = json.Unmarshal(rec2.Body.Bytes(), &body2)
	if strings.Contains(body2.Stderr, "out of date") {
		t.Errorf("unexpected stale hint on ordinary failure: %q", body2.Stderr)
	}
}

// TestArFailNotFoundIsMachineReadable pins INC-41 L5: an unknown session id must
// arrive as a real 404 with a stable code, so the UI branches on semantics rather
// than grepping the CLI's prose (which would silently rot when the wording moves).
func TestArFailNotFoundIsMachineReadable(t *testing.T) {
	fail := func(res arResult) (int, map[string]string) {
		rec := httptest.NewRecorder()
		arFail(rec, "ar inspect", res)
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("bad json: %v\n%s", err, rec.Body.String())
		}
		return rec.Code, body
	}

	code, body := fail(arResult{
		Stderr: "agentrunner: no session matches \"ghost-9999\"\n",
		Err:    fmt.Errorf("exit status 2"),
	})
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if body["code"] != "session_not_found" {
		t.Fatalf("code = %q, want session_not_found", body["code"])
	}
	// stderr stays verbatim: the toast text and the old-binary fallback both read it.
	if !strings.Contains(body["stderr"], "no session matches") || body["error"] == "" {
		t.Fatalf("not-found body lost its detail: %#v", body)
	}

	// Some subcommands print the diagnostic on stdout (`ar new`) — same verdict.
	if code, body = fail(arResult{
		Stdout: "agentrunner: no session matches \"ghost-9999\"\n",
		Err:    fmt.Errorf("exit status 2"),
	}); code != http.StatusNotFound || body["code"] != "session_not_found" {
		t.Fatalf("stdout-carried verdict = %d %#v, want 404 session_not_found", code, body)
	}

	// The daemon being down is now its own friendly class: a 503 + a machine
	// code the UI turns into a "start the service" affordance, instead of the
	// raw "daemon dial:" blob on every action.
	code, body = fail(arResult{
		Stderr: "agentrunner: daemon dial: connect: no such file or directory\n",
		Err:    fmt.Errorf("exit status 1"),
	})
	if code != http.StatusServiceUnavailable {
		t.Fatalf("daemon-down status = %d, want 503", code)
	}
	if body["code"] != "daemon_down" {
		t.Fatalf("daemon-down code = %q, want daemon_down", body["code"])
	}

	// A genuinely ordinary failure keeps the existing 502 shape, with no code.
	code, body = fail(arResult{
		Stderr: "agentrunner: something else went wrong\n",
		Err:    fmt.Errorf("exit status 1"),
	})
	if code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", code)
	}
	if body["code"] != "" {
		t.Fatalf("ordinary failure must carry no code, got %q", body["code"])
	}
}

// --- INC-49: worktree productization (location, apply-back, cleanup, diff meta) ---

// wtRepo builds a git repo with one commit (a.txt="1\n") on branch main and
// returns the repo path plus a mustGit helper bound to it.
func wtRepo(t *testing.T) (string, func(dir string, args ...string) string) {
	t.Helper()
	mustGit := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	repo := t.TempDir()
	mustGit(repo, "init", "-q", "-b", "main")
	mustGit(repo, "config", "user.name", "QA")
	mustGit(repo, "config", "user.email", "qa@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(repo, "add", "a.txt")
	mustGit(repo, "commit", "-q", "-m", "init")
	return repo, mustGit
}

// mkWorktree drives handleWorktree and returns the created worktree path.
func mkWorktree(t *testing.T, s *server, repo, body string) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/worktree", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleWorktree(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("worktree add status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Path, Repo, Branch string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Path
}

func TestWorktreeInDataDir(t *testing.T) {
	repo, mustGit := wtRepo(t)
	head := mustGit(repo, "rev-parse", "HEAD")
	wtRoot := t.TempDir()
	s := &server{worktreeDir: wtRoot}
	path := mkWorktree(t, s, repo, `{"repo":`+strconv.Quote(repo)+`,"ref":"main"}`)

	if !strings.HasPrefix(path, wtRoot+string(filepath.Separator)) {
		t.Fatalf("worktree %s not under shared root %s", path, wtRoot)
	}
	if base := filepath.Base(path); !strings.Contains(base, "-main-") {
		t.Fatalf("worktree name %q should record the ref label 'main'", base)
	}
	if got := mustGit(path, "rev-parse", "HEAD"); got != head {
		t.Fatalf("worktree HEAD = %s, want %s", got, head)
	}
}

func TestApplyBackCleanApply(t *testing.T) {
	repo, mustGit := wtRepo(t)
	s := &server{worktreeDir: t.TempDir(), meta: newMetaStore("")}
	wt := mkWorktree(t, s, repo, `{"repo":`+strconv.Quote(repo)+`,"ref":"main"}`)
	// Edit a tracked file and add an untracked one inside the worktree.
	if err := os.WriteFile(filepath.Join(wt, "a.txt"), []byte("1\n2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := "20260710-000000-apply-0001"
	s.meta.set(id, wt, "t")

	req := httptest.NewRequest("POST", "/api/sessions/x/apply", nil)
	req.SetPathValue("sid", id)
	rec := httptest.NewRecorder()
	s.handleApply(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply status %d: %s", rec.Code, rec.Body.String())
	}
	// Main checkout working tree now carries both changes.
	if got, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(got) != "1\n2\n" {
		t.Fatalf("main a.txt = %q, want applied edit", got)
	}
	if got, _ := os.ReadFile(filepath.Join(repo, "new.txt")); string(got) != "new\n" {
		t.Fatalf("main new.txt = %q, want applied new file", got)
	}
	// Applied unstaged: the new file shows as untracked (??) in the main repo.
	if st := mustGit(repo, "status", "--porcelain"); !strings.Contains(st, "?? new.txt") {
		t.Fatalf("expected new.txt unstaged in main repo, status:\n%s", st)
	}
	// The worktree must be left in its pre-apply UNSTAGED state (apply restores the
	// index after staging), so the Changes view still shows the changes — a staged
	// "A  new.txt" here would mean the Changes panel blanks out right after Apply.
	if st := mustGit(wt, "status", "--porcelain"); !strings.Contains(st, "?? new.txt") || strings.Contains(st, "A  new.txt") {
		t.Fatalf("worktree should be left unstaged after apply, status:\n%s", st)
	}
}

func TestApplyBackConflictReported(t *testing.T) {
	repo, mustGit := wtRepo(t)
	s := &server{worktreeDir: t.TempDir(), meta: newMetaStore("")}
	wt := mkWorktree(t, s, repo, `{"repo":`+strconv.Quote(repo)+`,"ref":"main"}`)
	// Diverge the same line in both trees so the patch cannot apply cleanly.
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("MAIN\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "a.txt"), []byte("WT\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := "20260710-000000-conflict-0001"
	s.meta.set(id, wt, "t")

	req := httptest.NewRequest("POST", "/api/sessions/x/apply", nil)
	req.SetPathValue("sid", id)
	rec := httptest.NewRecorder()
	s.handleApply(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("apply status %d, want 409 conflict: %s", rec.Code, rec.Body.String())
	}
	// The main working tree must be left exactly as it was — no half-merge.
	if got, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(got) != "MAIN\n" {
		t.Fatalf("main a.txt = %q, want untouched MAIN\\n after conflict", got)
	}
	_ = mustGit
}

func TestWorktreeRemoveGuardsDirty(t *testing.T) {
	repo, mustGit := wtRepo(t)
	s := &server{worktreeDir: t.TempDir(), meta: newMetaStore("")}
	wt := mkWorktree(t, s, repo, `{"repo":`+strconv.Quote(repo)+`,"ref":"main"}`)
	if err := os.WriteFile(filepath.Join(wt, "dirty.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := "20260710-000000-remove-0001"
	s.meta.set(id, wt, "t")

	call := func(force bool) *httptest.ResponseRecorder {
		body := `{"force":` + strconv.FormatBool(force) + `}`
		req := httptest.NewRequest("POST", "/api/sessions/x/worktree/remove", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetPathValue("sid", id)
		rec := httptest.NewRecorder()
		s.handleWorktreeRemove(rec, req)
		return rec
	}
	// Without force, a dirty worktree is refused with a structured dirty signal.
	rec := call(false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("remove(no force) status %d, want 409: %s", rec.Code, rec.Body.String())
	}
	var g struct{ Dirty bool }
	_ = json.Unmarshal(rec.Body.Bytes(), &g)
	if !g.Dirty {
		t.Fatalf("want dirty:true, body %s", rec.Body.String())
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("worktree should still exist after refused remove: %v", err)
	}
	// With force it is removed and pruned from the registry.
	rec = call(true)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove(force) status %d: %s", rec.Code, rec.Body.String())
	}
	if list := mustGit(repo, "worktree", "list", "--porcelain"); strings.Contains(list, wt) {
		t.Fatalf("worktree still listed after force remove:\n%s", list)
	}
}

func TestDiffReportsWorktreeMeta(t *testing.T) {
	repo, _ := wtRepo(t)
	s := &server{worktreeDir: t.TempDir(), meta: newMetaStore("")}
	wt := mkWorktree(t, s, repo, `{"repo":`+strconv.Quote(repo)+`,"ref":"main"}`)

	get := func(ws string) map[string]any {
		id := "20260710-000000-diffmeta-" + filepath.Base(ws)
		s.meta.set(id, ws, "t")
		req := httptest.NewRequest("GET", "/api/sessions/x/diff", nil)
		req.SetPathValue("sid", id)
		rec := httptest.NewRecorder()
		s.handleDiff(rec, req)
		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("bad json: %v\n%s", err, rec.Body.String())
		}
		return m
	}
	wtResp := get(wt)
	if wtResp["worktree"] != true {
		t.Fatalf("worktree diff should report worktree:true, got %v", wtResp["worktree"])
	}
	if mr, _ := wtResp["mainRepo"].(string); !samePath(mr, repo) {
		t.Fatalf("mainRepo = %q, want %s", mr, repo)
	}
	mainResp := get(repo)
	if mainResp["worktree"] == true {
		t.Fatalf("main checkout should report worktree:false, got %v", mainResp["worktree"])
	}
}

// TestSanitizeStagedPaths pins that a spec-load error from a content-submitted
// agent switch never leaks the internal staging path (QA Wave7 pat-04).
func TestSanitizeStagedPaths(t *testing.T) {
	base := "/home/user/agentrunner/runtime/specs/s4242/base.yaml"
	res := arResult{
		Stderr: "agentrunner: spec " + base + ": field tools: unknown tool \"flibber\"",
		Stdout: "see " + base + " and /home/user/agentrunner/runtime/specs/s4242/worker.yaml",
	}
	got := sanitizeStagedPaths(res, base)
	for _, s := range []string{got.Stderr, got.Stdout} {
		if strings.Contains(s, "runtime/specs") || strings.Contains(s, base) {
			t.Fatalf("staging path leaked: %q", s)
		}
	}
	if !strings.Contains(got.Stderr, "field tools: unknown tool") {
		t.Fatalf("stderr lost its actionable content: %q", got.Stderr)
	}
	// A sibling reference keeps its bare filename.
	if !strings.Contains(got.Stdout, "worker.yaml") {
		t.Fatalf("sibling name lost: %q", got.Stdout)
	}
}

// TestHandleArtifactNotFound pins that a missing artifact stream/version is a
// 404 (resource not found), not the 400 an exit-2 usage error would map to —
// matching the missing-session 404 (QA Wave7 pat-01).
func TestHandleArtifactNotFound(t *testing.T) {
	dir := t.TempDir()
	arPath := filepath.Join(dir, "ar")
	// Emit the CLI's stream-not-found error on stderr and exit 2.
	script := "#!/bin/sh\n" +
		"printf 'agentrunner: no published artifact stream \"nope\" (try: artifacts list)\\n' >&2\n" +
		"exit 2\n"
	if err := os.WriteFile(arPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &server{arPath: arPath}
	req := httptest.NewRequest("GET", "/api/sessions/sess-1/artifact?stream=nope", nil)
	req.SetPathValue("sid", "sess-1")
	rec := httptest.NewRecorder()
	s.handleArtifact(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s, want 404", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "artifact_not_found" {
		t.Fatalf("code=%q, want artifact_not_found", body["code"])
	}
}

// TestHandleStreamNotFound pins that the SSE stream endpoint 404s a nonexistent
// session instead of opening a 200 stream that ends with attach-exited (QA
// Wave7 pat-02).
func TestHandleStreamNotFound(t *testing.T) {
	dir := t.TempDir()
	arPath := filepath.Join(dir, "ar")
	// `ar ps <id>` prints "no session matches" for an unknown id (sessionExists
	// reads that from stderr).
	script := "#!/bin/sh\nprintf 'agentrunner: no session matches \"%s\"\\n' \"$2\" >&2\nexit 2\n"
	if err := os.WriteFile(arPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &server{arPath: arPath}
	req := httptest.NewRequest("GET", "/api/sessions/nope-1/stream", nil)
	req.SetPathValue("sid", "nope-1")
	rec := httptest.NewRecorder()
	s.handleStream(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s, want 404", rec.Code, rec.Body.String())
	}
}

// TestContentTypeCaseInsensitive pins that a case-variant application/json
// Content-Type is accepted (RFC 7231 media types are case-insensitive) while a
// non-JSON type is still rejected (QA Wave2 carol-10).
func TestContentTypeCaseInsensitive(t *testing.T) {
	check := func(ct string, wantOK bool) {
		req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"a":1}`))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		rec := httptest.NewRecorder()
		var v map[string]any
		ok := readBody(rec, req, &v)
		if ok != wantOK {
			t.Fatalf("Content-Type %q: readBody ok=%v, want %v (status %d)", ct, ok, wantOK, rec.Code)
		}
	}
	check("application/json", true)
	check("Application/JSON", true)
	check("application/JSON; charset=utf-8", true)
	check("text/plain", false)
}

// TestSendDeliveryValidation pins that the send endpoint rejects an unknown
// delivery mode instead of silently queueing it (QA Wave2 carol-09).
func TestSendDeliveryValidation(t *testing.T) {
	s := &server{arPath: "/nonexistent-ar"}
	body := `{"text":"hi","delivery":"steal"}`
	req := httptest.NewRequest("POST", "/api/sessions/s1/send", strings.NewReader(body))
	req.SetPathValue("sid", "s1")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleSend(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown delivery: status=%d body=%s, want 400", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "delivery must be") {
		t.Fatalf("body=%s, want a delivery error", rec.Body.String())
	}
}
