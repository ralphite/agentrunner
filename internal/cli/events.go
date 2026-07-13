package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ralphite/agentrunner/internal/driver"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// eventsCmd implements `agentrunner events <session> [--state] [--json]`:
// the debug window into a session's event log and its fold.
func eventsCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("events", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dumpState := fs.Bool("state", false, "print the folded state instead of the event list")
	asJSON := fs.Bool("json", false, "raw JSONL output (with --state: indented state JSON)")
	// All events flags are bool, so flags-after-positional can be supported
	// by partitioning before Parse (stdlib flag stops at the first non-flag).
	var flagArgs, positional []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
		} else {
			positional = append(positional, a)
		}
	}
	if err := fs.Parse(append(flagArgs, positional...)); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner events <session-id-or-prefix> [--state] [--json]")
		return ExitUsage
	}

	dir, err := resolveSessionDir(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	events, err := store.ReadEvents(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}

	if *dumpState {
		var folded any
		if isDriverJournal(events) {
			s, ferr := driver.Fold(events)
			if ferr != nil {
				fmt.Fprintf(stderr, "agentrunner: driver fold: %v\n", ferr)
				return ExitRun
			}
			folded = s
		} else {
			s, ferr := state.Fold(events)
			if ferr != nil {
				fmt.Fprintf(stderr, "agentrunner: fold: %v\n", ferr)
				return ExitRun
			}
			folded = s
		}
		raw, err := json.MarshalIndent(folded, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "agentrunner: %v\n", err)
			return ExitRun
		}
		fmt.Fprintln(stdout, string(raw))
		return ExitOK
	}

	for _, e := range events {
		if *asJSON {
			raw, err := json.Marshal(e)
			if err != nil {
				fmt.Fprintf(stderr, "agentrunner: %v\n", err)
				return ExitRun
			}
			fmt.Fprintln(stdout, string(raw))
			continue
		}
		fmt.Fprintf(stdout, "%5d  %s  %-20s %s\n",
			e.Seq, e.TS.Format("15:04:05.000"), e.Type, compactPayload(e.Payload, 100))
	}
	return ExitOK
}

func compactPayload(raw json.RawMessage, max int) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	s := buf.String()
	if len(s) > max {
		s = s[:max] + "…"
	}
	// Journal payloads carry user/model text verbatim; bidi control
	// characters (U+202A–U+202E, U+2066–U+2069) can visually reorder the
	// terminal line and disguise what actually ran (QA Round4 F-J5) —
	// escape them (and stray C0 controls) for display only.
	return strings.Map(func(r rune) rune {
		if (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) {
			return '\uFFFD'
		}
		if r < 0x20 && r != '\t' {
			return '\uFFFD'
		}
		return r
	}, s)
}

