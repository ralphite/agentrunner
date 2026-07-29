package agent

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/state"
)

// autotitleSystemPrompt instructs the title distiller. Like the compaction
// summarizer it is a harness-owned maintenance prompt, not the agent's own
// turn — the model never directed it.
// The instruction never uses the phrase it is asking for. Saying "reply with a
// short title" got weaker models to answer with the literal words "Short
// title" often enough to reach real sessions — the instruction's own vocabulary
// is the likeliest thing to be echoed, so it names the work instead, and the
// examples show the shape rather than describing it.
const autotitleSystemPrompt = "Read the user's opening message and name the work it asks for, in 3 to 6 " +
	"words. Reply with ONLY that name: no quotes, no trailing punctuation, no preamble, no explanation. " +
	"Write it in the language the user wrote in. Keep it under 60 characters.\n\n" +
	"Opening message: can you add retry logic to the http client, it keeps dying on 503s\n" +
	"Add retry to HTTP client\n\n" +
	"Opening message: 帮我看看为什么 CI 上的测试挂了，本地是好的\n" +
	"排查 CI 测试失败"

// autotitleActivityID is the stable id of the one title-distilling LLM
// activity per session: gated to fire once, it never collides with a turn.
const autotitleActivityID = "autotitle"

// autotitleMaxRunes caps the stored title; a runaway model reply is truncated
// rather than journaled whole.
const autotitleMaxRunes = 80

// autotitleSkipShortRunes: an opening prompt that is already a single short line
// makes a fine first-line fallback title on its own — distilling it would just
// spend a call to restate it. Skip generation below this length.
const autotitleSkipShortRunes = 48

// maybeAutoTitle distils the opening user message into a short display title
// once per session and journals it as SessionTitled{source:auto} (INC-52,
// HANDA-PARITY #14). It is a harness-internal maintenance LLM call in the same
// family as the compaction summarizer and the goal judge: a recorded llm_call
// Activity (usage settles into the budget), NOT permission-gated, and crash-
// idempotent — a completed activity reuses its recorded result, and once the
// SessionTitled event lands the fold's TitleSource closes the gate for good.
//
// It runs at the loop's safe boundary AFTER the opening turn's assistant
// message has landed, so it never delays the user-visible opening reply. A
// title is cosmetic: an LLM failure is swallowed (the surfaces fall back to the
// opening first line) and only a store-append failure surfaces as an error.
func (l *Loop) maybeAutoTitle(ctx context.Context, ds *driveState, appendE AppendFunc) error {
	// Already titled (crash-replay safe: SessionTitled folded TitleSource), or
	// already attempted-and-given-up this process — nothing to do.
	if ds.s.Session.TitleSource != "" || l.titleTried {
		return nil
	}
	// Scope: enabled only on a top-level hosted session (the daemon sets
	// AutoTitle; a headless one-shot, a spawned child, a driver iteration, and
	// every scripted test leave it false) with a real provider at the root.
	if !l.AutoTitle || l.Depth != 0 || l.Provider == nil {
		l.titleTried = true
		return nil
	}
	// Not yet ready: the opening turn's assistant message has not landed. Do
	// NOT set titleTried — retry at the next safe boundary once it has. This
	// is what keeps the opening turn unblocked.
	if !hasAssistantMessage(ds.s) {
		return nil
	}
	prompt := strings.TrimSpace(ds.s.Session.Prompt)
	first := firstLineTrim(prompt)
	if first == "" {
		l.titleTried = true
		return nil
	}
	// A single short opening line is already a good title — the first-line
	// fallback restates it for free.
	if !strings.Contains(prompt, "\n") && utf8.RuneCountInString(first) <= autotitleSkipShortRunes {
		l.titleTried = true
		return nil
	}
	// This session attempts the distillation at most once per process.
	l.titleTried = true

	// Crash window: a title activity already completed (append of SessionTitled
	// crashed in between) → reuse its recorded result rather than re-calling.
	// The result is the cleaned title as a JSON string.
	if res, ok, err := completedVerifierResult(l, autotitleActivityID); err != nil {
		return err
	} else if ok {
		if title := decodeTitleResult(res.Payload); title != "" {
			return l.journalAutoTitle(appendE, title)
		}
		return nil
	}

	req := provider.CompleteRequest{
		Model:     l.Spec.Model.ID,
		MaxTokens: 64,
		System:    autotitleSystemPrompt,
		Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{
			{Kind: provider.PartText, Text: prompt}}}},
	}
	var title string
	// A dedicated single-attempt executor: a cosmetic title must not retry-storm
	// against a flaky provider or stall the boundary — one try, then fall back.
	exec := &ActivityExecutor{Append: appendE, Clock: l.Clock, Redact: redact.FromEnv(), MaxAttempts: 1}
	if err := exec.Do(ctx, Activity{
		ID: autotitleActivityID, Kind: event.KindLLM, Name: "autotitle", Idempotent: true,
		Run: func(runCtx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
			turn, cerr := provider.CollectTurnStreaming(l.Provider.Complete(runCtx, req), func(string) {})
			if cerr != nil {
				return nil, &turn.Usage, false, cerr
			}
			title = cleanTitle(assistantText(turn.Message))
			u := turn.Usage
			// Record the cleaned title (JSON string) so a crash between here and
			// the SessionTitled append reuses it instead of re-calling.
			payload, _ := json.Marshal(title)
			return payload, &u, false, nil
		},
	}); err != nil {
		// Cosmetic: a title that cannot be generated is not an error — the
		// surfaces keep the opening first line. The activity's own failure fact
		// is already journaled; swallow here and never abort the session.
		l.emit(protocol.Event{Kind: protocol.KindMessage,
			Text: "auto-title skipped: " + redact.FromEnv().String(err.Error())})
		return nil
	}
	if title == "" || titleEchoesInstruction(title) {
		return nil // nothing usable: keep the first-line fallback
	}
	return l.journalAutoTitle(appendE, title)
}

