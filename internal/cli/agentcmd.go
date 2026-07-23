package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/modelconfig"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// agentCmd switches a session's agent spec (决策 #32): `agentrunner agent
// <session-id-or-prefix> <spec.yaml>`. The user's switch needs NO
// confirmation — the command itself is the intent. Flow: validate the new
// spec, ask a running daemon to release the hosted loop (plain teardown,
// nothing journaled), append the SpecChanged fact with the re-frozen
// prefix blocks, and let the next send revive the session under the new
// spec.
func agentCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	modelFlags := addModelFlags(fs)
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: agentrunner agent [--model <provider>/<id>] [--effort <level>] <session-id-or-prefix> <agent-name|spec.yaml>")
		return ExitUsage
	}
	dir, err := resolveSessionDir(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	sessionID := filepath.Base(dir)
	started, err := readSessionStarted(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	specJSON := started.Spec
	if changed, cerr := readLatestSpecChange(dir); cerr == nil && changed != nil {
		specJSON = changed.Spec
	}
	var current agent.AgentSpec
	if err := json.Unmarshal(specJSON, &current); err != nil {
		fmt.Fprintf(stderr, "agentrunner: journaled spec: %v\n", err)
		return ExitRun
	}
	model := current.Model
	if *modelFlags.ref != "" || *modelFlags.effort != "" {
		base := modelconfig.FromSpec(current.Model)
		selection, err := modelconfig.WithExplicit(base, *modelFlags.ref, *modelFlags.effort)
		if err != nil {
			fmt.Fprintf(stderr, "agentrunner: %v\n", err)
			return ExitUsage
		}
		if !knownProviderName(selection.Provider) {
			fmt.Fprintf(stderr, "agentrunner: unknown provider %q\n", selection.Provider)
			return ExitUsage
		}
		model, err = selection.Resolve()
		if err != nil {
			fmt.Fprintf(stderr, "agentrunner: %v\n", err)
			return ExitUsage
		}
	}
	spec, specRef, err := resolveAgent(fs.Arg(1), model)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}

	// A hosted loop holds the journal lock: ask the daemon to release it.
	// No daemon (or not hosted) is fine — the switch is a journal append.
	if sock, serr := socketPath(); serr == nil {
		_ = daemon.Dial(sock, daemon.Command{Cmd: "agent", Session: sessionID},
			func(protocol.Event) {})
	}

	// The new generation's frozen blocks and effective permission layers,
	// rendered exactly like a session start would.
	ws, err := workspace.New(started.WorkspaceRoot)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	pipe, _, err := buildPipeline(ws, spec.Permissions, spec.Mode, spec.Budget.MaxTotalTokens, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	changed, err := agent.RenderSpecChange(spec, specRef, started.WorkspaceRoot,
		time.Now(), siblingSpecResolver(specRef, spec.Model, false), pipe)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}

	es, err := store.OpenEventStore(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v (a hosted loop may still hold the session — is the daemon reachable?)\n", err)
		return ExitRun
	}
	defer func() { _ = es.Close() }()
	env, err := event.New(event.TypeSpecChanged, changed)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	if _, err := es.Append(env); err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	fmt.Fprintf(stdout, "agent switched to %s\n", spec.Name)
	fmt.Fprintf(stderr, "(takes effect on the next send: agentrunner send %s \"...\")\n", sessionID)
	return ExitOK
}

// readLatestSpecChange returns the most recent SpecChanged fact, if any —
// the resume/revival assembly must honor the CURRENT agent (决策 #32).
func readLatestSpecChange(dir string) (*event.SpecChanged, error) {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return nil, err
	}
	var latest *event.SpecChanged
	for _, e := range events {
		if e.Type != event.TypeSpecChanged {
			continue
		}
		if decoded, derr := event.DecodePayload(e); derr == nil {
			latest = decoded.(*event.SpecChanged)
		}
	}
	return latest, nil
}
