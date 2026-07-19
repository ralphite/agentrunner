package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/snapshot"
)

// promoteFixture builds a workspace + shadow snapshot (the base), and a
// winner directory = base tree + one edit + one new file.
func promoteFixture(t *testing.T) (shadowDir, baseRef, winner, root string) {
	t.Helper()
	root = t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	shadowDir = filepath.Join(t.TempDir(), "shadow.git")
	st, err := snapshot.NewShadowRepo(shadowDir, root)
	if err != nil {
		t.Fatal(err)
	}
	baseRef, err = st.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	winner = filepath.Join(t.TempDir(), "att-2")
	if err := st.Materialize(context.Background(), baseRef, winner); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(winner, "main.go"), []byte("package main\n\nfunc win() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(winner, "NEW.md"), []byte("winner artifact\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return shadowDir, baseRef, winner, root
}

// PLAN 5.8 twin: the winner's base→tree patch lands clean-or-nothing and
// unstaged on the workspace — edits and new files both arrive, and the
// workspace needs no .git of its own.
func TestPromoteWinnerCleanApply(t *testing.T) {
	shadowDir, baseRef, winner, root := promoteFixture(t)
	applied, out, err := promoteWinner(context.Background(), shadowDir, baseRef, winner, root)
	if err != nil {
		t.Fatalf("promote failed: %v\n%s", err, out)
	}
	if len(applied) != 2 {
		t.Fatalf("applied = %v, want main.go + NEW.md", applied)
	}
	got, _ := os.ReadFile(filepath.Join(root, "main.go"))
	if !strings.Contains(string(got), "func win()") {
		t.Fatalf("workspace main.go missing winner edit: %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "NEW.md")); err != nil {
		t.Fatal("winner's new file did not arrive in the workspace")
	}
}

// A workspace that diverged from the round's base refuses the patch verbatim
// and is left byte-for-byte untouched (clean-or-nothing, INC-49 教义).
func TestPromoteWinnerConflictLeavesWorkspaceUntouched(t *testing.T) {
	shadowDir, baseRef, winner, root := promoteFixture(t)
	diverged := []byte("package main // diverged since the round began\n")
	if err := os.WriteFile(filepath.Join(root, "main.go"), diverged, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := promoteWinner(context.Background(), shadowDir, baseRef, winner, root)
	if err == nil || !strings.Contains(err.Error(), "does not apply cleanly") {
		t.Fatalf("err = %v, want clean-apply refusal", err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "main.go"))
	if string(got) != string(diverged) {
		t.Fatal("conflict path modified the workspace")
	}
	if _, serr := os.Stat(filepath.Join(root, "NEW.md")); serr == nil {
		t.Fatal("conflict path half-applied the new file")
	}
}

// An identical winner (no changes over the base) is a graceful no-op.
func TestPromoteWinnerNoChanges(t *testing.T) {
	shadowDir, baseRef, _, root := promoteFixture(t)
	same := filepath.Join(t.TempDir(), "att-1")
	st, err := snapshot.NewShadowRepo(shadowDir, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Materialize(context.Background(), baseRef, same); err != nil {
		t.Fatal(err)
	}
	applied, out, err := promoteWinner(context.Background(), shadowDir, baseRef, same, root)
	if err != nil {
		t.Fatalf("no-op promote failed: %v\n%s", err, out)
	}
	if len(applied) != 0 {
		t.Fatalf("applied = %v, want none", applied)
	}
}
