package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A repo with CLAUDE.md at the git root, an intermediate level, and the
// workspace: all three collect, outermost first (nearest renders last, so
// nearest wins for the model).
func TestCollectHierarchyOrder(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	ws := filepath.Join(repo, "services", "api")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(dir, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(repo, "repo-level rules")
	write(filepath.Join(repo, "services"), "services rules")
	write(ws, "api rules")

	files, err := Collect(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("files = %+v", files)
	}
	if !strings.Contains(files[0].Content, "repo-level") ||
		!strings.Contains(files[1].Content, "services") ||
		!strings.Contains(files[2].Content, "api rules") {
		t.Errorf("order wrong (want outermost first): %+v", files)
	}

	block := Render(files, ws)
	// Nearest content renders last.
	if strings.Index(block, "repo-level") > strings.Index(block, "api rules") {
		t.Errorf("nearest must render last:\n%s", block)
	}
	for _, want := range []string{"<memory>", "</memory>"} {
		if !strings.Contains(block, want) {
			t.Errorf("block missing %q", want)
		}
	}
}

// Without an enclosing git repo, only the workspace root's own CLAUDE.md
// counts — the walk must not hoover up unrelated ancestors.
func TestCollectNoRepoStopsAtRoot(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "CLAUDE.md"), []byte("ws rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := Collect(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !strings.Contains(files[0].Content, "ws rules") {
		t.Fatalf("files = %+v, want only the workspace's own", files)
	}
}

func TestRenderEmpty(t *testing.T) {
	if Render(nil, ".") != "" {
		t.Error("no files must render no block")
	}
}