// resolveSessionDir maps a session id or unique prefix to its directory.
// A CHILD session (INC-1) is addressed by its FULL id: every "-sub-"
// segment maps to a "/sub/" path step under the parent's directory —
// `<parent>-sub-<call>-a<n>` → `sessions/<parent>/sub/<call>-a<n>`,
// nesting recursively for grandchildren. Child ids get no prefix matching:
// spawn/settle events carry the full id verbatim, so it is always at hand.
//
// A TOP-LEVEL slug may itself contain "-sub-" (ids are minted from free
// prompt text — "spawn 3 sub-agents…" → "…-worker-sub-age-8588"), so child
// addressing must never shadow an existing top-level session: exact
// top-level match wins, then child split points are tried longest-parent
// first (below the top level the split is unambiguous — call ids are
// harness-minted `call_%d_%d` and never contain "-sub-"), and finally
// ordinary prefix matching. QA Round1 F-B2.
func resolveSessionDir(idOrPrefix string) (string, error) {
	if strings.TrimSpace(idOrPrefix) == "" {
		// "" would Stat() the sessions root itself and wander into an
		// internal-path error (QA Round4 F-J4).
		return "", fmt.Errorf("no session given — pass a session id or unique prefix (see agentrunner sessions)")
	}
	if !runtime.ValidSessionID(idOrPrefix) {
		return "", fmt.Errorf("invalid session id or prefix %q", idOrPrefix)
	}
	data, err := runtime.DataDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(data, "sessions")
	if dir := filepath.Join(root, idOrPrefix); safeSessionDir(root, dir) {
		if !validSessionDir(dir) {
			return "", fmt.Errorf("session %q is incomplete (no journal genesis); start a new session", idOrPrefix)
		}
		return dir, nil
	}
	for i := strings.LastIndex(idOrPrefix, "-sub-"); i >= 0; i = strings.LastIndex(idOrPrefix[:i], "-sub-") {
		parent, rest := idOrPrefix[:i], idOrPrefix[i+len("-sub-"):]
		dir := filepath.Join(root, parent, "sub", strings.ReplaceAll(rest, "-sub-", "/sub/"))
		if safeSessionDir(root, dir) {
			if !validSessionDir(dir) {
				return "", fmt.Errorf("session %q is incomplete (no journal genesis); start a new session", idOrPrefix)
			}
			return dir, nil
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		// The empty-state common case: the sessions dir does not exist yet.
		// Say so plainly instead of leaking the internal XDG path in an
		// "open …/sessions: no such file" wrap (黑盒 R2 minor).
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no sessions yet — start one with `agentrunner run` or `agentrunner new`")
		}
		return "", fmt.Errorf("cannot read sessions: %w", err)
	}
	var matches []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !validSessionDir(filepath.Join(root, e.Name())) {
			continue
		}
		if e.Name() == idOrPrefix {
			return filepath.Join(root, e.Name()), nil
		}
		if strings.HasPrefix(e.Name(), idOrPrefix) {
			matches = append(matches, e.Name())
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session matches %q", idOrPrefix)
	case 1:
		return filepath.Join(root, matches[0]), nil
	default:
		sort.Strings(matches)
		// A short prefix can match half the store — a screenful of ids
		// helps nobody (QA Round1 F-A13). Show a sample and the count.
		if len(matches) > 5 {
			return "", fmt.Errorf("session prefix %q is ambiguous: %d sessions match (e.g. %s, …) — use a longer prefix or `agentrunner sessions`",
				idOrPrefix, len(matches), strings.Join(matches[:3], ", "))
		}
		return "", fmt.Errorf("session prefix %q is ambiguous: %s", idOrPrefix, strings.Join(matches, ", "))
	}
}

// safeSessionDir proves that a constructed session path names the same real
// directory beneath the shared store. Lstat rejects a final symlink; comparing
// logical and real relative paths also rejects an intermediate symlink alias or
// escape while tolerating platform aliases in the root itself (/tmp on macOS).
func safeSessionDir(root, dir string) bool {
	st, err := os.Lstat(dir)
	if err != nil || !st.IsDir() {
		return false
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	want, err := filepath.Rel(filepath.Clean(root), filepath.Clean(dir))
	if err != nil || want == "." || want == ".." || strings.HasPrefix(want, ".."+string(filepath.Separator)) {
		return false
	}
	got, err := filepath.Rel(realRoot, realDir)
	return err == nil && got == want
}

func validSessionDir(dir string) bool {
	events, err := store.ReadEvents(dir)
	if err != nil || len(events) == 0 {
		return false
	}
	switch events[0].Type {
	case event.TypeSessionStarted, event.TypeDriverStarted:
		return true
	case event.TypeForkedFrom:
		return len(events) > 1 && events[1].Type == event.TypeSessionStarted
	default:
		return false
	}
}

// splitSessionAddress is the store-aware address resolver wired into the
// daemon (Server.SplitAddress): a top-level session wins even when its
// slug contains "-sub-" (QA Round1 F-B2); a child resolves to its tree
// root as host. Unknown addresses fall back to the structural first-split
// so error paths keep their historic shape.
func splitSessionAddress(session string) (host, target string) {
	if dir, err := resolveSessionDir(session); err == nil {
		if data, derr := runtime.DataDir(); derr == nil {
			root := filepath.Join(data, "sessions")
			if rel, rerr := filepath.Rel(root, dir); rerr == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				parts := strings.Split(rel, string(filepath.Separator))
				if len(parts) == 1 {
					return parts[0], "" // top-level, full id
				}
				return parts[0], session // child: hosted by the tree root
			}
		}
	}
	if idx := strings.Index(session, "-sub-"); idx > 0 {
		return session[:idx], session
	}
	return session, ""
}
