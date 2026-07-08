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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ralphite/agentrunner/internal/protocol"
)

const inboxFile = "inbox.jsonl"

// inboxMu serializes appends per session dir within this process (the
// daemon is the only writer of mailbox files).
var inboxMu sync.Mutex

// AppendInbox persists one delivery, assigning the next seq, and fsyncs
// before returning. The returned input carries its DeliverySeq.
func AppendInbox(sessionDir string, in protocol.UserInput) (protocol.UserInput, error) {
	inboxMu.Lock()
	defer inboxMu.Unlock()
	last, err := lastInboxSeq(sessionDir)
	if err != nil {
		return in, err
	}
	in.DeliverySeq = last + 1
	line, err := json.Marshal(in)
	if err != nil {
		return in, err
	}
	f, err := os.OpenFile(filepath.Join(sessionDir, inboxFile),
		os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return in, err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return in, err
	}
	if err := f.Sync(); err != nil { // the ack promises durability
		_ = f.Close()
		return in, err
	}
	return in, f.Close()
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
	return f.Close()
}

// ReadInbox returns every persisted delivery with seq > after, in order.
// A missing mailbox is an empty one.
func ReadInbox(sessionDir string, after int64) ([]protocol.UserInput, error) {
	f, err := os.Open(filepath.Join(sessionDir, inboxFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []protocol.UserInput
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)
	for sc.Scan() {
		var in protocol.UserInput
		if err := json.Unmarshal(sc.Bytes(), &in); err != nil {
			return nil, fmt.Errorf("inbox: bad line: %w", err)
		}
		if in.DeliverySeq > after {
			out = append(out, in)
		}
	}
	return out, sc.Err()
}

func lastInboxSeq(sessionDir string) (int64, error) {
	entries, err := ReadInbox(sessionDir, 0)
	if err != nil {
		return 0, err
	}
	if len(entries) == 0 {
		return 0, nil
	}
	return entries[len(entries)-1].DeliverySeq, nil
}
