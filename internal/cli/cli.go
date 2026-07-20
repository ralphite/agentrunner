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
		// `help <command>` shows that command's usage instead of silently
		// dumping the global help and ignoring the argument (QA Wave1
		// cli-life-05). A command with no dedicated blurb falls back to global.
		if args[0] == "help" && len(args) > 1 {
			if h := commandHelp(args[1]); h != "" {
				fmt.Fprint(stdout, h)
				return ExitOK
			}
		}
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
	case "diff":
		return diffCmd(args[1:], stdout, stderr)
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
	case "hook":
		return hookCmd(args[1:], stdout, stderr)
	case "answer":
		return answerCmd(args[1:], stdout, stderr)
	case "close":
		// Undocumented webui transport (INC-83): the lifecycle verb is gone
		// from the product face; the route dies with the webui migration.
		return closeCmd(args[1:], stdout, stderr)
	case "interrupt":
		return interruptCmd(args[1:], stdout, stderr)
	case "stop":
		// Undocumented transport for cancelling a running series (INC-83):
		// the domain terminal is SeriesEnded{cancelled}, not a session mark.
		return stopCmd(args[1:], stdout, stderr)
	case "compact":
		return compactCmd(args[1:], stdout, stderr)
	case "clear":
		return clearCmd(args[1:], stdout, stderr)
	case "remember":
		return rememberCmd(args[1:], stdout, stderr)
	case "title":
		return titleCmd(args[1:], stdout, stderr)
	case "promote":
		return promoteCmd(args[1:], stdout, stderr)
	case "mode":
		return modeCmd(args[1:], stdout, stderr)
	case "goal":
		return goalCmd(args[1:], stdout, stderr)
	case "schedule":
		return scheduleCmd(args[1:], stdout, stderr)
	case "agent":
		return agentCmd(args[1:], stdout, stderr)
	case "kill":
		// Undocumented webui transport (INC-83): kill is the MODEL's tool for
		// its own background work; the user face is gone.
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
	case "doctor":
		return doctorCmd(args[1:], stdout, stderr)
	case "dictate":
		return dictateCmd(args[1:], stdout, stderr)
	case "optimize":
		return optimizeCmd(args[1:], stdout, stderr)
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
		return "usage: agentrunner init [path]\n\nWrite a commented example agent spec (default: spec.yaml).\nRefuses to overwrite. Scheduled work (goals, repeating runs,\nbest-of-N) attaches to a session: see `agentrunner goal` and\n`agentrunner schedule`, or the web UI's Scheduled page.\n"
	case "resume":
		return "usage: agentrunner resume <session-id-or-prefix>\n\nResume an interrupted or crashed session in the foreground.\n"
	case "interrupt":
		return "usage: agentrunner interrupt <session-id-or-prefix>\n\nStop what the session is doing right now (a no-op at idle).\nNothing is ever \"closed\": the conversation stays continuable —\njust send the next message when you want more.\n"
	case "close":
		return "usage: agentrunner close <session-id-or-prefix>\n\n(internal web-UI transport — not part of the command set)\n"
	case "stop":
		return "usage: agentrunner stop <session-id-or-prefix>\n\n(internal transport: cancels a running scheduled series —\nthe series records its own cancelled terminal)\n"
	case "kill":
		return "usage: agentrunner kill <session-id-or-prefix> <handle>\n\n(internal web-UI transport — the model manages its own\nbackground work)\n"
	case "compact":
		return "usage: agentrunner compact <session-id-or-prefix> [focus directive]\n\nSummarize the session's context now. The optional focus directive\ntells the summarizer what to preserve.\n"
	case "clear":
		return "usage: agentrunner clear <session-id-or-prefix>\n\nDrop the session's context prefix (the journal keeps everything).\n"
	case "title":
		return "usage: agentrunner title <session-id-or-prefix> <new title>\n\nRename the session. The title is journaled and shown everywhere.\n"
	case "promote":
		return "usage: agentrunner promote <best-of-N-session-id-or-prefix>\n\nApply the finished round's WINNER attempt onto the project workspace\n(clean-or-nothing; changes land unstaged for your review).\n"
	case "goal":
		return "usage: agentrunner goal <session-id-or-prefix> <attach|update|status|pause|resume|cancel> [flags]\n\nAttach a goal to the session (it keeps working until the goal is\nmet), or manage the one it has. status shows the active goal and\nits check budget. attach/update take the goal text and optional\n--verify \"<cmd>\" / --max-checks N.\n"
	case "schedule":
		return "usage: agentrunner schedule <session-id-or-prefix> <attach|status|pause|resume|cancel> [flags]\n\nAttach a recurring self-wake cadence to the session: at each tick it\nwakes, runs one turn on the standing prompt (context continues), and\nre-arms — even across daemon restarts. attach takes the prompt plus\n--every <duration> or --cron \"<5-field>\" and optional --max-wakes N.\npause stops wakes (no catch-up); resume re-anchors the cadence.\n"
	case "agent":
		return "usage: agentrunner agent <session-id-or-prefix> <spec.yaml>\n\nSwitch the session's agent spec; the conversation continues with\nthe new agent from the next message.\n"
	case "ps":
		return "usage: agentrunner ps <session-id-or-prefix>\n\nList the session's in-flight background work (sub-agents and tools).\n"
	case "approve":
		return "usage: agentrunner approve <session-id-or-prefix> <approval-id> <approve|deny> [reason] [--always]\n\nAnswer a pending permission ask. attach or inspect shows the id.\n--always (with approve) also saves an exact allow rule to your user\nconfig so the same call no longer asks in future sessions.\n"
	case "barrier":
		return "usage: agentrunner barrier <session-id-or-prefix>\n\nRecord a barrier (a fork point) in the session's journal;\n`fork --list` shows them, `fork` branches from one.\n"
	case "sessions":
		return "usage: agentrunner sessions [list] [--json] [--limit N] [--offset N]\n\nList sessions and their status. JSON includes workspace and title.\n"
	case "trust":
		return "usage: agentrunner trust <dir>\n\nMark a workspace directory as trusted on this machine.\n"
	case "doctor":
		return "usage: agentrunner doctor\n\nPreflight this environment: probe the OS sandbox backend (bubblewrap\non Linux, Seatbelt on macOS) that bash and command tools require\n(fail-closed). Prints the fix when a probe fails; exit 0 = ready.\n"
	case "remember":
		return "usage: agentrunner remember <session-id-or-prefix> \"note\"\n\nSave a durable note to the workspace's project CLAUDE.md; future\nsessions in that workspace see it in their prompt prefix, and the\ntarget session honors it from now on.\n"
	case "hook":
		return "usage: agentrunner hook create <session> [--name ci] | list [<session>] | revoke <hook-id>\n\nManage webhook ingress capabilities: create prints the hook URL and\nits bearer token ONCE (only a hash is stored). POST /hooks/<id> on\nthe daemon's --http address delivers an external event into the\nsession as untrusted machine input.\n"
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
  agentrunner run spec.yaml "your prompt"   one-shot run, output streams here
  agentrunner daemon --detach             start the runtime that hosts conversations
  agentrunner new spec.yaml "hello"       start a conversation, print the reply
  agentrunner send <session> "and this?"  continue it (unique id prefix is enough)

