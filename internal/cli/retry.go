package cli

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// retryCmd re-sends a session's last user input as a new turn (INC-44 §B,
// HANDA #16). The payload is reassembled as a PURE FUNCTION of the journal
// — text verbatim, attachment bytes read back from the session CAS,
// provenance constant — and the command id derives from the original
// ("retry:<orig-id>"), so a double-click hits AppendCommand's same-id
// same-payload idempotency instead of double-sending. A failed retry can be
// retried again: that attempt's target is the retry command itself, so the
// derived id differs and the chain continues.
func retryCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("retry", flag.ContinueOnError)
	fs.SetOutput(stderr)
	detach := fs.Bool("detach", false, "deliver the retry and exit without waiting for the reply")
	if err := fs.Parse(reorderFlags(fs, args)); err != nil {
		return ExitUsage
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, `usage: agentrunner retry [--detach] <session-id-or-prefix>`)
		return ExitUsage
	}
	dir, err := resolveSessionDir(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	events, err := store.ReadEvents(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	s, err := state.Fold(events)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: fold: %v\n", err)
		return ExitRun
	}
	target, newID, perr := planRetry(events, s, store.HasLiveWriter(dir))
	if perr != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", perr)
		return ExitUsage
	}
	images, files, err := attachmentsFromCAS(dir, target)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	cmd := daemon.Command{Cmd: "send", Session: resolvePrefixLenient(rest[0]),
		Text: target.Text, Images: images, Files: files, CommandID: newID,
		Principal: "local-user", Source: "cli", Trust: "local"}
	if *detach {
		return oneShot(stderr, cmd, stdout)
	}
	sock, serr := socketPath()
	if serr != nil {
		fmt.Fprintln(stderr, serr)
		return ExitRun
	}
	cmd.Follow = true
	return followTurn(sock, cmd, "retry delivered", stdout, stderr)
}

// planRetry applies the busy guards and locates the retry target: the last
// USER-class input (program/agent/parent inputs are not retry targets) and
// the derived command id. Pure over the fold + journal — the testable half
// of retryCmd.
func planRetry(events []event.Envelope, s state.State, liveWriter bool) (*event.InputReceived, string, error) {
	// Busy guards. A QUIESCENT session is always a lawful target — note that
	// idle standby folds as Waiting{input} too, so the wait check must come
	// AFTER the quiescence one (真验 first caught the standby false-positive).
	// Non-quiescent waits (an ask park's unresolved call, an approval) want
	// their ANSWER — a send there pairs as the reply, so retry must refuse;
	// and a live mid-turn session would double-run the input once it settles.
	if q, _ := state.Quiescence(s); !q {
		if s.Waiting != nil {
			return nil, "", fmt.Errorf("session is waiting (%s) — answer it instead of retrying", s.Waiting.Kind)
		}
		if s.Session.Closed == nil && liveWriter {
			return nil, "", fmt.Errorf("session is mid-turn; wait for it to settle (or interrupt) before retrying")
		}
	}
	var target *event.InputReceived
	var origID string
	var origSeq int64
	for i := len(events) - 1; i >= 0 && target == nil; i-- {
		if events[i].Type != event.TypeInputReceived {
			continue
		}
		dec, derr := event.DecodePayload(events[i])
		if derr != nil {
			continue
		}
		p := dec.(*event.InputReceived)
		// Human senders only (决策 #30 family).
		if protocol.UserClassSource(p.Source) {
			target, origID, origSeq = p, events[i].CommandID, events[i].Seq
		}
	}
	if target == nil {
		return nil, "", fmt.Errorf("no user input in this session to retry")
	}
	newID := "retry:" + origID
	if origID == "" { // legacy journal without command ids: seq is as stable
		newID = fmt.Sprintf("retry:seq%d", origSeq)
	}
	return target, newID, nil
}

// attachmentsFromCAS reads the journaled refs' bytes back from the session
// CAS so the retry rides the same wire shape as the original send. Reads
// are deterministic — the same refs yield the same bytes — which is what
// keeps the derived command id's payload hash stable.
func attachmentsFromCAS(dir string, in *event.InputReceived) ([]protocol.ImageAttachment, []protocol.FileAttachment, error) {
	if len(in.Images) == 0 && len(in.Files) == 0 {
		return nil, nil, nil
	}
	as, err := store.OpenArtifactStore(filepath.Join(dir, "artifacts"))
	if err != nil {
		return nil, nil, err
	}
	var images []protocol.ImageAttachment
	for _, ref := range in.Images {
		data, gerr := as.Get(ref.Ref)
		if gerr != nil {
			return nil, nil, fmt.Errorf("attachment %s: %w", ref.Ref, gerr)
		}
		images = append(images, protocol.ImageAttachment{MediaType: ref.MediaType, Data: data})
	}
	var files []protocol.FileAttachment
	for _, ref := range in.Files {
		data, gerr := as.Get(ref.Ref)
		if gerr != nil {
			return nil, nil, fmt.Errorf("attachment %s: %w", ref.Ref, gerr)
		}
		files = append(files, protocol.FileAttachment{MediaType: ref.MediaType, Data: data})
	}
	return images, files, nil
}
