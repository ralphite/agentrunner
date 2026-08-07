package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// `ar search` finds a message across sessions (G44). The sibling capability
// — the command palette / sidebar filter — only ever matched titles and ids
// of ALREADY-LOADED sessions, so "which conversation was that in?" was
// unanswerable once history outgrew one page.
//
// Design choices worth stating, because each has a tempting wrong answer:
//
//   - SCAN, DO NOT INDEX. A resident index buys speed and costs correctness:
//     it must be invalidated, and a session written one millisecond ago is
//     exactly the one a user searches for. Scanning is fresh BY CONSTRUCTION,
//     so "new journal visibility" is not a policy to get right — it cannot be
//     wrong. Bounds (below) keep the cost honest instead.
//   - SUBSTRING, NOT TOKENS. The BM25 indexer in internal/index tokenizes on
//     word boundaries, which silently matches NOTHING for Chinese/Japanese
//     text — it has no spaces. A case-folded substring match is worse at
//     ranking English prose and is the only thing that works at all for CJK,
//     and this repo's conversations are bilingual.
//   - TOOL PAYLOADS ARE OFF BY DEFAULT. Tool args and results carry file
//     contents, command output, and whatever the workspace held. Searching
//     them by default would turn one careless query into a disclosure of data
//     the user was not looking at. --include-tools is the opt-in.
//   - RECENCY ORDER, NOT A RELEVANCE SCORE. Substring matching yields no
//     honest relevance signal; inventing one would rank arbitrarily while
//     looking authoritative. Newest session first is a claim we can defend.
func searchCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonFlag := fs.Bool("json", false, "emit matches as JSON")
	limitFlag := fs.Int("limit", searchDefaultLimit, "maximum matches to return")
	maxSessions := fs.Int("max-sessions", searchDefaultSessions, "newest sessions to scan")
	includeTools := fs.Bool("include-tools", false, "also search tool arguments and results (may surface file contents)")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		fmt.Fprintln(stderr, "usage: agentrunner search [--json] [--limit N] [--max-sessions N] [--include-tools] <query>")
		return ExitUsage
	}
	if *limitFlag <= 0 || *maxSessions <= 0 {
		fmt.Fprintln(stderr, "agentrunner: --limit and --max-sessions must be positive")
		return ExitUsage
	}
	if *limitFlag > searchMaxLimit {
		*limitFlag = searchMaxLimit
	}

	data, err := runtime.DataDir()
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	res, err := searchSessions(filepath.Join(data, "sessions"), query, searchOpts{
		Limit: *limitFlag, MaxSessions: *maxSessions, IncludeTools: *includeTools,
	})
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}

	if *jsonFlag {
		raw, mErr := json.MarshalIndent(res, "", "  ")
		if mErr != nil {
			fmt.Fprintf(stderr, "agentrunner: %v\n", mErr)
			return ExitRun
		}
		fmt.Fprintln(stdout, string(raw))
		return ExitOK
	}
	if len(res.Matches) == 0 {
		fmt.Fprintf(stdout, "no matches for %q (scanned %d session(s))\n", query, res.SessionsScanned)
	}
	for _, m := range res.Matches {
		title := m.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(stdout, "%s  %s\n    %s: %s\n", m.Session, title, m.Role, m.Snippet)
	}
	// Never let a bound pass for completeness: a user who does not know the
	// scan stopped early will read "no matches" as "it is not there".
	if res.Truncated {
		fmt.Fprintf(stdout, "\n(stopped at %d matches; narrow the query or raise --limit)\n", res.Limit)
	}
	if res.SessionsSkipped > 0 {
		fmt.Fprintf(stdout, "(scanned the %d newest sessions; %d older not searched — raise --max-sessions)\n",
			res.SessionsScanned, res.SessionsSkipped)
	}
	return ExitOK
}

const (
	searchDefaultLimit    = 30
	searchMaxLimit        = 200
	searchDefaultSessions = 200
	// searchSnippetRunes bounds each snippet. Runes, not bytes: a byte window
	// would slice a multi-byte character in half and emit mojibake for exactly
	// the CJK text this search exists to find.
	searchSnippetRunes = 160
)

type searchOpts struct {
	Limit        int
	MaxSessions  int
	IncludeTools bool
}

// SearchMatch is one hit. One match per session per role-run keeps a single
// chatty session from crowding out every other result.
type searchMatch struct {
	Session string `json:"session"`
	Title   string `json:"title,omitempty"`
	Role    string `json:"role"`
	Snippet string `json:"snippet"`
	// Kind distinguishes conversation text from a tool payload so a consumer
	// can style (or refuse to show) the latter.
	Kind string `json:"kind"`
}

type searchResult struct {
	Query   string        `json:"query"`
	Matches []searchMatch `json:"matches"`
	// The bounds are part of the ANSWER, not diagnostics: without them an
	// empty result is indistinguishable from an unsearched corpus.
	SessionsScanned int  `json:"sessions_scanned"`
	SessionsSkipped int  `json:"sessions_skipped"`
	Truncated       bool `json:"truncated"`
	Limit           int  `json:"limit"`
}

