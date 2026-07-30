package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// promoteCmd applies a finished best-of-N series' WINNER onto the project
// workspace (PLAN 5.8, "Apply winner" — the session-shaped complement of
// INC-49's worktree Apply-to-project). The winner attempt's tree lives at
// <session>/wt/att-<N>, materialized from the round's pinned base snapshot
// (Series.BaseRef); the patch is base→winner, produced in the workspace's
// shadow repo, and lands clean-or-nothing and UNSTAGED on the workspace —
// a conflict is reported verbatim with the tree untouched (INC-49 教义).
func promoteCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner promote <best-of-N-session-id-or-prefix>")
		return ExitUsage
	}
	dir, err := resolveSessionDir(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	session := filepath.Base(dir)
	events, err := store.ReadEvents(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	s, err := state.Fold(events)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: fold %s: %v\n", session, err)
		return ExitRun
	}
	sr := s.Series
	switch {
	case sr == nil || sr.Kind != "best_of_n":
		fmt.Fprintf(stderr, "agentrunner: %s is not a best-of-N session\n", session)
		return ExitUsage
	case !sr.Ended:
		fmt.Fprintf(stderr, "agentrunner: best-of-N round %s is still running — promote after it finishes\n", session)
		return ExitUsage
	case sr.BestIter == 0:
		fmt.Fprintf(stderr, "agentrunner: %s finished without a usable attempt (no winner to apply)\n", session)
		return ExitRun
	case sr.BaseRef == "":
		fmt.Fprintf(stderr, "agentrunner: %s predates the pinned base snapshot; cannot compute the winner's changes\n", session)
		return ExitRun
	}
	winner := filepath.Join(dir, "wt", fmt.Sprintf("att-%d", sr.BestIter))
	if st, werr := os.Stat(winner); werr != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "agentrunner: winner worktree %s is gone (cleaned up?); nothing to apply\n", winner)
		return ExitRun
	}
	started, err := readSessionStarted(dir)
	if err != nil || started.WorkspaceRoot == "" {
		fmt.Fprintf(stderr, "agentrunner: session %s has no workspace root in its journal\n", session)
		return ExitRun
	}
	shadowData, err := shadowDirFor(started.WorkspaceRoot)
	shadow := filepath.Join(shadowData, "shadow.git") // snapshot.Open's layout
	if err == nil {
		if _, serr := os.Stat(shadow); serr != nil {
			err = fmt.Errorf("shadow snapshot store missing at %s", shadow)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	applied, out, err := promoteWinner(context.Background(), shadow, sr.BaseRef, winner, started.WorkspaceRoot)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n%s\n", err, out)
		return ExitRun
	}
	if len(applied) == 0 {
		fmt.Fprintf(stdout, "winner attempt %d made no changes over the base — nothing to apply\n", sr.BestIter)
		return ExitOK
	}
	fmt.Fprintf(stdout, "applied winner attempt %d to %s:\n%s\n", sr.BestIter, started.WorkspaceRoot, strings.Join(applied, "\n"))
	fmt.Fprintln(stderr, "changes are unstaged — review and commit them in your own checkout")
	return ExitOK
}

// promoteWinner produces the base→winner binary patch inside the shadow
// git-dir (a temp index over the winner tree; the workspace's own .git is
// never involved) and applies it clean-or-nothing onto root. Returns the
// applied file list; an empty list means the winner equals the base.
func promoteWinner(ctx context.Context, shadowDir, baseRef, winnerDir, root string) (applied []string, output string, err error) {
	idx, err := os.CreateTemp("", "ar-promote-index-*")
	if err != nil {
		return nil, "", err
	}
	idxPath := idx.Name()
	_ = idx.Close()
	_ = os.Remove(idxPath) // git wants to create it itself
	defer func() { _ = os.Remove(idxPath) }()

	env := append(os.Environ(), "GIT_INDEX_FILE="+idxPath)
	git := func(dir string, stdin []byte, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		cmd.Env = env
		if stdin != nil {
			cmd.Stdin = strings.NewReader(string(stdin))
		}
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	gitShadow := func(stdin []byte, args ...string) (string, error) {
		full := append([]string{"--git-dir=" + shadowDir, "--work-tree=" + winnerDir}, args...)
		return git(winnerDir, stdin, full...)
	}

	// Winner tree → object DB via a throwaway index (the shadow's real index
	// and the workspace stay untouched). The shadow's info/exclude keeps the
	// same credential hard-excludes every snapshot honors.
	if out, err := gitShadow(nil, "add", "-A"); err != nil {
		return nil, out, fmt.Errorf("promote: git add on winner tree failed: %w", err)
	}
	tree, err := gitShadow(nil, "write-tree")
	if err != nil {
		return nil, tree, fmt.Errorf("promote: git write-tree failed: %w", err)
	}
	patch, err := gitShadow(nil, "diff", "--binary", baseRef, tree)
	if err != nil {
		return nil, patch, fmt.Errorf("promote: git diff base→winner failed: %w", err)
	}
	if strings.TrimSpace(patch) == "" {
		return nil, "", nil
	}
	files, _ := gitShadow(nil, "diff", "--name-only", baseRef, tree)
	pb := []byte(patch + "\n")
	// Clean-or-nothing on the workspace: dry-run first, report a conflict
	// verbatim with nothing changed (INC-49 discipline). Plain `git apply`
	// works with or without the workspace being a git repository.
	if out, err := git(root, pb, "apply", "--check", "-"); err != nil {
		return nil, out, fmt.Errorf("promote: winner does not apply cleanly onto %s (the workspace diverged from the round's base) — resolve and retry", root)
	}
	if out, err := git(root, pb, "apply", "-"); err != nil {
		return nil, out, fmt.Errorf("promote: git apply failed: %w", err)
	}
	for _, f := range strings.Split(files, "\n") {
		if f = strings.TrimSpace(f); f != "" {
			applied = append(applied, f)
		}
	}
	return applied, "", nil
}
