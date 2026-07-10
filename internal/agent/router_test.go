package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// INC-12 安全 review P0: send_message's `to` reaches DirOf without the
// spawn-time safeCallIDRe guard. A `to` carrying path syntax must be
// REFUSED — InTree/DirOf are the write-side floor, not an audit afterthought.
func TestTreeRouterRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	rootDir := filepath.Join(root, "sessions", "team-root")
	if err := os.MkdirAll(filepath.Join(rootDir, "sub", "call_1-a1"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A sibling top-level session that a traversal would try to reach.
	victim := filepath.Join(root, "sessions", "victim")
	if err := os.MkdirAll(victim, 0o700); err != nil {
		t.Fatal(err)
	}
	r := NewTreeRouter("team-root", rootDir)

	// Legit member resolves.
	if _, err := r.DirOf("team-root-sub-call_1-a1"); err != nil {
		t.Fatalf("legit member rejected: %v", err)
	}
	// Traversals — every one must be refused BEFORE any inbox write.
	for _, bad := range []string{
		"team-root-sub-../../victim",
		"team-root-sub-../../../../../../tmp/evil",
		"team-root-sub-call_1-a1-sub-../../../victim",
		"team-root-sub-..",
		"team-root-sub-a/b",
	} {
		if r.InTree(bad) {
			t.Errorf("InTree accepted traversal %q", bad)
		}
		if _, err := r.DirOf(bad); err == nil {
			t.Errorf("DirOf resolved traversal %q (should refuse)", bad)
		}
		// Send must not create an inbox anywhere for a bad target.
		if _, err := r.Send(bad, protocol.UserInput{Text: "x", CommandID: "c-" + bad}); err == nil {
			t.Errorf("Send accepted traversal %q", bad)
		}
	}
	// The victim's inbox was never created (nothing escaped the tree).
	if _, err := os.Stat(filepath.Join(victim, "inbox.jsonl")); err == nil {
		t.Fatal("traversal wrote into a sibling session's inbox")
	}
	// Only the legit member's inbox dir exists under the tree root.
	entries, _ := os.ReadDir(filepath.Join(rootDir, "sub"))
	if len(entries) != 1 || entries[0].Name() != "call_1-a1" {
		t.Fatalf("unexpected dirs under root/sub: %v", entries)
	}
}

// INC-12 交互 review P1: a forked session copies sub/ verbatim, so an
// inherited child's journaled WorkspaceRoot is the ORIGINAL session's
// absolute path. childExecutorFromJournal must REFUSE to reopen a workspace
// outside this session (which would write into the original) — the fork
// isolation guard. Auto-revive then skips it; the mail stays durable.
func TestReviveRefusesForeignWorkspace(t *testing.T) {
	storeDir := t.TempDir()
	es, err := store.OpenEventStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })
	ws, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	l := &Loop{Store: es, SessionID: "lead", Spec: &AgentSpec{Name: "lead"},
		Exec: &tool.Executor{WS: ws}}

	// A child journal whose WorkspaceRoot points OUTSIDE this session's store
	// (as a fork's verbatim sub/ copy would carry the original absolute path).
	foreign := t.TempDir() // stands in for the ORIGINAL session's workspace
	childDir := filepath.Join(storeDir, "sub", "ext-a1")
	ces, err := store.OpenEventStore(childDir)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(&event.SessionStarted{
		SpecName: "member", Task: "t", WorkspaceRoot: foreign,
		Spec: json.RawMessage(`{"name":"member","model":{"provider":"scripted","id":"m"}}`),
	})
	if _, err := ces.Append(event.Envelope{Type: event.TypeSessionStarted, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	_ = ces.Close()

	// The guard refuses the foreign workspace (no workspace.New on it).
	if _, err := l.childExecutorFromJournal(childDir, "lead-sub-ext-a1"); !errors.Is(err, errForeignWorkspace) {
		t.Fatalf("childExecutorFromJournal(foreign) err = %v, want errForeignWorkspace", err)
	}

	// A child whose WorkspaceRoot is UNDER this store (a legit isolated
	// member of THIS tree) is accepted.
	localWT := filepath.Join(childDir, "worktree")
	if err := os.MkdirAll(localWT, 0o700); err != nil {
		t.Fatal(err)
	}
	localDir := filepath.Join(storeDir, "sub", "loc-a1")
	les, err := store.OpenEventStore(localDir)
	if err != nil {
		t.Fatal(err)
	}
	lp, _ := json.Marshal(&event.SessionStarted{
		SpecName: "member", Task: "t", WorkspaceRoot: filepath.Join(localDir, "worktree"),
		Spec: json.RawMessage(`{"name":"member","model":{"provider":"scripted","id":"m"}}`),
	})
	_, _ = les.Append(event.Envelope{Type: event.TypeSessionStarted, Payload: lp})
	_ = les.Close()
	if err := os.MkdirAll(filepath.Join(localDir, "worktree"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := l.childExecutorFromJournal(localDir, "lead-sub-loc-a1"); err != nil {
		t.Fatalf("legit isolated child rejected: %v", err)
	}
	// The foreign workspace was never created/written by the guard path.
	entries, _ := os.ReadDir(foreign)
	if len(entries) != 0 {
		t.Fatalf("guard wrote into the foreign workspace: %v", entries)
	}
}

// underDir must canonicalize symlinks (macOS /tmp → /private/tmp) and get the
// prefix boundary right (/a/bc is NOT under /a/b).
func TestUnderDir(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "sub", "x")
	_ = os.MkdirAll(sub, 0o700)
	cases := []struct {
		path, dir string
		want      bool
	}{
		{sub, base, true},
		{base, base, true},
		{filepath.Dir(base), base, false},
		{base + "-sibling", base, false}, // prefix-string trap
		{"/somewhere/else", base, false},
	}
	for _, c := range cases {
		if got := underDir(c.path, c.dir); got != c.want {
			t.Errorf("underDir(%q,%q)=%v want %v", c.path, c.dir, got, c.want)
		}
	}
}
