// Package cli implements the agentrunner command-line interface.
package cli

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
)

// Exit codes per PLAN.md S1 执行包.
const (
	ExitOK    = 0 // run completed
	ExitRun   = 1 // run failed (provider/tool fatal)
	ExitUsage = 2 // usage or spec error
)

// Run executes the CLI and returns the process exit code.
func Run(args []string, version string, stdout, stderr io.Writer) int {
	setupLogging(stderr)

	if len(args) == 0 {
		fmt.Fprint(stderr, usage())
		return ExitUsage
	}

	switch args[0] {
	case "--version", "version":
		fmt.Fprintf(stdout, "agentrunner %s (%s)\n", version, runtime.Version())
		return ExitOK
	case "--help", "-h", "help":
		fmt.Fprint(stdout, helpText())
		return ExitOK
	case "init":
		return initCmd(args[1:], stdout, stderr)
	case "run":
		return runCmd(args[1:], false, version, stdout, stderr)
	case "drive":
		return driveCmd(args[1:], version, stdout, stderr)
	case "record-fixture":
		return runCmd(args[1:], true, version, stdout, stderr)
	case "accept":
		return acceptCmd(args[1:], stdout, stderr)
	case "events":
		return eventsCmd(args[1:], stdout, stderr)
	case "inspect":
		return inspectCmd(args[1:], stdout, stderr)
	case "resume":
		return resumeCmd(args[1:], version, stdout, stderr)
	case "daemon":
		return daemonCmd(args[1:], version, stdout, stderr)
	case "attach":
		return attachCmd(args[1:], stdout, stderr)
	case "submit":
		return submitCmd(args[1:], stdout, stderr)
	case "new":
		return newCmd(args[1:], stdout, stderr)
	case "send":
		return sendCmd(args[1:], stdout, stderr)
	case "close":
		return closeCmd(args[1:], stdout, stderr)
	case "interrupt":
		return interruptCmd(args[1:], stdout, stderr)
	case "stop":
		return stopCmd(args[1:], stdout, stderr)
	case "compact":
		return compactCmd(args[1:], stdout, stderr)
	case "clear":
		return clearCmd(args[1:], stdout, stderr)
	case "remember":
		return rememberCmd(args[1:], stdout, stderr)
	case "goal":
		return goalCmd(args[1:], stdout, stderr)
	case "agent":
		return agentCmd(args[1:], stdout, stderr)
	case "kill":
		return killCmd(args[1:], stdout, stderr)
	case "ps":
		return psCmd(args[1:], stdout, stderr)
	case "approve":
		return approveCmd(args[1:], stdout, stderr)
	case "fork":
		return forkCmd(args[1:], stdout, stderr)
	case "barrier":
		return barrierCmd(args[1:], stdout, stderr)
	case "sessions":
		return sessionsCmd(args[1:], stdout, stderr)
	case "trust":
		return trustCmd(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "agentrunner: unknown command %q\n%s", args[0], usage())
		return ExitUsage
	}
}

func usage() string {
	return "usage: agentrunner <command> [flags] [args]\nrun `agentrunner help` for the command list, `agentrunner init` for an example spec\n"
}

// helpText is the top-level help (INC-2 BB-me-1/2): grouped commands with
// one-line explanations and a quick start, so a first-time user can go from
// zero to a visible reply without reading source or docs.
func helpText() string {
	return `agentrunner — declarative LLM agents with durable, resumable sessions

Quick start:
  agentrunner init                        write a commented example spec.yaml
  agentrunner run spec.yaml "your task"   one-shot run, output streams here
  agentrunner daemon --detach             start the runtime that hosts conversations
  agentrunner new spec.yaml "hello"       start a conversation, print the reply
  agentrunner send <session> "and this?"  continue it (unique id prefix is enough)

One-shot runs (no daemon needed):
  run <spec.yaml> "task"      run to completion in the foreground
  drive <driver.yaml>         run an iteration-driver series (plan/verify loop)

Conversations (need the daemon):
  daemon [--detach]           start the resident runtime (--detach backgrounds it, surviving this terminal)
  new <spec.yaml> "msg"       start a session, print the reply, leave it running
  send <session> "msg"        send a message and print the reply (--image attaches files)
  attach <session>            replay the whole conversation, then follow live (Ctrl-C detaches;
                              the session keeps running; --replay-only prints history and exits)
  close <session>             end a session gracefully
  interrupt <session>         interrupt the current turn (a no-op at idle; close is separate)
  stop <session>              stop a hosted run: graceful teardown, no mark; send revives it
  compact <session> [focus]   summarize the context now (optional focus directive)
  clear <session>             drop the context prefix (keep the full journal)

Background work (daemon):
  submit <spec.yaml> "task"   hand a one-shot run to the daemon, stream until it ends
  resume <session>            resume an interrupted or crashed session

Observe:
  sessions                    list sessions and their status
  ps <session>                in-flight background work of a session
  inspect <session>           session facts: status, turns, token usage, budget
  events <session>            raw journal events (debugging)

Control:
  approve <session> <id> approve|deny   answer a pending permission ask
  kill <session> <handle>               cancel one background handle
  agent <session> <spec.yaml>           switch the session's agent (决策 #32)
  fork <session> <barrier>    branch a session at a barrier into a new one (--list shows barriers)
  trust <dir>                 mark a workspace as trusted

Other: barrier, accept, record-fixture, version — see each command's -h.

Sessions are addressed by any unique prefix of their id.
Spec format: agentrunner init writes a commented example.
`
}

// setupLogging configures the process-wide slog default. Logs go to stderr
// (never stdout, which belongs to run output); AGENTRUNNER_DEBUG=1 raises
// the level to Debug.
func setupLogging(stderr io.Writer) {
	level := slog.LevelInfo
	if os.Getenv("AGENTRUNNER_DEBUG") == "1" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level})))
}