func searchSessions(root, query string, opt searchOpts) (searchResult, error) {
	res := searchResult{Query: query, Matches: []searchMatch{}, Limit: opt.Limit}
	needle := strings.ToLower(query)

	entries, err := os.ReadDir(root)
	if err != nil {
		// No session store yet is an empty corpus, not a failure — the same
		// reading `ar sessions` gives it.
		if os.IsNotExist(err) {
			return res, nil
		}
		return res, err
	}
	type candidate struct {
		name  string
		mtime int64
	}
	candidates := make([]candidate, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if !validSessionDir(dir) {
			continue
		}
		candidates = append(candidates, candidate{name: e.Name(), mtime: sessionTreeJournalMTime(dir)})
	}
	// Newest first, so a bounded scan keeps the sessions a user is most likely
	// to mean rather than an arbitrary directory-order slice.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].mtime > candidates[j].mtime })
	if len(candidates) > opt.MaxSessions {
		res.SessionsSkipped = len(candidates) - opt.MaxSessions
		candidates = candidates[:opt.MaxSessions]
	}

	for _, c := range candidates {
		if len(res.Matches) >= opt.Limit {
			res.Truncated = true
			break
		}
		res.SessionsScanned++
		events, rErr := store.ReadEvents(filepath.Join(root, c.name))
		if rErr != nil {
			continue // an unreadable session is skipped, never fatal to the query
		}
		s, fErr := state.Fold(events)
		if fErr != nil {
			continue
		}
		if m, ok := matchSession(s, needle, opt.IncludeTools); ok {
			m.Session = c.name
			m.Title = sessionDisplayTitle(s)
			res.Matches = append(res.Matches, m)
		}
	}
	if len(res.Matches) >= opt.Limit {
		res.Truncated = true
	}
	return res, nil
}

// matchSession returns the FIRST match in a session, newest message first —
// the freshest mention is the one a user is usually chasing back to.
func matchSession(s state.State, needle string, includeTools bool) (searchMatch, bool) {
	msgs := s.Conversation.Messages
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		for _, p := range m.Parts {
			switch p.Kind {
			case provider.PartText:
				if idx := indexFold(p.Text, needle); idx >= 0 {
					return searchMatch{Role: string(m.Role), Kind: "message",
						Snippet: snippetAround(p.Text, idx, len(needle))}, true
				}
			case provider.PartToolCall, provider.PartToolResult:
				if !includeTools {
					continue
				}
				// The call's args ride the message, but the RESULT is folded
				// into a side map keyed by call id — searching only the parts
				// would quietly miss every tool output, which is most of the
				// payload --include-tools is asking for.
				body := string(p.Args) + string(p.Result)
				if p.CallID != "" {
					body += string(s.Conversation.ToolResults[p.CallID].Result)
				}
				if idx := indexFold(body, needle); idx >= 0 {
					name := p.ToolName
					if name == "" {
						name = "tool"
					}
					return searchMatch{Role: name, Kind: "tool",
						Snippet: snippetAround(body, idx, len(needle))}, true
				}
			}
		}
	}
	return searchMatch{}, false
}

// sessionDisplayTitle mirrors the list surfaces: the folded title, else the
// opening prompt's first line — so a hit is identified the same way a user
// already sees that session named.
func sessionDisplayTitle(s state.State) string {
	if t := strings.TrimSpace(s.Session.RawTitle); t != "" {
		return t
	}
	if p := strings.TrimSpace(s.Session.Prompt); p != "" {
		return strings.TrimSpace(strings.SplitN(p, "\n", 2)[0])
	}
	return ""
}

// indexFold is a case-insensitive substring search. ToLower on both sides is
// deliberate: it leaves CJK untouched (no case to fold) while making ASCII
// queries behave the way every search box has taught users to expect.
func indexFold(haystack, loweredNeedle string) int {
	return strings.Index(strings.ToLower(haystack), loweredNeedle)
}

// snippetAround centers a rune-bounded window on the match and collapses
// whitespace so a snippet stays one readable line.
func snippetAround(text string, byteIdx, needleLen int) string {
	flat := strings.Join(strings.Fields(text), " ")
	// Re-locate in the flattened text: collapsing whitespace moves offsets.
	if i := strings.Index(strings.ToLower(flat), strings.ToLower(safeSlice(text, byteIdx, needleLen))); i >= 0 {
		byteIdx = i
	} else {
		byteIdx = 0
	}
	runes := []rune(flat)
	hitRune := utf8.RuneCountInString(flat[:min(byteIdx, len(flat))])
	start := hitRune - searchSnippetRunes/2
	if start < 0 {
		start = 0
	}
	end := start + searchSnippetRunes
	if end > len(runes) {
		end = len(runes)
		if start = end - searchSnippetRunes; start < 0 {
			start = 0
		}
	}
	out := string(runes[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}

// safeSlice extracts the matched text without splitting a rune.
func safeSlice(text string, idx, n int) string {
	if idx < 0 || idx > len(text) {
		return ""
	}
	end := idx + n
	if end > len(text) {
		end = len(text)
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}
	return text[idx:end]
}
