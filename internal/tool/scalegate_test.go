package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/workspace"
)

func wsAtScale(t *testing.T, root string, large bool) *workspace.Workspace {
	t.Helper()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetScale(0, large)
	return ws
}

// Reaching keyword_search in a large workspace means a replayed or forced call
// (the model is not advertised it). Building the index is the ONE outcome the
// gate exists to prevent, so the executor must refuse — and must name the
// bounded alternatives so a model that got here recovers instead of retrying.
func TestKeywordSearchRefusesLargeWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\nfunc Alpha() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := &Executor{WS: wsAtScale(t, root, true)}

	res := e.keywordSearch(json.RawMessage(`{"query":"Alpha"}`))
	if !res.IsError {
		t.Fatalf("expected an error result, got %+v", res)
	}
	body := strings.ToLower(string(res.Payload))
	for _, want := range []string{"grep", "glob"} {
		if !strings.Contains(body, want) {
			t.Errorf("error must point at %q; got %q", want, body)
		}
	}
	// The index must not have been built as a side effect.
	if e.index != nil {
		t.Error("refusal must not build the index")
	}
}

// The same call in a normal workspace still works — the gate is the only thing
// that changed, not the tool.
func TestKeywordSearchWorksWhenNotLarge(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\nfunc AlphaBeta() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := &Executor{WS: wsAtScale(t, root, false)}
	if res := e.keywordSearch(json.RawMessage(`{"query":"AlphaBeta"}`)); res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
}

// The credential scan used to descend .git and node_modules in full on EVERY
// bash call. It must now prune in lockstep with the index walk while still
// finding real credential-shaped files.
func TestCredentialPathsPrunesDerivedTrees(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mk(".env")                        // must be denied
	mk("sub/deploy.pem")              // must be denied
	mk("node_modules/pkg/secret.pem") // pruned tree — must NOT appear
	mk(".git/config.key")             // pruned tree — must NOT appear
	mk("src/main.go")                 // not credential-shaped

	got := credentialPaths(root)
	var found []string
	for _, d := range got {
		rel, _ := filepath.Rel(root, d.Path)
		found = append(found, filepath.ToSlash(rel))
	}
	for _, want := range []string{".env", "sub/deploy.pem"} {
		if !hasPath(found, want) {
			t.Errorf("%s must be denied; got %v", want, found)
		}
	}
	for _, unwanted := range []string{"node_modules/pkg/secret.pem", ".git/config.key"} {
		if hasPath(found, unwanted) {
			t.Errorf("%s is inside a pruned tree and must not be walked; got %v", unwanted, found)
		}
	}
	if hasPath(found, "src/main.go") {
		t.Errorf("non-credential file must not be denied; got %v", found)
	}
}

// A .ssh directory is still denied wholesale (subpath), not per-file.
func TestCredentialPathsDeniesSSHSubtree(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	got := credentialPaths(root)
	if len(got) != 1 || !got[0].Subpath {
		t.Fatalf("want one subpath deny for .ssh, got %+v", got)
	}
}

// The deny list is memoized: it used to be a full tree walk per bash call.
func TestCredentialDeniesMemoized(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("K=v"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &Executor{WS: wsAtScale(t, root, false)}

	first := e.credentialDenies(root)
	if len(first) != 1 {
		t.Fatalf("want 1 deny, got %d", len(first))
	}
	// A file appearing after the first call must not change the memoized answer
	// — that is the documented trade, and asserting it keeps the behavior honest.
	if err := os.WriteFile(filepath.Join(root, "later.pem"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if second := e.credentialDenies(root); len(second) != len(first) {
		t.Errorf("deny list must be memoized: %d then %d", len(first), len(second))
	}
}

// The cap exists because every entry becomes an argv line for sandbox-exec:
// past ARG_MAX an unbounded list fails the command outright with E2BIG.
func TestCredentialPathsCapped(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < maxCredentialDenies+50; i++ {
		p := filepath.Join(root, "k"+itoa(i)+".pem")
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(credentialPaths(root)); got > maxCredentialDenies {
		t.Errorf("deny list = %d entries, must not exceed the %d cap", got, maxCredentialDenies)
	}
}

func hasPath(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
