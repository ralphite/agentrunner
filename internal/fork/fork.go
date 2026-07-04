// Package fork copies a barrier cut into a fresh session (S7 模块 3, DESIGN
// §fork/rewind): the ONLY legal fork target is a CheckpointBarrier, and the
// two axes stay orthogonal — this package handles the EVENT cut (journal,
// child streams, artifact CAS); worktree materialization from the snapshot
// ref is the caller's axis (snapshot.Store.Materialize).
package fork

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// Options names everything a cut copy needs. The caller resolves the
// barrier from the parent fold and materializes the worktree; Cut owns the
// journal-and-stores copy.
type Options struct {
	ParentDir     string // parent session dir
	ParentSession string
	NewDir        string // fork session dir; its journal must not exist yet
	NewSession    string
	Barrier       state.Barrier // fork target (from the parent fold's barriers)
	WorkspaceRoot string        // the fork's own worktree (never the parent's)
	Now           time.Time     // fork moment (genesis TS)
}

// Cut writes the fork's journal — a ForkedFrom genesis followed by the
// parent's events up to and including the barrier, seqs shifted by one and
// ids remapped — and copies the child journals (sub/) and artifact CAS
// verbatim. Fold snapshots are NOT copied: they cache parent seqs, and the
// fork's first resume folds from scratch. Returns the distinct snapshot
// refs inside the cut so the caller can pin them in the fork's own shadow
// store.
func Cut(opts Options) ([]string, error) {
	events, err := store.ReadEvents(opts.ParentDir)
	if err != nil {
		return nil, fmt.Errorf("fork: parent journal: %w", err)
	}
	cut := -1
	for i, e := range events {
		if e.Seq == opts.Barrier.Seq && e.Type == event.TypeCheckpointBarrier {
			cut = i
			break
		}
	}
	if cut < 0 {
		return nil, fmt.Errorf("fork: barrier %s (seq %d) not found in parent journal",
			opts.Barrier.BarrierID, opts.Barrier.Seq)
	}
	// A parent that is itself a fork carries its OWN genesis at seq 1; the
	// new fork gets exactly ONE genesis (its own — provenance names the
	// immediate parent, the full lineage is walkable through the parents'
	// journals). Copying it would bury run_started two deep and break every
	// consumer that skips a single genesis (S7 出口 review P0).
	src := events[:cut+1]
	shift := int64(1)
	if len(src) > 0 && src[0].Type == event.TypeForkedFrom {
		src = src[1:]
		shift = 0
	}

	genesis, err := event.New(event.TypeForkedFrom, &event.ForkedFrom{
		ParentSession: opts.ParentSession,
		BarrierID:     opts.Barrier.BarrierID,
		SnapshotRef:   opts.Barrier.SnapshotRef,
		WorkspaceRoot: opts.WorkspaceRoot,
	})
	if err != nil {
		return nil, err
	}
	genesis.Seq, genesis.ID = 1, event.EventID(1)
	genesis.CorrelationID = opts.NewSession
	genesis.Sender, genesis.Target = "cli", "session"
	genesis.TS = opts.Now.UTC()

	lines := make([]event.Envelope, 0, len(src)+2)
	lines = append(lines, genesis)
	var refs []string
	seen := map[string]bool{}
	for _, e := range src {
		lines = append(lines, remap(e, opts.ParentSession, opts.NewSession, shift))
		if e.Type == event.TypeCheckpointBarrier {
			if dec, derr := event.DecodePayload(e); derr == nil {
				if ref := dec.(*event.CheckpointBarrier).SnapshotRef; ref != "" && !seen[ref] {
					seen[ref] = true
					refs = append(refs, ref)
				}
			}
		}
	}
	// The barrier's task disposition vector is APPLIED here, not just
	// copied (S7 出口 review P1): a cancel_at_fork task gets a synthetic
	// terminal right after the cut, so the fork's fold has no in-flight
	// non-idempotent activity (which would refuse every resume as
	// in-doubt) and the model sees the cancellation outcome instead.
	cancels, err := cancelAtFork(lines, opts)
	if err != nil {
		return nil, err
	}
	lines = append(lines, cancels...)
	if err := writeJournal(opts.NewDir, lines); err != nil {
		return nil, err
	}

	// Child journals and the artifact CAS travel with the cut: copied
	// conversation references child results and artifact refs, and the
	// barrier vector's sub/ streams must stay readable from the fork.
	for _, aux := range []string{"sub", "artifacts"} {
		src := filepath.Join(opts.ParentDir, aux)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.CopyFS(filepath.Join(opts.NewDir, aux), os.DirFS(src)); err != nil {
			return nil, fmt.Errorf("fork: copy %s: %w", aux, err)
		}
	}
	return refs, nil
}

