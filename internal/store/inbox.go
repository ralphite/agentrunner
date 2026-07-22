// Durable session mailbox (v2 收口, closes the "崩溃不丢输入" gap): the
// daemon persists every conversational delivery HERE, fsynced, BEFORE
// acking the sender — the ack means durable, not merely enqueued. Each
// entry carries a per-session monotonic delivery seq; the loop journals
// the seq with the InputReceived it consumes, so a resume replays exactly
// the entries the journal has not seen (at-least-once + seq dedup =
// effectively once). This file is the daemon's mailbox, NOT the session
// journal: a separate writer, so the journal's single-writer discipline
// is untouched.
package store

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
)

const inboxFile = "inbox.jsonl"

// inboxMu serializes appends per session dir within this process (the
// daemon is the only writer of mailbox files).
var (
	inboxMu    sync.Mutex
	inboxCache = map[string]*inboxIndex{}
)

type inboxReceipt struct {
	seq  int64
	hash [32]byte
}

type draftClaim struct {
	commandID string
	consumed  bool
}

// inboxIndex is loaded once per session per process. The old implementation
// rescanned the whole JSONL on every append, making N deliveries O(N²).
// A daemon is the sole writer, so an in-process append index is sufficient;
// after restart one linear scan rebuilds it without a side database.
type inboxIndex struct {
	last     int64
	receipts map[string]inboxReceipt
	drafts   map[string]draftClaim
}

// AppendInbox is the compatibility input view over the unified command log.
func AppendInbox(sessionDir string, in protocol.UserInput) (protocol.UserInput, error) {
	cmd, err := AppendCommand(sessionDir, protocol.SessionCommand{
		CommandRef: protocol.CommandRef{CommandID: in.CommandID},
		Kind:       protocol.CommandInput, Input: &in,
		Principal: in.Principal, Source: in.Source, Trust: in.Trust,
	})
	if err != nil {
		return in, err
	}
	result := *cmd.Input
	result.CommandID = cmd.CommandID
	result.DeliverySeq = cmd.CommandSeq
	return result, nil
}

// AppendCommand persists one typed session command, assigning the next
// sequence and fsyncing before return. Exact command-id retries return the
// original receipt; conflicting payload reuse is rejected.
func AppendCommand(sessionDir string, cmd protocol.SessionCommand) (protocol.SessionCommand, error) {
	inboxMu.Lock()
	defer inboxMu.Unlock()
	idx, err := loadInboxIndex(sessionDir)
	if err != nil {
		return cmd, err
	}
	if cmd.CommandID == "" {
		cmd.CommandID = event.NewCommandID() // compatibility with older callers
	}
	if err := validateCommand(cmd); err != nil {
		return cmd, err
	}
	hash, err := commandPayloadHash(cmd)
	if err != nil {
		return cmd, err
	}
	if prior, ok := idx.receipts[cmd.CommandID]; ok {
		if prior.hash != hash {
			return cmd, fmt.Errorf("inbox: command_id %q reused with different payload", cmd.CommandID)
		}
		cmd.CommandSeq = prior.seq
		cmd.PreviouslyAccepted = true
		stampInputReceipt(&cmd)
		return cmd, nil
	}
	if cmd.Input != nil && cmd.Input.ForkDraftID != "" {
		if err := settleDraftClaims(sessionDir, idx); err != nil {
			return cmd, err
		}
		if prior, ok := idx.drafts[cmd.Input.ForkDraftID]; ok && prior.commandID != cmd.CommandID {
			state := "has an unsettled attempt"
			if prior.consumed {
				state = "was already consumed"
			}
			return cmd, fmt.Errorf("inbox: fork draft %q %s", cmd.Input.ForkDraftID, state)
		}
	}
	cmd.CommandSeq = idx.last + 1
	stampInputReceipt(&cmd)
	line, err := json.Marshal(cmd)
	if err != nil {
		return cmd, err
	}
	f, err := os.OpenFile(filepath.Join(sessionDir, inboxFile),
		os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return cmd, err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return cmd, err
	}
	if err := f.Sync(); err != nil { // the ack promises durability
		_ = f.Close()
		return cmd, err
	}
	if err := f.Close(); err != nil {
		return cmd, err
	}
	idx.last = cmd.CommandSeq
	idx.receipts[cmd.CommandID] = inboxReceipt{seq: cmd.CommandSeq, hash: hash}
	if cmd.Input != nil && cmd.Input.ForkDraftID != "" {
		idx.drafts[cmd.Input.ForkDraftID] = draftClaim{commandID: cmd.CommandID}
	}
	return cmd, nil
}