// titleEchoesInstruction reports whether a reply restates what was asked for
// instead of naming the work — "Short title", "Session name", "Untitled". The
// prompt above avoids handing the model those words, but a weak model finds
// them anyway, and the opening line is a strictly better name than any of
// them. Belt to the prompt's braces: this is what a real session shipped as
// its display name before both were in place.
func titleEchoesInstruction(title string) bool {
	switch strings.ToLower(strings.TrimSpace(title)) {
	case "title", "short title", "session", "session title", "session name",
		"work session", "name", "untitled", "标题", "会话标题":
		return true
	}
	return false
}

// decodeTitleResult reads a recorded autotitle activity result (a JSON string
// of the cleaned title). Anything unparseable yields "" — fall back.
func decodeTitleResult(payload json.RawMessage) string {
	var title string
	if err := json.Unmarshal(payload, &title); err != nil {
		return ""
	}
	return strings.TrimSpace(title)
}

// journalAutoTitle lands the SessionTitled{source:auto} fact. A store failure
// is the only hard error the auto-title path surfaces (append is fatal, like
// every other drain's).
func (l *Loop) journalAutoTitle(appendE AppendFunc, title string) error {
	if _, err := appendE(event.TypeSessionTitled, &event.SessionTitled{
		Title: title, Source: event.TitleSourceAuto,
	}); err != nil {
		return err
	}
	l.emit(protocol.Event{Kind: protocol.KindMessage, Text: "session titled: " + title})
	return nil
}

// hasAssistantMessage reports whether the opening turn has produced its
// assistant message yet — the gate that keeps auto-title off the opening turn.
func hasAssistantMessage(s state.State) bool {
	for _, m := range s.Conversation.Messages {
		if m.Role == provider.RoleAssistant {
			return true
		}
	}
	return false
}

// firstLineTrim returns the first non-empty line, trimmed.
func firstLineTrim(s string) string {
	return strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
}

// cleanTitle normalizes a model-produced title: first line only, collapsed
// whitespace, stripped wrapping quotes/backticks, trailing punctuation dropped,
// and capped to autotitleMaxRunes. Trimming and unwrapping run to a fixed point
// so a quoted-then-punctuated reply ("Fix it".) unwinds fully.
func cleanTitle(raw string) string {
	t := firstLineTrim(raw)
	t = strings.Join(strings.Fields(t), " ") // collapse internal whitespace
	for i := 0; i < 4; i++ {
		before := t
		t = strings.TrimRight(strings.TrimSpace(t), " .;,。；，")
		t = unwrapTitle(t)
		if t == before {
			break
		}
	}
	if utf8.RuneCountInString(t) > autotitleMaxRunes {
		runes := []rune(t)
		t = strings.TrimRight(string(runes[:autotitleMaxRunes]), " ") + "…"
	}
	return t
}

// unwrapTitle strips one matching pair of wrapping quotes/backticks.
func unwrapTitle(t string) string {
	if t == "" {
		return t
	}
	pairs := map[rune]rune{'`': '`', '"': '"', '\'': '\'', '“': '”', '‘': '’'}
	r := []rune(t)
	if end, ok := pairs[r[0]]; ok && len(r) >= 2 && r[len(r)-1] == end {
		return strings.TrimSpace(string(r[1 : len(r)-1]))
	}
	return t
}
