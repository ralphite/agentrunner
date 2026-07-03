// Package store holds persistence. In S1 this is journal v0: an append-only
// JSONL observation log — record-only, never read back by the harness.
// S2 upgrades this package into the EventStore (source of truth).
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ralphite/agentrunner/internal/provider"
)

// Journal is an append-only JSONL writer for one session.
type Journal struct {
	mu sync.Mutex
	f  *os.File
}

// OpenJournal opens (creating, 0600) the journal file for appending.
func OpenJournal(path string) (*Journal, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("journal: %w", err)
	}
	return &Journal{f: f}, nil
}

// Close flushes and closes the journal.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.f.Close()
}

// record is the S1 line shape: {"type", "ts", "data": {…}} (nested data by
// decision — unambiguous and forward-parseable).
type record struct {
	Type string `json:"type"`
	TS   string `json:"ts"`
	Data any    `json:"data"`
}

func (j *Journal) append(typ string, data any) error {
	line, err := json.Marshal(record{
		Type: typ,
		TS:   time.Now().UTC().Format(time.RFC3339Nano),
		Data: data,
	})
	if err != nil {
		return fmt.Errorf("journal %s: %w", typ, err)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, err := j.f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("journal %s: %w", typ, err)
	}
	return nil
}

// RunMeta records run identity (first line of every journal).
type RunMeta struct {
	SpecName string `json:"spec_name"`
	Model    string `json:"model"`
	Task     string `json:"task"`
	Version  string `json:"version"`
}

// RecordRunMeta writes the run_meta record.
func (j *Journal) RecordRunMeta(m RunMeta) error {
	return j.append("run_meta", m)
}

// RecordAssistantMessage writes one assistant turn message.
func (j *Journal) RecordAssistantMessage(turn int, msg provider.Message) error {
	return j.append("assistant_message", map[string]any{"turn": turn, "message": msg})
}

// RecordToolCall writes one requested tool call.
func (j *Journal) RecordToolCall(turn int, call provider.ToolCall) error {
	return j.append("tool_call", map[string]any{
		"turn": turn, "call_id": call.CallID, "name": call.Name, "args": call.Args,
	})
}

// RecordToolResult writes one tool execution outcome.
func (j *Journal) RecordToolResult(turn int, callID, name string, result json.RawMessage, isError bool) error {
	return j.append("tool_result", map[string]any{
		"turn": turn, "call_id": callID, "name": name, "result": result, "is_error": isError,
	})
}

// RecordRunEnd writes the terminal record.
func (j *Journal) RecordRunEnd(reason string, turns int, usage provider.Usage) error {
	return j.append("run_end", map[string]any{
		"reason": reason, "turns": turns, "usage": usage,
	})
}
