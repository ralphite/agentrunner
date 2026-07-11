package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/store"
)

type lastTurnBaseline struct {
	InputSeq    int64
	BarrierSeq  int64
	BarrierID   string
	SnapshotRef string
}

type lastTurnDiffResponse struct {
	Scope      string `json:"scope"`
	Available  bool   `json:"available"`
	Reason     string `json:"reason,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
	InputSeq   int64  `json:"input_seq,omitempty"`
	BarrierSeq int64  `json:"barrier_seq,omitempty"`
	BarrierID  string `json:"barrier_id,omitempty"`
	Diff       string `json:"diff"`
	Numstat    string `json:"numstat"`
}

// planLastTurnDiffBaseline is the journal-pure half of Last turn review.
// Latest human input wins; the first following durable workspace barrier is
// the baseline. Machine/program/agent traffic does not silently redefine the
// user's review window.
func planLastTurnDiffBaseline(events []event.Envelope) (*lastTurnBaseline, string, error) {
	var inputSeq int64
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type != event.TypeInputReceived {
			continue
		}
		decoded, err := event.DecodePayload(events[i])
		if err != nil {
			return nil, "", fmt.Errorf("decode input at seq %d: %w", events[i].Seq, err)
		}
		if protocol.UserClassSource(decoded.(*event.InputReceived).Source) {
			inputSeq = events[i].Seq
			break
		}
	}
	if inputSeq == 0 {
		return nil, "no human turn in this session", nil
	}
	for _, env := range events {
		if env.Seq <= inputSeq || env.Type != event.TypeCheckpointBarrier {
			continue
		}
		decoded, err := event.DecodePayload(env)
		if err != nil {
			return nil, "", fmt.Errorf("decode barrier at seq %d: %w", env.Seq, err)
		}
		barrier := decoded.(*event.CheckpointBarrier)
		// Only loop-owned generation-start barriers are lawful Last turn
		// baselines. Explicit `ar barrier` cuts (bar-m*) and bar-final happen
		// after arbitrary work and would falsely shrink the review window.
		turn, turnErr := strconv.Atoi(strings.TrimPrefix(barrier.BarrierID, "bar-t"))
		if !strings.HasPrefix(barrier.BarrierID, "bar-t") || turnErr != nil || turn < 1 || barrier.SnapshotRef == "" {
			continue
		}
		return &lastTurnBaseline{InputSeq: inputSeq, BarrierSeq: env.Seq,
			BarrierID: barrier.BarrierID, SnapshotRef: barrier.SnapshotRef}, "", nil
	}
	return nil, "latest human turn has no durable workspace baseline yet", nil
}

// diffCmd exposes the runtime's durable Last turn comparison without leaking
// the snapshot backend to Web UI or other clients.
func diffCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	scope := fs.String("scope", "last-turn", "diff scope (last-turn)")
	jsonOutput := fs.Bool("json", false, "print structured JSON")
	if err := fs.Parse(reorderFlags(fs, args)); err != nil {
		return ExitUsage
	}
	rest := fs.Args()
	if len(rest) != 1 || *scope != "last-turn" {
		fmt.Fprintln(stderr, "usage: agentrunner diff <session-id-or-prefix> [--scope last-turn] [--json]")
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
	started, err := sessionStartedFromEvents(events)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	resp := lastTurnDiffResponse{Scope: *scope, Workspace: started.WorkspaceRoot}
	baseline, reason, err := planLastTurnDiffBaseline(events)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	if baseline == nil {
		resp.Reason = reason
		return writeLastTurnDiff(resp, *jsonOutput, stdout, stderr)
	}
	resp.InputSeq, resp.BarrierSeq, resp.BarrierID = baseline.InputSeq, baseline.BarrierSeq, baseline.BarrierID
	if started.WorkspaceRoot == "" {
		resp.Reason = "session has no recorded workspace"
		return writeLastTurnDiff(resp, *jsonOutput, stdout, stderr)
	}
	shadow, err := openShadow(started.WorkspaceRoot)
	if err != nil {
		resp.Reason = "workspace snapshot backend is unavailable"
		return writeLastTurnDiff(resp, *jsonOutput, stdout, stderr)
	}
	result, err := shadow.Diff(context.Background(), baseline.SnapshotRef)
	if err != nil {
		resp.Reason = "durable workspace baseline is unavailable"
		return writeLastTurnDiff(resp, *jsonOutput, stdout, stderr)
	}
	resp.Available, resp.Diff, resp.Numstat = true, result.Diff, result.Numstat
	return writeLastTurnDiff(resp, *jsonOutput, stdout, stderr)
}

func writeLastTurnDiff(resp lastTurnDiffResponse, asJSON bool, stdout, stderr io.Writer) int {
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintf(stderr, "agentrunner: encode diff: %v\n", err)
			return ExitRun
		}
		return ExitOK
	}
	if !resp.Available {
		fmt.Fprintf(stdout, "Last turn unavailable: %s\n", resp.Reason)
		return ExitOK
	}
	if resp.Diff == "" {
		fmt.Fprintln(stdout, "No changes since the latest human turn began.")
		return ExitOK
	}
	fmt.Fprintln(stdout, resp.Diff)
	return ExitOK
}
