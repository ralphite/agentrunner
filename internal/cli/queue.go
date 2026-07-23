package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// queueCmd lists a session's PENDING next-turn inputs (INC-46): durable queue
// commands above the fold's consumed high-water, with their command id (the
// unqueue handle) and whether a revoke already covers them. Mid-turn steer
// inputs are deliberately absent: presenting them as withdrawable "Queued"
// work contradicts their current-turn delivery contract.
func queueCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("queue", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit the queue as JSON (for tooling / the web UI)")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, `usage: agentrunner queue <session-id-or-prefix>`)
		return ExitUsage
	}
	dir, err := resolveSessionDir(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	pending, revoked, err := pendingQueue(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	if *jsonOut {
		type row struct {
			CommandID string `json:"command_id"`
			Text      string `json:"text"`
			Revoked   bool   `json:"revoked"`
		}
		rows := make([]row, 0, len(pending))
		for _, in := range pending {
			rows = append(rows, row{CommandID: in.CommandID, Text: in.Text, Revoked: revoked[in.CommandID]})
		}
		b, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Fprintln(stdout, string(b))
		return ExitOK
	}
	if len(pending) == 0 {
		fmt.Fprintln(stdout, "no queued messages")
		return ExitOK
	}
	fmt.Fprintf(stdout, "%-28s %-9s %s\n", "COMMAND-ID", "STATE", "TEXT")
	for _, in := range pending {
		state := "queued"
		if revoked[in.CommandID] {
			state = "revoked"
		}
		text := strings.ReplaceAll(in.Text, "\n", " ")
		if len(text) > 60 {
			text = text[:60] + "…"
		}
		fmt.Fprintf(stdout, "%-28s %-9s %s\n", in.CommandID, state, text)
	}
	fmt.Fprintln(stdout, "\nwithdraw one: agentrunner unqueue <session> <command-id>")
	return ExitOK
}

// pendingQueue reads the durable command log and the fold's high-water:
// pending = next-turn queue commands not yet consumed; revoked = targets a
// revoke command already covers. Empty delivery is the legacy queue default.
func pendingQueue(dir string) ([]protocol.UserInput, map[string]bool, error) {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return nil, nil, err
	}
	s, err := state.Fold(events)
	if err != nil {
		return nil, nil, fmt.Errorf("fold: %w", err)
	}
	cmds, err := store.ReadCommands(dir, s.Session.ConsumedInputSeq)
	if err != nil {
		return nil, nil, err
	}
	revoked := map[string]bool{}
	for _, c := range cmds {
		if c.Kind == protocol.CommandRevoke && c.Revoke != nil {
			revoked[c.Revoke.TargetCommandID] = true
		}
	}
	var pending []protocol.UserInput
	for _, c := range cmds {
		if c.Kind == protocol.CommandInput && c.Input != nil &&
			c.Input.Delivery != protocol.DeliverySteer {
			pending = append(pending, *c.Input)
		}
	}
	return pending, revoked, nil
}

// unqueueCmd withdraws one queued message (INC-46, §2 rev1). The local
// precheck is the UX courtesy the design assigns to the client (target
// exists, is an input, not yet consumed); the loop's consume-side guard is
// the actual safety boundary and a late revoke is a no-op there.
func unqueueCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("unqueue", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintln(stderr, `usage: agentrunner unqueue <session-id-or-prefix> <command-id>`)
		return ExitUsage
	}
	dir, err := resolveSessionDir(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	target := rest[1]
	cmds, err := store.ReadCommands(dir, 0)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	var found *protocol.SessionCommand
	for i := range cmds {
		if cmds[i].CommandID == target {
			found = &cmds[i]
			break
		}
	}
	if found == nil {
		fmt.Fprintf(stderr, "agentrunner: no command %s in this session (agentrunner queue lists them)\n", target)
		return ExitUsage
	}
	if found.Kind != protocol.CommandInput {
		fmt.Fprintf(stderr, "agentrunner: only queued conversational inputs are revocable (%s is %s)\n", target, found.Kind)
		return ExitUsage
	}
	if events, rerr := store.ReadEvents(dir); rerr == nil {
		if s, ferr := state.Fold(events); ferr == nil && found.Input != nil &&
			found.Input.DeliverySeq > 0 && found.Input.DeliverySeq <= s.Session.ConsumedInputSeq {
			fmt.Fprintf(stderr, "agentrunner: %s is already being processed — interrupt the session instead\n", target)
			return ExitUsage
		}
	}
	cmd := daemon.Command{Cmd: "unqueue", Session: resolvePrefixLenient(rest[0]),
		TargetCommandID: target, CommandID: event.NewCommandID(),
		Principal: "local-user", Source: "cli", Trust: "local"}
	return oneShot(stderr, cmd, stdout)
}
