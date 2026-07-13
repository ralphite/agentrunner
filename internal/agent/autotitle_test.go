package agent

import (
	"context"
	"encoding/json"
	"iter"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// titleProvider is a one-shot fake for the auto-title distiller: it returns a
// fixed reply (or an error) and counts how many times it was asked, so a test
// can assert the LLM call happened exactly once — or never.
type titleProvider struct {
	text  string
	err   error
	calls int
}

func (p *titleProvider) Capabilities() provider.Capabilities { return provider.Capabilities{} }
func (p *titleProvider) Complete(_ context.Context, _ provider.CompleteRequest) iter.Seq2[provider.StreamEvent, error] {
	return func(yield func(provider.StreamEvent, error) bool) {
		p.calls++
		if p.err != nil {
			yield(provider.StreamEvent{}, p.err)
			return
		}
		yield(provider.StreamEvent{Kind: provider.EventTextDelta, TextDelta: p.text}, nil)
		yield(provider.StreamEvent{Kind: provider.EventFinish, Finish: provider.FinishEndTurn}, nil)
	}
}

// titleTestLoop builds a root, interactively-hosted Loop (UserInputs wired,
// Depth 0) over a fresh store, plus its driveState and appender.
func titleTestLoop(t *testing.T, p provider.Provider) (*Loop, *driveState, AppendFunc) {
	t.Helper()
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })
	l := &Loop{
		Spec:       &AgentSpec{Name: "t", Model: ModelSpec{Provider: "x", ID: "m", MaxTokens: 100}},
		Provider:   p,
		Store:      es,
		Clock:      clock.NewFake(time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)),
		SessionID:  "title-test",
		UserInputs: make(chan protocol.UserInput), // interactively hosted
		AutoTitle:  true,
	}
	ds := &driveState{s: state.New()}
	return l, ds, l.appender(ds)
}

// openTurn folds the opening turn into the loop's state/store: a SessionStarted
// with the given prompt, then the opening assistant message (so the auto-title
// gate — "opening reply landed" — is open).
func openTurn(t *testing.T, appendE AppendFunc, prompt string) {
	t.Helper()
	if _, err := appendE(event.TypeSessionStarted, &event.SessionStarted{
		SpecName: "t", Model: "m", Prompt: prompt, Version: "dev",
		SubStateVersions: state.SubStateVersions()}); err != nil {
		t.Fatal(err)
	}
	if _, err := appendE(event.TypeInputReceived, &event.InputReceived{Text: prompt, Source: "user"}); err != nil {
		t.Fatal(err)
	}
	if _, err := appendE(event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := appendE(event.TypeAssistantMessage, &event.AssistantMessage{GenStep: 1,
		Message: provider.Message{Role: provider.RoleAssistant,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "on it"}}}}); err != nil {
		t.Fatal(err)
	}
}

func countTitled(t *testing.T, dir string) (n int, last *event.SessionTitled) {
	t.Helper()
	for _, e := range readEvents(t, dir) {
		if e.Type != event.TypeSessionTitled {
			continue
		}
		n++
		decoded, err := event.DecodePayload(e)
		if err != nil {
			t.Fatal(err)
		}
		last = decoded.(*event.SessionTitled)
	}
	return n, last
}

const longPrompt = "Please refactor the authentication boundary so every request is checked once,\n" +
	"and add regression tests for the three bypass cases we found last week."

// INC-52: a root interactive session distils the opening message into exactly
// one SessionTitled{auto}, the fold projects it, and a second pass is a no-op
// (idempotent — the folded TitleSource closes the gate).
func TestAutoTitleGeneratesOnceAndFoldsProjection(t *testing.T) {
	p := &titleProvider{text: "  \"Refactor the auth boundary\". "}
	l, ds, appendE := titleTestLoop(t, p)
	openTurn(t, appendE, longPrompt)

	for i := 0; i < 2; i++ {
		if err := l.maybeAutoTitle(context.Background(), ds, appendE); err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
	}
	if p.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", p.calls)
	}
	if ds.s.Session.RawTitle != "Refactor the auth boundary" || ds.s.Session.TitleSource != event.TitleSourceAuto {
		t.Fatalf("fold RawTitle/TitleSource = %q/%q", ds.s.Session.RawTitle, ds.s.Session.TitleSource)
	}
	n, last := countTitled(t, l.Store.Dir())
	if n != 1 || last.Title != "Refactor the auth boundary" || last.Source != event.TitleSourceAuto {
		t.Fatalf("titled events = %d, last = %+v", n, last)
	}
}

