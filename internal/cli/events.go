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
		return s[:max] + "…"
	}
	return s
}

// resolveSessionDir maps a session id or unique prefix to its directory.
// A CHILD session (INC-1) is addressed by its FULL id: every "-sub-"
// segment maps to a "/sub/" path step under the parent's directory —
// `<parent>-sub-<call>-a<n>` → `sessions/<parent>/sub/<call>-a<n>`,
// nesting recursively for grandchildren. The split is unambiguous because
// call ids are harness-minted (`call_%d_%d`, provider.CallID) and never
// contain "-sub-". Child ids get no prefix matching: spawn/settle events
// carry the full id verbatim, so it is always at hand.
func resolveSessionDir(idOrPrefix string) (string, error) {
	data, err := runtime.DataDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(data, "sessions")
	if strings.Contains(idOrPrefix, "-sub-") {
		dir := filepath.Join(root, strings.ReplaceAll(idOrPrefix, "-sub-", "/sub/"))
		if st, serr := os.Stat(dir); serr != nil || !st.IsDir() {
			return "", fmt.Errorf("no child session %q", idOrPrefix)
		}
		return dir, nil
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
		return "", fmt.Errorf("session prefix %q is ambiguous: %s", idOrPrefix, strings.Join(matches, ", "))
	}
}
