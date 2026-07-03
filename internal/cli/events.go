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
		s, err := state.Fold(events)
		if err != nil {
			fmt.Fprintf(stderr, "agentrunner: fold: %v\n", err)
			return ExitRun
		}
		raw, err := json.MarshalIndent(s, "", "  ")
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
func resolveSessionDir(idOrPrefix string) (string, error) {
	data, err := runtime.DataDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(data, "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("no sessions found (%v)", err)
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
