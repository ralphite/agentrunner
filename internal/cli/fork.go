package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/ralphite/agentrunner/internal/fork"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/snapshot"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// forkCmd implements `agentrunner fork` (S7 模块 3): copy a session's
// barrier cut into a fresh session with its OWN worktree materialized from
// the barrier's snapshot. rewind = fork + the user explicitly switching to
// the fork and abandoning the original (DESIGN §fork/rewind) — there is no
// in-place rewind.
//
//	fork <session-id-or-prefix> --list
//	fork <session-id-or-prefix> <barrier-id> [--workspace <dir>]
func forkCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fork", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceDir := fs.String("workspace", "", "fork worktree dir (default: <parent-workspace>-fork-<id>; must be empty)")
	list := fs.Bool("list", false, "list the session's barriers and exit")
	if err := fs.Parse(reorderFlags(fs, args)); err != nil {
		return ExitUsage
	}
	rest := fs.Args()
	listing := *list && len(rest) == 1
	if len(rest) != 2 && !listing {
		fmt.Fprintln(stderr, "usage: agentrunner fork <session-id-or-prefix> <barrier-id> [--workspace <dir>]\n       agentrunner fork <session-id-or-prefix> --list")
		return ExitUsage
	}

	parentDir, err := resolveSessionDir(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	parentSession := filepath.Base(parentDir)
	events, err := store.ReadEvents(parentDir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	fold, err := state.Fold(events)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: fold %s: %v\n", parentSession, err)
		return ExitRun
	}

	if *list {
		if len(fold.Barriers) == 0 {
			fmt.Fprintln(stdout, "no barriers (run predates S7 or snapshots were unavailable)")
			return ExitOK
		}
		fmt.Fprintf(stdout, "%-12s %-6s %-6s %s\n", "BARRIER", "TURN", "SEQ", "SNAPSHOT")
		for _, b := range fold.Barriers {
			fmt.Fprintf(stdout, "%-12s %-6d %-6d %s\n", b.BarrierID, b.GenStep, b.Seq, short(b.SnapshotRef))
		}
		return ExitOK
	}

	barrierID := rest[1]
	var target *state.Barrier
	for i := range fold.Barriers {
		if fold.Barriers[i].BarrierID == barrierID {
			target = &fold.Barriers[i] // last wins: re-runs of bar-final shadow earlier ones
		}
	}
	if target == nil {
		fmt.Fprintf(stderr, "agentrunner: no barrier %q in %s (try --list)\n", barrierID, parentSession)
		return ExitUsage
	}

	// The parent's shadow store holds the snapshot; readSessionStarted resolves
	// the parent's REAL worktree even when the parent is itself a fork.
	started, err := readSessionStarted(parentDir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	if started.WorkspaceRoot == "" {
		fmt.Fprintf(stderr, "agentrunner: session %s predates resumable metadata\n", parentSession)
		return ExitRun
	}
	parentShadow, err := openShadow(started.WorkspaceRoot)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: parent snapshot store: %v\n", err)
		return ExitRun
	}

	newSession := runtime.NewSessionID(time.Now(), "fork "+barrierID)
	newDir, err := runtime.SessionDir(newSession)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	workRoot := *workspaceDir
	if workRoot == "" {
		workRoot = started.WorkspaceRoot + "-fork-" + newSession[len(newSession)-4:]
	}

	// Axis 1: materialize the fork's own worktree from the snapshot.
	ctx := context.Background()
	if err := parentShadow.Materialize(ctx, target.SnapshotRef, workRoot); err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	ws, err := workspace.New(workRoot)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}

	// Axis 2: copy the event cut.
	refs, err := fork.Cut(fork.Options{
		ParentDir:     parentDir,
		ParentSession: parentSession,
		NewDir:        newDir,
		NewSession:    newSession,
		Barrier:       *target,
		WorkspaceRoot: ws.Root(),
		Now:           time.Now(),
	})
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}

	// Pin the cut's snapshot refs in the fork workspace's own shadow store:
	// the fork's inherited barriers stay materializable without reaching
	// back into the parent's repo (fork-of-fork works).
	if forkShadow, serr := openShadow(ws.Root()); serr != nil {
		fmt.Fprintf(stderr, "warning: could not pin inherited snapshot refs: %v\n", serr)
	} else if serr := parentShadow.PushRefs(ctx, forkShadow.GitDir(), refs); serr != nil {
		fmt.Fprintf(stderr, "warning: could not pin inherited snapshot refs: %v\n", serr)
	}

	fmt.Fprintf(stderr, "forked %s @ %s\n", parentSession, barrierID)
	fmt.Fprintf(stdout, "session %s\nworkspace %s\n", newSession, ws.Root())
	fmt.Fprintf(stderr, "continue with: agentrunner resume %s\n", newSession)
	return ExitOK
}

// openShadow opens (initializing if needed) the shadow repo for a
// workspace root and insists on the real backend — fork cannot degrade.
func openShadow(root string) (*snapshot.ShadowRepo, error) {
	dir, err := shadowDirFor(root)
	if err != nil {
		return nil, err
	}
	st, err := snapshot.Open(dir, root)
	if err != nil {
		return nil, err
	}
	repo, ok := st.(*snapshot.ShadowRepo)
	if !ok {
		return nil, fmt.Errorf("snapshot backend unavailable (git missing?) — fork/rewind needs it")
	}
	return repo, nil
}

func short(ref string) string {
	if len(ref) > 12 {
		return ref[:12]
	}
	return ref
}
