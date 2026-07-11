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

	// -h/--help right after a command always means "show help". Positional
	// commands must never swallow it as a session id, path — or worse, a
	// side effect (`init -h` used to write a file named "-h", `trust -h`
	// trusted one). Flag-parsed commands return "" here and keep their
	// flag-package help (QA Round1 F-A07/F-A09).
	if len(args) >= 2 && (args[1] == "-h" || args[1] == "--help") {
		if h := commandHelp(args[0]); h != "" {
			fmt.Fprint(stdout, h)
			return ExitOK
		}
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
	case "artifacts":
		return artifactsCmd(args[1:], stdout, stderr)
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
	case "retry":
		return retryCmd(args[1:], stdout, stderr)
	case "queue":
		return queueCmd(args[1:], stdout, stderr)
	case "unqueue":
		return unqueueCmd(args[1:], stdout, stderr)
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
	case "mode":
		return modeCmd(args[1:], stdout, stderr)
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

// commandHelp is the -h text for commands that take positional arguments
// only (no flag.FlagSet of their own). Flag-parsed commands return "" and
// let the flag package print its richer default. Keep the usage lines in
// sync with each command's own usage error.
func commandHelp(cmd string) string {
	switch cmd {
	case "init":
		return "usage: agentrunner init [--driver] [path]\n\nWrite a commented example agent spec (default: spec.yaml).\n--driver writes an iteration-driver spec instead (default:\ndriver.yaml, for `agentrunner drive`). Refuses to overwrite.\n"
	case "resume":
		return "usage: agentrunner resume <session-id-or-prefix>\n\nResume an interrupted or crashed session in the foreground.\n"
	case "close":
		return "usage: agentrunner close <session-id-or-prefix>\n\nEnd a session gracefully. A later `send` revives it.\n"
	case "interrupt":
		return "usage: agentrunner interrupt <session-id-or-prefix>\n\nInterrupt the session's current turn (a no-op at idle).\nUnlike a queued message, this cancels in-flight work now.\n"
	case "stop":
		return "usage: agentrunner stop <session-id-or-prefix>\n\nStop a hosted run: graceful teardown, no mark; `send` revives it.\n"
	case "compact":
		return "usage: agentrunner compact <session-id-or-prefix> [focus directive]\n\nSummarize the session's context now. The optional focus directive\ntells the summarizer what to preserve.\n"
	case "clear":
		return "usage: agentrunner clear <session-id-or-prefix>\n\nDrop the session's context prefix (the journal keeps everything).\n"
	case "goal":
		return "usage: agentrunner goal <session-id-or-prefix> <attach|update|status|pause|resume|cancel> [flags]\n\nAttach a goal to the session (it keeps working until the goal is\nmet), or manage the one it has. status shows the active goal and\nits check budget. attach/update take the goal text and optional\n--verify \"<cmd>\" / --max-checks N.\n"
	case "agent":
		return "usage: agentrunner agent <session-id-or-prefix> <spec.yaml>\n\nSwitch the session's agent spec; the conversation continues with\nthe new agent from the next message.\n"
	case "kill":
		return "usage: agentrunner kill <session-id-or-prefix> <handle>\n\nCancel one background handle (sub-agent or task); `ps` lists them.\n"
	case "ps":
		return "usage: agentrunner ps <session-id-or-prefix>\n\nList the session's in-flight background work (sub-agents, tasks).\n"
	case "approve":
		return "usage: agentrunner approve <session-id-or-prefix> <approval-id> <approve|deny> [reason] [--always]\n\nAnswer a pending permission ask. attach or inspect shows the id.\n--always (with approve) also saves an exact allow rule to your user\nconfig so the same call no longer asks in future sessions.\n"
	case "barrier":
		return "usage: agentrunner barrier <session-id-or-prefix>\n\nRecord a barrier (a fork point) in the session's journal;\n`fork --list` shows them, `fork` branches from one.\n"
	case "sessions":
		return "usage: agentrunner sessions [list] [--json]\n\nList sessions and their status. JSON includes workspace and title.\n"
	case "trust":
		return "usage: agentrunner trust <dir>\n\nMark a workspace directory as trusted on this machine.\n"
	case "remember":
		return "usage: agentrunner remember <session-id-or-prefix> \"note\"\n\nSave a durable note to the workspace's project CLAUDE.md; future\nsessions in that workspace see it in their prompt prefix, and the\ntarget session honors it from now on.\n"
	case "mode":
		return "usage: agentrunner mode <session-id-or-prefix> <default|acceptEdits>\n\nSwitch the session's permission mode at its next safe boundary\n(journaled as mode_changed). acceptEdits auto-allows edits —\nexecute and protected-path writes still ask. plan and bypass are\nstart-time choices (spec `mode:` or --mode), not runtime targets.\n"
	}
	return ""
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
  retry <session>             re-send the session's last user message as a new turn
  queue <session>             list queued (not yet consumed) messages
  unqueue <session> <cmd-id>  withdraw a queued message before it runs
  attach <session>            replay the whole conversation, then follow live (Ctrl-C detaches;
                              the session keeps running; --replay-only prints history and exits)
  close <session>             end a session gracefully
  interrupt <session>         interrupt the current turn (a no-op at idle; close is separate)
  stop <session>              stop a hosted run: graceful teardown, no mark; send revives it
  compact <session> [focus]   summarize the context now (optional focus directive)
  clear <session>             drop the context prefix (keep the full journal)
  remember <session> "note"   save a durable note to the project CLAUDE.md
                              (injected into future sessions in this workspace)
  mode <session> <mode>       switch permission mode (default|acceptEdits) at the
                              next safe boundary; plan/bypass are start-time only
  goal <session> attach "…"   attach a goal the session keeps working toward
                              (also: goal <session> update|pause|resume|cancel)

Background work (daemon):
  submit <spec.yaml> "task"   hand a one-shot run to the daemon, stream until it ends
  resume <session>            resume an interrupted or crashed session

Observe:
  sessions                    list sessions and their status
  ps <session>                in-flight background work of a session
  inspect <session>           session facts: status, turns, token usage, budget
  artifacts <session> [list|read <stream>[@vN]]   published artifacts: table or raw content
  events <session>            raw journal events (debugging)

Control:
  approve <session> <id> approve|deny   answer a pending permission ask
  kill <session> <handle>               cancel one background handle
  agent <session> <spec.yaml>           switch the session's agent (决策 #32)
  fork <session> <barrier>    branch a session at a barrier into a new one (--list shows barriers)
  trust <dir>                 mark a workspace as trusted

Other: barrier, version — see each command's -h.
Developer (run the product's own test suites; not for everyday use): accept, record-fixture.

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
