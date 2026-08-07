package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// writeSearchSession lays down one session journal: an opening input, one
// assistant reply, and optionally a tool call carrying a payload.
func writeSearchSession(t *testing.T, root, id, opening, reply, toolResult string) {
	t.Helper()
	dir := filepath.Join(root, id)
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()

	evs := []event.Envelope{
		mkEnv(t, event.TypeSessionStarted, &event.SessionStarted{
			SpecName: "demo", Model: "m", Prompt: opening,
			SubStateVersions: state.SubStateVersions()}),
		mkEnv(t, event.TypeInputReceived, &event.InputReceived{Text: opening, Source: "cli"}),
		mkEnv(t, event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}),
		mkEnv(t, event.TypeAssistantMessage, &event.AssistantMessage{
			GenStep: 1, Message: provider.Message{Role: provider.RoleAssistant,
				Parts: []provider.Part{{Kind: provider.PartText, Text: reply}}}}),
	}
	if toolResult != "" {
		evs = append(evs,
			mkEnv(t, event.TypeAssistantMessage, &event.AssistantMessage{
				GenStep: 1, Message: provider.Message{Role: provider.RoleAssistant,
					Parts: []provider.Part{{Kind: provider.PartToolCall, CallID: "c1",
						ToolName: "read_file", Args: json.RawMessage(`{"path":"secrets.txt"}`)}}}}),
			mkEnv(t, event.TypeActivityStarted, &event.ActivityStarted{
				ActivityID: "tool-c1", Kind: event.KindTool, Name: "read_file", CallID: "c1"}),
			mkEnv(t, event.TypeActivityCompleted, &event.ActivityCompleted{
				ActivityID: "tool-c1", Result: json.RawMessage(`"` + toolResult + `"`)}),
		)
	}
	for _, e := range evs {
		if _, err := es.Append(e); err != nil {
			t.Fatal(err)
		}
	}
}

// The capability this feature exists for: find the conversation that mentioned
// a thing, by its message text.
func TestSearchFindsMessageText(t *testing.T) {
	root := t.TempDir()
	writeSearchSession(t, root, "sess-aaaaaaaaaaaaaaaa", "hello there", "the answer is QA-87 indeed", "")
	writeSearchSession(t, root, "sess-bbbbbbbbbbbbbbbb", "unrelated opening", "nothing to see", "")

	res, err := searchSessions(root, "qa-87", searchOpts{Limit: 10, MaxSessions: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 1 {
		t.Fatalf("matches = %d, want 1: %+v", len(res.Matches), res.Matches)
	}
	m := res.Matches[0]
	if m.Session != "sess-aaaaaaaaaaaaaaaa" {
		t.Errorf("matched session = %q", m.Session)
	}
	if !strings.Contains(m.Snippet, "QA-87") {
		t.Errorf("snippet does not carry the match: %q", m.Snippet)
	}
	if m.Kind != "message" {
		t.Errorf("kind = %q, want message", m.Kind)
	}
	if res.SessionsScanned != 2 {
		t.Errorf("SessionsScanned = %d, want 2", res.SessionsScanned)
	}
}

// The reason this search is substring-based rather than reusing the BM25
// indexer: Chinese has no word boundaries, so a tokenizing search matches
// NOTHING here. If this test ever fails, bilingual search is silently broken.
func TestSearchMatchesCJKWithoutWordBoundaries(t *testing.T) {
	root := t.TempDir()
	writeSearchSession(t, root, "sess-cccccccccccccccc", "开场白", "我们把静止模型的边界讲清楚了", "")

	res, err := searchSessions(root, "静止模型", searchOpts{Limit: 10, MaxSessions: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 1 {
		t.Fatalf("CJK query matched %d sessions, want 1", len(res.Matches))
	}
	if !strings.Contains(res.Matches[0].Snippet, "静止模型") {
		t.Errorf("CJK snippet mangled: %q", res.Matches[0].Snippet)
	}
}

// Disclosure boundary: a tool result can hold whatever the workspace held, so
// a plain query must not reach into it. Opting in must be a deliberate act.
func TestSearchExcludesToolPayloadsUnlessAskedFor(t *testing.T) {
	root := t.TempDir()
	writeSearchSession(t, root, "sess-dddddddddddddddd", "opening", "a normal reply",
		"AKIA_EXAMPLE_TOKEN_VALUE")

	res, err := searchSessions(root, "AKIA_EXAMPLE", searchOpts{Limit: 10, MaxSessions: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 0 {
		t.Fatalf("default search reached into a tool payload: %+v", res.Matches)
	}

	opted, err := searchSessions(root, "AKIA_EXAMPLE", searchOpts{Limit: 10, MaxSessions: 10, IncludeTools: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(opted.Matches) != 1 || opted.Matches[0].Kind != "tool" {
		t.Fatalf("--include-tools did not surface the payload: %+v", opted.Matches)
	}
}

// A bound that is not reported reads as "not there". Both caps must be
// visible in the result, or an empty/short answer is misleading.
func TestSearchReportsItsOwnBounds(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"sess-1111111111111111", "sess-2222222222222222", "sess-3333333333333333"} {
		writeSearchSession(t, root, id, "opening", "shared needle here", "")
	}

	limited, err := searchSessions(root, "needle", searchOpts{Limit: 2, MaxSessions: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited.Matches) != 2 || !limited.Truncated {
		t.Errorf("limit not reported: matches=%d truncated=%v", len(limited.Matches), limited.Truncated)
	}

	capped, err := searchSessions(root, "needle", searchOpts{Limit: 10, MaxSessions: 2})
	if err != nil {
		t.Fatal(err)
	}
	if capped.SessionsSkipped != 1 {
		t.Errorf("SessionsSkipped = %d, want 1 — an unsearched session must be declared",
			capped.SessionsSkipped)
	}
}

// A missing store is an empty corpus, and an unreadable session must not
// abort the whole query.
func TestSearchToleratesMissingAndUnreadable(t *testing.T) {
	res, err := searchSessions(filepath.Join(t.TempDir(), "nope"), "x", searchOpts{Limit: 5, MaxSessions: 5})
	if err != nil {
		t.Fatalf("missing store should be empty, got %v", err)
	}
	if len(res.Matches) != 0 {
		t.Errorf("matches from a missing store: %+v", res.Matches)
	}
}