// remap shifts one parent envelope into the fork's id space: seq/id move by
// one (the genesis claimed seq 1), causation ids that reference events move
// with them, and the correlation id becomes the fork's session. Payloads
// are provenance and stay byte-identical.
func remap(e event.Envelope, parentSession, newSession string, shift int64) event.Envelope {
	e.Seq += shift
	e.ID = event.EventID(e.Seq)
	if n, ok := eventSeq(e.CausationID); ok {
		e.CausationID = event.EventID(n + shift)
	}
	if e.CorrelationID == parentSession {
		e.CorrelationID = newSession
	}
	return e
}

// cancelAtFork folds the copied cut and appends one ActivityCancelled per
// barrier task whose policy is cancel_at_fork — the task's process never
// existed in the fork, and the fold renders the cancellation as the
// model-visible outcome ("fork 后模型可自行重启", DESIGN §fork/rewind).
func cancelAtFork(lines []event.Envelope, opts Options) ([]event.Envelope, error) {
	if len(opts.Barrier.Tasks) == 0 {
		return nil, nil
	}
	folded, err := state.Fold(lines)
	if err != nil {
		return nil, fmt.Errorf("fork: fold cut for task disposition: %w", err)
	}
	seq := lines[len(lines)-1].Seq
	var out []event.Envelope
	for _, task := range opts.Barrier.Tasks {
		if task.Policy != "cancel_at_fork" {
			continue // unknown policies stay untouched; the fold will refuse resume loudly
		}
		started, ok := folded.Tasks[task.TaskID]
		if !ok {
			continue // already settled inside the cut
		}
		env, err := event.New(event.TypeActivityCancelled, &event.ActivityCancelled{
			ActivityID:    started.ActivityID,
			PartialOutput: "cancelled at fork (policy cancel_at_fork): the task's process belongs to the original run",
		})
		if err != nil {
			return nil, err
		}
		seq++
		env.Seq, env.ID = seq, event.EventID(seq)
		env.CausationID = event.EventID(1) // the fork genesis caused the disposition
		env.CorrelationID = opts.NewSession
		env.Sender, env.Target = "cli", "session"
		env.TS = opts.Now.UTC()
		out = append(out, env)
	}
	return out, nil
}

func eventSeq(id string) (int64, bool) {
	rest, ok := strings.CutPrefix(id, "evt-")
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// writeJournal creates the fork's events.jsonl in one durable step (write
// temp → fsync → rename); a half-written fork must never look resumable.
func writeJournal(dir string, lines []event.Envelope) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("fork: %w", err)
	}
	final := filepath.Join(dir, "events.jsonl")
	if _, err := os.Stat(final); err == nil {
		return fmt.Errorf("fork: %s already has a journal", dir)
	}
	var buf []byte
	for _, e := range lines {
		raw, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("fork: marshal seq %d: %w", e.Seq, err)
		}
		buf = append(buf, raw...)
		buf = append(buf, '\n')
	}
	tmp := final + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("fork: %w", err)
	}
	if _, err := f.Write(buf); err != nil {
		_ = f.Close()
		return fmt.Errorf("fork: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("fork: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("fork: %w", err)
	}
	return os.Rename(tmp, final)
}
