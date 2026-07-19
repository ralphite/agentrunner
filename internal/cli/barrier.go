package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// barrierCmd is the EXPLICIT barrier entry point (S7 模块 2 承诺, 兑现于
// 出口 review): `agentrunner barrier <session-id-or-prefix>` cuts a
// CheckpointBarrier on a session that is not currently running — snapshot
// of the workspace as it stands now, cut vector at the journal head — so
// fork/rewind can target the present state, not just turn boundaries. The
// event-store flock refuses a session a live process still holds.
func barrierCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner barrier <session-id-or-prefix>")
		return ExitUsage
	}
	dir, err := resolveSessionDir(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	session := filepath.Base(dir)

	es, err := store.OpenEventStore(dir)
	if err != nil {
		// Name the way out, not just the lock (QA Round1 F-B5): the flow is
		// stop → barrier → fork, and nothing else surfaces it.
		fmt.Fprintf(stderr, "agentrunner: %v (a live session cannot be barriered externally — quiesce it first: agentrunner stop %s)\n", err, session)
		return ExitRun
	}
	defer func() { _ = es.Close() }()

	events, err := store.ReadEvents(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	fold, err := state.Fold(events)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: fold %s: %v\n", session, err)
		return ExitRun
	}
	started, err := readSessionStarted(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	if started.WorkspaceRoot == "" {
		fmt.Fprintf(stderr, "agentrunner: session %s predates resumable metadata\n", session)
		return ExitRun
	}
	shadow, err := openShadow(started.WorkspaceRoot)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	ref, err := shadow.Snapshot(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}

	vector := map[string]int64{".": es.LastSeq()}
	for _, child := range fold.Session.ChildSessions {
		idx := strings.LastIndex(child, "-sub-")
		if idx < 0 {
			continue
		}
		suffix := child[idx+len("-sub-"):]
		childEvents, rerr := store.ReadEvents(filepath.Join(dir, "sub", suffix))
		if rerr != nil || len(childEvents) == 0 {
			continue
		}
		vector["sub/"+suffix] = childEvents[len(childEvents)-1].Seq
	}
	var handles []event.BarrierHandle
	for id := range fold.Handles {
		handles = append(handles, event.BarrierHandle{Handle: id})
	}
	sort.Slice(handles, func(i, j int) bool { return handles[i].Handle < handles[j].Handle })

	barrierID := fmt.Sprintf("bar-m%d", es.LastSeq()+1)
	env, err := event.New(event.TypeCheckpointBarrier, &event.CheckpointBarrier{
		BarrierID: barrierID, GenStep: fold.Session.GenStep,
		Vector: vector, SnapshotRef: ref, Handles: handles,
	})
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	env.CorrelationID = session
	env.Sender, env.Target = "cli", "session"
	if _, err := es.Append(env); err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	fmt.Fprintf(stdout, "barrier %s\nsnapshot %s\n", barrierID, short(ref))
	fmt.Fprintf(stderr, "fork with: agentrunner fork %s %s\n", session, barrierID)
	return ExitOK
}
