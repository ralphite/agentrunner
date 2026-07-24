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
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/snapshot"
	"github.com/ralphite/agentrunner/internal/store"
)

type lastTurnBaseline struct {
	InputSeq       int64
	TurnID         string
	BarrierSeq     int64
	BarrierID      string
	SnapshotRef    string
	Completed      bool
	EndBarrierSeq  int64
	EndBarrierID   string
	EndSnapshotRef string
}

type lastTurnDiffResponse struct {
	Scope            string            `json:"scope"`
	Available        bool              `json:"available"`
	Reason           string            `json:"reason,omitempty"`
	Workspace        string            `json:"workspace,omitempty"`
	InputSeq         int64             `json:"input_seq,omitempty"`
	TurnID           string            `json:"turn_id,omitempty"`
	BarrierSeq       int64             `json:"barrier_seq,omitempty"`
	BarrierID        string            `json:"barrier_id,omitempty"`
	Completed        bool              `json:"completed,omitempty"`
	EndBarrierSeq    int64             `json:"end_barrier_seq,omitempty"`
	EndBarrierID     string            `json:"end_barrier_id,omitempty"`
	Diff             string            `json:"diff"`
	Numstat          string            `json:"numstat"`
	Untracked        []string          `json:"untracked"`
	UntrackedReasons map[string]string `json:"untrackedReasons"`
	HiddenUntracked  int               `json:"hiddenUntracked"`
}

// planLastTurnDiffBaseline is the journal-pure half of Last turn review.
// A completed turn is bounded by message-anchored snapshots carrying the same
// turn_id. An active turn has only its start snapshot and may compare to the
// live workspace. Machine/program/agent traffic never redefines the window.
func planLastTurnDiffBaseline(events []event.Envelope) (*lastTurnBaseline, string, error) {
	var inputSeq int64
	var input *event.InputReceived
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type != event.TypeInputReceived {
			continue
		}
		decoded, err := event.DecodePayload(events[i])
		if err != nil {
			return nil, "", fmt.Errorf("decode input at seq %d: %w", events[i].Seq, err)
		}
		candidate := decoded.(*event.InputReceived)
		if protocol.UserClassSource(candidate.Source) {
			inputSeq = events[i].Seq
			input = candidate
			break
		}
	}
	if inputSeq == 0 || input == nil {
		return nil, "no human turn in this session", nil
	}
	baseline := &lastTurnBaseline{InputSeq: inputSeq, TurnID: input.TurnID}
	var legacyStart *lastTurnBaseline
	for _, env := range events {
		if env.Type == event.TypeAssistantMessage && env.Seq > inputSeq {
			decoded, err := event.DecodePayload(env)
			if err != nil {
				return nil, "", fmt.Errorf("decode assistant at seq %d: %w", env.Seq, err)
			}
			msg := decoded.(*event.AssistantMessage)
			if msg.TurnID == input.TurnID && assistantCompletesTurn(msg) {
				baseline.Completed = true
			}
			continue
		}
		if env.Type != event.TypeCheckpointBarrier {
			continue
		}
		decoded, err := event.DecodePayload(env)
		if err != nil {
			return nil, "", fmt.Errorf("decode barrier at seq %d: %w", env.Seq, err)
		}
		barrier := decoded.(*event.CheckpointBarrier)
		if anchor := barrier.MessageAnchor; anchor != nil && anchor.TurnID == input.TurnID &&
			barrier.SnapshotRef != "" {
			switch anchor.Side {
			case "before_user":
				if env.Seq < inputSeq && env.Seq > baseline.BarrierSeq {
					baseline.BarrierSeq, baseline.BarrierID, baseline.SnapshotRef =
						env.Seq, barrier.BarrierID, barrier.SnapshotRef
				}
			case "after_assistant":
				if env.Seq > inputSeq && env.Seq > baseline.EndBarrierSeq {
					baseline.Completed = true
					baseline.EndBarrierSeq, baseline.EndBarrierID, baseline.EndSnapshotRef =
						env.Seq, barrier.BarrierID, barrier.SnapshotRef
				}
			}
			continue
		}
		// Only loop-owned generation-start barriers are lawful Last turn
		// baselines for legacy journals without message anchors. Explicit
		// `ar barrier` cuts (bar-m*) and bar-final happen after arbitrary work
		// and would falsely shrink the review window.
		if env.Seq <= inputSeq {
			continue
		}
		turn, turnErr := strconv.Atoi(strings.TrimPrefix(barrier.BarrierID, "bar-t"))
		if !strings.HasPrefix(barrier.BarrierID, "bar-t") || turnErr != nil || turn < 1 || barrier.SnapshotRef == "" {
			continue
		}
		if legacyStart == nil {
			legacyStart = &lastTurnBaseline{InputSeq: inputSeq, TurnID: input.TurnID,
				BarrierSeq: env.Seq, BarrierID: barrier.BarrierID, SnapshotRef: barrier.SnapshotRef}
		}
	}
	if baseline.SnapshotRef == "" && legacyStart != nil {
		baseline.BarrierSeq, baseline.BarrierID, baseline.SnapshotRef =
			legacyStart.BarrierSeq, legacyStart.BarrierID, legacyStart.SnapshotRef
	}
	if baseline.SnapshotRef == "" {
		return nil, "latest human turn has no durable workspace baseline yet", nil
	}
	if baseline.Completed && baseline.EndSnapshotRef == "" {
		return nil, "completed human turn has no durable end snapshot", nil
	}
	return baseline, "", nil
}