// INC-52: the auto-title never fires before the opening turn's assistant
// message lands — that is what keeps the opening reply unblocked. It retries at
// a later boundary (titleTried stays false).
func TestAutoTitleWaitsForOpeningReply(t *testing.T) {
	p := &titleProvider{text: "Refactor the auth boundary"}
	l, ds, appendE := titleTestLoop(t, p)
	// Opening input journaled, but NO assistant message yet.
	if _, err := appendE(event.TypeSessionStarted, &event.SessionStarted{
		SpecName: "t", Model: "m", Prompt: longPrompt, Version: "dev",
		SubStateVersions: state.SubStateVersions()}); err != nil {
		t.Fatal(err)
	}
	if _, err := appendE(event.TypeInputReceived, &event.InputReceived{Text: longPrompt, Source: "user"}); err != nil {
		t.Fatal(err)
	}
	if err := l.maybeAutoTitle(context.Background(), ds, appendE); err != nil {
		t.Fatal(err)
	}
	if p.calls != 0 || l.titleTried {
		t.Fatalf("fired too early: calls=%d titleTried=%v", p.calls, l.titleTried)
	}
	if n, _ := countTitled(t, l.Store.Dir()); n != 0 {
		t.Fatalf("titled events = %d, want 0", n)
	}
}

// INC-52: auto never overrides a manual title (server-side, were one to exist).
// A folded manual TitleSource closes the gate — the provider is never called.
func TestAutoTitleDoesNotOverrideManual(t *testing.T) {
	p := &titleProvider{text: "an auto guess"}
	l, ds, appendE := titleTestLoop(t, p)
	openTurn(t, appendE, longPrompt)
	if _, err := appendE(event.TypeSessionTitled, &event.SessionTitled{
		Title: "My renamed prompt", Source: event.TitleSourceManual}); err != nil {
		t.Fatal(err)
	}
	if err := l.maybeAutoTitle(context.Background(), ds, appendE); err != nil {
		t.Fatal(err)
	}
	if p.calls != 0 {
		t.Fatalf("provider called despite manual title: calls=%d", p.calls)
	}
	if ds.s.Session.RawTitle != "My renamed prompt" || ds.s.Session.TitleSource != event.TitleSourceManual {
		t.Fatalf("manual title clobbered: %q/%q", ds.s.Session.RawTitle, ds.s.Session.TitleSource)
	}
}

// INC-52: a short single-line opening prompt is its own good title — skip the
// call and keep the first-line fallback.
func TestAutoTitleSkipsShortPrompt(t *testing.T) {
	p := &titleProvider{text: "should not run"}
	l, ds, appendE := titleTestLoop(t, p)
	openTurn(t, appendE, "make it loud")
	if err := l.maybeAutoTitle(context.Background(), ds, appendE); err != nil {
		t.Fatal(err)
	}
	if p.calls != 0 || !l.titleTried {
		t.Fatalf("short prompt: calls=%d titleTried=%v", p.calls, l.titleTried)
	}
	if n, _ := countTitled(t, l.Store.Dir()); n != 0 {
		t.Fatalf("titled events = %d, want 0", n)
	}
}

// INC-52: a title is cosmetic — an LLM failure is swallowed (no SessionTitled,
// no error, the session is never aborted), and the fallback first line stands.
func TestAutoTitleSwallowsLLMFailure(t *testing.T) {
	p := &titleProvider{err: context.DeadlineExceeded}
	l, ds, appendE := titleTestLoop(t, p)
	openTurn(t, appendE, longPrompt)
	if err := l.maybeAutoTitle(context.Background(), ds, appendE); err != nil {
		t.Fatalf("LLM failure must not surface: %v", err)
	}
	if ds.s.Session.TitleSource != "" {
		t.Fatalf("title landed despite failure: %q", ds.s.Session.RawTitle)
	}
	if n, _ := countTitled(t, l.Store.Dir()); n != 0 {
		t.Fatalf("titled events = %d, want 0", n)
	}
}

// INC-52: crash between the title activity's ActivityCompleted and the
// SessionTitled append — a replay REUSES the recorded activity result and does
// NOT re-call the provider (崩溃重放不重复生成).
func TestAutoTitleReusesRecordedResultOnReplay(t *testing.T) {
	p := &titleProvider{text: "should not run"}
	l, ds, appendE := titleTestLoop(t, p)
	openTurn(t, appendE, longPrompt)
	// Simulate the crash window: the title activity already recorded its result
	// (a JSON string of the cleaned title), but SessionTitled never landed.
	payload, _ := json.Marshal("Reused auth-boundary title")
	if _, err := appendE(event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: autotitleActivityID, Kind: event.KindLLM, Name: "autotitle", Attempt: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := appendE(event.TypeActivityCompleted, &event.ActivityCompleted{
		ActivityID: autotitleActivityID, Result: payload}); err != nil {
		t.Fatal(err)
	}
	if err := l.maybeAutoTitle(context.Background(), ds, appendE); err != nil {
		t.Fatal(err)
	}
	if p.calls != 0 {
		t.Fatalf("provider re-called on replay: calls=%d", p.calls)
	}
	n, last := countTitled(t, l.Store.Dir())
	if n != 1 || last.Title != "Reused auth-boundary title" || last.Source != event.TitleSourceAuto {
		t.Fatalf("replay titled = %d, last = %+v", n, last)
	}
}
