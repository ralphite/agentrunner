package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/protocol"
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