func assistantCompletesTurn(msg *event.AssistantMessage) bool {
	for _, part := range msg.Message.Parts {
		if part.Kind == provider.PartToolCall {
			return false
		}
	}
	// A clean end_turn may legitimately contain no text. It is still terminal;
	// treating it as active would compare its old baseline to the newer live
	// workspace and reintroduce cross-turn attribution.
	return true
}

// diffCmd exposes the runtime's durable Last turn comparison without leaking
// the snapshot backend to Web UI or other clients.
func diffCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	scope := fs.String("scope", "last-turn", "diff scope (last-turn)")
	jsonOutput := fs.Bool("json", false, "print structured JSON")
	if ok, code := parseFlags(fs, args); !ok {
		return code
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
	resp := lastTurnDiffResponse{
		Scope: *scope, Workspace: started.WorkspaceRoot,
		Untracked: []string{}, UntrackedReasons: map[string]string{},
	}
	baseline, reason, err := planLastTurnDiffBaseline(events)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	if baseline == nil {
		resp.Reason = reason
		return writeLastTurnDiff(resp, *jsonOutput, stdout, stderr)
	}
	resp.InputSeq, resp.TurnID = baseline.InputSeq, baseline.TurnID
	resp.BarrierSeq, resp.BarrierID = baseline.BarrierSeq, baseline.BarrierID
	resp.Completed = baseline.Completed
	resp.EndBarrierSeq, resp.EndBarrierID = baseline.EndBarrierSeq, baseline.EndBarrierID
	if started.WorkspaceRoot == "" {
		resp.Reason = "session has no recorded workspace"
		return writeLastTurnDiff(resp, *jsonOutput, stdout, stderr)
	}
	shadow, err := openShadow(started.WorkspaceRoot)
	if err != nil {
		resp.Reason = "workspace snapshot backend is unavailable"
		return writeLastTurnDiff(resp, *jsonOutput, stdout, stderr)
	}
	var result snapshot.DiffResult
	if baseline.Completed {
		result, err = shadow.DiffSnapshots(context.Background(), baseline.SnapshotRef, baseline.EndSnapshotRef)
	} else {
		result, err = shadow.Diff(context.Background(), baseline.SnapshotRef)
	}
	if err != nil {
		resp.Reason = "durable workspace baseline is unavailable"
		return writeLastTurnDiff(resp, *jsonOutput, stdout, stderr)
	}
	resp.Available, resp.Diff, resp.Numstat = true, result.Diff, result.Numstat
	resp.Untracked, resp.UntrackedReasons, resp.HiddenUntracked =
		result.Untracked, result.UntrackedReasons, result.HiddenUntracked
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