// SeedInboxWatermark writes one inert mailbox entry at seq so a fresh mailbox
// numbers its next real delivery ABOVE seq. A fork's journal inherits the
// parent's consumed-input high-water mark (the copied input_received events
// carry their delivery seqs), but its inbox FILE starts empty — without this
// seed the next send reuses delivery_seq 1, which the dedup drops as
// already-consumed, and the fork silently swallows every message (C4). The
// seed is never replayed: ReadInbox returns only seq > ConsumedInputSeq, and
// the seed's seq equals it. It exists solely to advance lastInboxSeq.
func SeedInboxWatermark(sessionDir string, seq int64) error {
	if seq <= 0 {
		return nil
	}
	inboxMu.Lock()
	defer inboxMu.Unlock()
	line, err := json.Marshal(protocol.UserInput{DeliverySeq: seq})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(sessionDir, inboxFile),
		os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if idx := inboxCache[sessionDir]; idx != nil && seq > idx.last {
		idx.last = seq
	}
	return nil
}

// ReadCommands returns every command with seq > after. It accepts legacy
// input-only lines and upgrades them in memory without rewriting user data.
func ReadCommands(sessionDir string, after int64) ([]protocol.SessionCommand, error) {
	f, err := os.Open(filepath.Join(sessionDir, inboxFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []protocol.SessionCommand
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)
	for sc.Scan() {
		var probe struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(sc.Bytes(), &probe); err != nil {
			return nil, fmt.Errorf("inbox: bad line: %w", err)
		}
		var cmd protocol.SessionCommand
		if probe.Kind != "" {
			if err := json.Unmarshal(sc.Bytes(), &cmd); err != nil {
				return nil, fmt.Errorf("inbox: bad command: %w", err)
			}
		} else {
			var in protocol.UserInput
			if err := json.Unmarshal(sc.Bytes(), &in); err != nil {
				return nil, fmt.Errorf("inbox: bad legacy input: %w", err)
			}
			cmd = protocol.SessionCommand{
				CommandRef: protocol.CommandRef{CommandID: in.CommandID, CommandSeq: in.DeliverySeq},
				Kind:       protocol.CommandInput, Input: &in,
			}
		}
		stampInputReceipt(&cmd)
		if cmd.CommandSeq > after {
			out = append(out, cmd)
		}
	}
	return out, sc.Err()
}

// ReadInbox is the legacy input projection used by Loop.Resume.
func ReadInbox(sessionDir string, after int64) ([]protocol.UserInput, error) {
	commands, err := ReadCommands(sessionDir, after)
	if err != nil {
		return nil, err
	}
	var out []protocol.UserInput
	for _, cmd := range commands {
		if cmd.Kind == protocol.CommandInput && cmd.Input != nil {
			out = append(out, *cmd.Input)
		}
	}
	return out, nil
}

func loadInboxIndex(sessionDir string) (*inboxIndex, error) {
	if idx := inboxCache[sessionDir]; idx != nil {
		return idx, nil
	}
	entries, err := ReadCommands(sessionDir, 0)
	if err != nil {
		return nil, err
	}
	idx := &inboxIndex{receipts: map[string]inboxReceipt{}, drafts: map[string]draftClaim{}}
	for _, cmd := range entries {
		if cmd.CommandSeq > idx.last {
			idx.last = cmd.CommandSeq
		}
		if cmd.CommandID == "" { // legacy mailbox entry: seq-only dedup remains
			continue
		}
		hash, herr := commandPayloadHash(cmd)
		if herr != nil {
			return nil, herr
		}
		idx.receipts[cmd.CommandID] = inboxReceipt{seq: cmd.CommandSeq, hash: hash}
		if cmd.Input != nil && cmd.Input.ForkDraftID != "" {
			idx.drafts[cmd.Input.ForkDraftID] = draftClaim{commandID: cmd.CommandID}
		}
	}
	if len(idx.drafts) > 0 {
		if err := settleDraftClaims(sessionDir, idx); err != nil {
			return nil, err
		}
	}
	inboxCache[sessionDir] = idx
	return idx, nil
}

// settleDraftClaims projects the journal receipts that settle a durable
// first-send claim. A successful InputReceived consumes the draft forever;
// InputRevoked or a hook-veto CommandHandled releases it for a new attempt.
// Reading complete event lines is safe beside the journal's single writer.
func settleDraftClaims(sessionDir string, idx *inboxIndex) error {
	events, err := ReadEvents(sessionDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, env := range events {
		switch env.Type {
		case event.TypeInputReceived:
			decoded, err := event.DecodePayload(env)
			if err != nil {
				return err
			}
			p := decoded.(*event.InputReceived)
			if claim, ok := idx.drafts[p.ForkDraftID]; p.ForkDraftID != "" && ok &&
				(env.CommandID == "" || claim.commandID == env.CommandID) {
				claim.consumed = true
				idx.drafts[p.ForkDraftID] = claim
			}
		case event.TypeInputRevoked:
			decoded, err := event.DecodePayload(env)
			if err != nil {
				return err
			}
			target := decoded.(*event.InputRevoked).TargetCommandID
			for draftID, claim := range idx.drafts {
				if claim.commandID == target && !claim.consumed {
					delete(idx.drafts, draftID)
				}
			}
		case event.TypeCommandHandled:
			decoded, err := event.DecodePayload(env)
			if err != nil {
				return err
			}
			p := decoded.(*event.CommandHandled)
			if p.Result != "input_rejected" {
				continue
			}
			for draftID, claim := range idx.drafts {
				if claim.commandID == env.CommandID && !claim.consumed {
					delete(idx.drafts, draftID)
				}
			}
		}
	}
	return nil
}

func commandPayloadHash(cmd protocol.SessionCommand) ([32]byte, error) {
	cmd.CommandSeq = 0
	cmd.PreviouslyAccepted = false
	if cmd.Input != nil {
		copy := *cmd.Input
		copy.DeliverySeq = 0
		copy.CommandID = ""
		cmd.Input = &copy
	}
	if cmd.Control != nil {
		copy := *cmd.Control
		copy.CommandRef = protocol.CommandRef{}
		cmd.Control = &copy
	}
	if cmd.Revoke != nil {
		copy := *cmd.Revoke
		copy.CommandRef = protocol.CommandRef{}
		cmd.Revoke = &copy
	}
	if cmd.Answer != nil {
		copy := *cmd.Answer
		copy.CommandRef = protocol.CommandRef{}
		cmd.Answer = &copy
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

func stampInputReceipt(cmd *protocol.SessionCommand) {
	if cmd.Control != nil {
		cmd.Control.CommandRef = cmd.CommandRef
	}
	if cmd.Input == nil {
		return
	}
	cmd.Input.CommandID = cmd.CommandID
	cmd.Input.DeliverySeq = cmd.CommandSeq
}

func validateCommand(cmd protocol.SessionCommand) error {
	switch cmd.Kind {
	case protocol.CommandInput:
		if cmd.Input == nil {
			return fmt.Errorf("inbox: input command missing payload")
		}
	case protocol.CommandControl:
		if cmd.Control == nil {
			return fmt.Errorf("inbox: control command missing payload")
		}
	case protocol.CommandApproval:
		if cmd.Approval == nil {
			return fmt.Errorf("inbox: approval command missing payload")
		}
	case protocol.CommandInterrupt:
	case protocol.CommandClose:
		if cmd.Control == nil {
			return fmt.Errorf("inbox: close command missing payload")
		}
	case protocol.CommandKill:
		if cmd.Handle == "" {
			return fmt.Errorf("inbox: kill command missing handle")
		}
	case protocol.CommandRevoke:
		if cmd.Revoke == nil || cmd.Revoke.TargetCommandID == "" {
			return fmt.Errorf("inbox: revoke command missing target_command_id")
		}
	case protocol.CommandAnswer:
		if cmd.Answer == nil || (len(cmd.Answer.Answers) == 0 && !cmd.Answer.Cancelled) {
			return fmt.Errorf("inbox: answer command needs answers or cancelled")
		}
	default:
		return fmt.Errorf("inbox: unknown command kind %q", cmd.Kind)
	}
	return nil
}
