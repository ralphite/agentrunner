package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
)

func TestJournalWritesParseableRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	j, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(j.RecordRunMeta(RunMeta{SpecName: "hello", Model: "gemini-flash-latest", Task: "fix", Version: "dev"}))
	must(j.RecordAssistantMessage(1, provider.Message{Role: provider.RoleAssistant,
		Parts: []provider.Part{{Kind: provider.PartText, Text: "on it"}}}))
	must(j.RecordToolCall(1, provider.ToolCall{CallID: "call_1_0", Name: "bash", Args: json.RawMessage(`{"command":"ls"}`)}))
	must(j.RecordToolResult(1, "call_1_0", "bash", json.RawMessage(`{"stdout":"ok"}`), false))
	must(j.RecordRunEnd("completed", 1, provider.Usage{InputTokens: 3, OutputTokens: 4}))
	must(j.Close())

	// permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 0600", info.Mode().Perm())
	}

	// every line parses; types in order
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	wantTypes := []string{"run_meta", "assistant_message", "tool_call", "tool_result", "run_end"}
	var gotTypes []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec struct {
			Type string          `json:"type"`
			TS   string          `json:"ts"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("unparseable line: %s", sc.Text())
		}
		if rec.TS == "" || len(rec.Data) == 0 {
			t.Errorf("incomplete record: %s", sc.Text())
		}
		gotTypes = append(gotTypes, rec.Type)
	}
	for i, want := range wantTypes {
		if i >= len(gotTypes) || gotTypes[i] != want {
			t.Fatalf("types = %v, want %v", gotTypes, wantTypes)
		}
	}
}