One-shot runs (no daemon needed):
  run <spec.yaml> "prompt"      run to completion in the foreground
  dictate <audio-file>        transcribe an audio recording to text (prints the transcript)
  optimize "draft"            rewrite a draft prompt into a clearer instruction

Conversations (need the daemon):
  daemon [--detach]           start the resident runtime (--detach backgrounds it, surviving this terminal)
  new <spec.yaml> "msg"       start a session, print the reply, leave it running
  send <session> "msg"        send a message and print the reply (--image attaches files)
  retry <session>             re-send the session's last user message as a new turn
  queue <session>             list queued (not yet consumed) messages
  unqueue <session> <cmd-id>  withdraw a queued message before it runs
  answer <session> <q>:<n>... answer a structured question (--skip to decline)
  attach <session>            replay the whole conversation, then follow live (Ctrl-C detaches;
                              the session keeps running; --replay-only prints history and exits)
  interrupt <session>         stop what it's doing now (a no-op at idle)
  compact <session> [focus]   summarize the context now (optional focus directive)
  clear <session>             drop the context prefix (keep the full journal)
  remember <session> "note"   save a durable note to the project CLAUDE.md
  title <session> "name"      rename the session (journaled, shown everywhere)
  promote <session>           apply a finished best-of-N round's winner to the project
                              (injected into future sessions in this workspace)
  mode <session> <mode>       switch permission mode (default|acceptEdits) at the
                              next safe boundary; plan/bypass are start-time only
  goal <session> attach "…"   attach a goal the session keeps working toward
                              (also: goal <session> update|pause|resume|cancel)
  schedule <session> attach --every 30m "…"   the session wakes itself on a
                              cadence and runs the standing prompt each round
                              (also: --cron "…"; schedule status|pause|resume|cancel)
  hook create <session>       mint a webhook URL+token for external events
                              (daemon --http <addr> serves POST /hooks/<id>;
                              also: hook list, hook revoke <id>)

Background work (daemon):
  submit <spec.yaml> "prompt"   hand a one-shot run to the daemon, stream until it ends
  resume <session>            resume an interrupted or crashed session

Observe:
  doctor                      preflight this machine: is the OS sandbox that
                              bash/command tools require actually available?
  sessions                    list sessions and their status
  ps <session>                in-flight background work of a session
  inspect <session>           session facts: status, turns, token usage, budget
  diff <session>              workspace changes since the latest human turn began
  artifacts <session> [list|read <stream>[@vN]]   published artifacts: table or raw content
  events <session>            raw journal events (debugging)

Control:
  approve <session> <id> approve|deny   answer a pending permission ask
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
