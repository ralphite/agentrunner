package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/structured"
)

// newCmd starts a daemon-hosted CONVERSATIONAL session (v2 M1.2): `agentrunner
// new <spec.yaml> "opening message" [--workspace dir]`. By default it follows
// the opening turn and RENDERS THE REPLY (INC-2 BB-me-4: asking for a haiku
// must show the haiku), detaching at the turn's idle — the session keeps
// running and `send`/`attach`/`close` address it by the printed id.
// --detach restores the fire-and-forget form (prints just the id).
func newCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceDir := fs.String("workspace", ".", "workspace root")
	mode := fs.String("mode", "", "run mode: default|plan|acceptEdits")
	detach := fs.Bool("detach", false, "print the session id and exit without waiting for the reply")
	jsonSchema := fs.String("json-schema", "", "path to a JSON Schema; the reply must be JSON matching it (validated, retried) — INC-26 #91")
	jsonSchemaRetries := fs.Int("json-schema-max-retries", 2, "extra re-prompts to coax a conforming reply when --json-schema is set")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	rest, terr := completeTextArg(fs.Args(), 2)
	if terr != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", terr)
		return ExitUsage
	}
	if len(rest) != 2 {
		fmt.Fprintln(stderr, `usage: agentrunner new [flags] <spec.yaml> "opening message"  (message may be piped via stdin)`)
		return ExitUsage
	}
	if strings.TrimSpace(rest[1]) == "" {
		fmt.Fprintln(stderr, "agentrunner: new needs a non-empty opening message")
		return ExitUsage
	}
	specPath, err := filepath.Abs(rest[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	wsAbs, err := filepath.Abs(*workspaceDir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	// Validate spec and workspace before dialing, exactly like a foreground
	// run: with --detach the client leaves at RunStart, so a daemon-side
	// early failure would otherwise mint a session id for a run that never
	// lands on disk — a ghost session (QA Round1 F-A02).
	loadedSpec, err := agent.LoadSpec(specPath)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	// Reject an unknown provider up front, exactly like `ar run` does: otherwise
	// the daemon mints a session id, prints it, then fails at provider
	// construction — a phantom session plus a misleading exit 0 (QA Wave1
	// dave-01). Validating here keeps the spec-error contract (exit 2, no ghost).
	if !knownProviderName(loadedSpec.Model.Provider) {
		fmt.Fprintf(stderr, "agentrunner: unknown provider %q (available: gemini, anthropic, scripted)\n", loadedSpec.Model.Provider)
		return ExitUsage
	}
	if st, err := os.Stat(wsAbs); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "agentrunner: workspace root %s is not a directory\n", wsAbs)
		return ExitUsage
	}
	// --json-schema needs the reply, so it cannot be fire-and-forget; compile
	// the schema BEFORE dialing so a bad schema fails fast (no ghost session).
	var validator *structured.Validator
	if *jsonSchema != "" {
		if *detach {
			fmt.Fprintln(stderr, "agentrunner: --json-schema cannot be combined with --detach (it must wait for the reply)")
			return ExitUsage
		}
		if *jsonSchemaRetries < 0 {
			fmt.Fprintln(stderr, "agentrunner: --json-schema-max-retries must be >= 0")
			return ExitUsage
		}
		raw, rerr := os.ReadFile(*jsonSchema)
		if rerr != nil {
			fmt.Fprintf(stderr, "agentrunner: read --json-schema: %v\n", rerr)
			return ExitUsage
		}
		validator, err = structured.Compile(raw)
		if err != nil {
			fmt.Fprintf(stderr, "agentrunner: invalid --json-schema: %v\n", err)
			return ExitUsage
		}
	}
	sock, err := socketPath()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	cmd := daemon.Command{
		Cmd: "run", SpecPath: specPath, Prompt: rest[1],
		Workspace: wsAbs, Mode: *mode,
	}
	if *detach {
		// Detach after RunStart: read just the id and leave; the session
		// runs on under the daemon.
		sid, derr := dialUntilStart(sock, cmd)
		if derr != nil {
			daemonDialErr(stderr, derr)
			return ExitRun
		}
		if sid == "" {
			fmt.Fprintln(stderr, "agentrunner: session did not start")
			return ExitRun
		}
		fmt.Fprintf(stdout, "%s\n", sid)
		fmt.Fprintf(stderr, "session %s (send: agentrunner send %s \"...\")\n", sid, sid)
		return ExitOK
	}
	if validator != nil {
		return runStructured(sock, cmd, validator, *jsonSchemaRetries, stdout, stderr)
	}
	return followTurn(sock, cmd, "", stdout, stderr)
}

// followTurn issues cmd and renders the session's events until the turn goes
// idle, then detaches and prints the continue hint (INC-2 BB-me-4/5/6: the
// conversational path shows its output exactly like `run` does, and points
// at `send`/`attach` for what comes next). ackText, when non-empty, names the
// daemon's request/reply ack line to swallow (send's "delivered").
func followTurn(sock string, cmd daemon.Command, ackText string, stdout, stderr io.Writer) int {
	render := newTextRenderer(stdout)
	sid := cmd.Session // send knows it already; new learns it from SessionStart
	var sawIdle, sawErr, acked, announced bool
	err := daemon.DialUntil(sock, cmd, func(e protocol.Event) bool {
		if e.Session != "" && sid == "" {
			sid = e.Session
			render.anchor(sid)
		}
		// A tree MEMBER's live event (INC-12.6) must not steer this follow:
		// its Idle/RunEnd are not the anchored turn's. The renderer folds it.
		if sid != "" && e.Session != "" && e.Session != sid {
			render.Emit(e)
			return true
		}
		switch e.Kind {
		case protocol.KindSessionStart:
			// Printed once: the daemon's ack and the loop's own emit both
			// carry this kind.
			if !announced {
				fmt.Fprintf(stderr, "session %s\n", e.Session)
				announced = true
			}
			return true
		case protocol.KindMessage:
			if ackText != "" && !acked && e.Text == ackText {
				acked = true // the request/reply ack, not the model speaking
				return true
			}
		case protocol.KindIdle:
			sawIdle = true
			return false // turn done: detach, the session keeps running
		case protocol.KindError:
			sawErr = true
		case protocol.KindRunEnd:
			// The session ended instead of idling (failure or close).
			sawErr = e.Reason != "completed" && e.Reason != "closed"
			render.Emit(e)
			return false
		}
		render.Emit(e)
		return true
	})
	if err != nil {
		daemonDialErr(stderr, err)
		return ExitRun
	}
	switch {
	case sawErr:
		return ExitRun
	case sawIdle:
		fmt.Fprintf(stderr, "(session %s is waiting — continue: agentrunner send %s \"...\"  history: agentrunner attach %s)\n",
			sid, sid, sid)
		return ExitOK
	case acked:
		// The daemon acked but closed without streaming (an older daemon
		// without follow): the message IS delivered, the reply just is not
		// on this connection.
		fmt.Fprintf(stderr, "delivered; this daemon does not stream replies — read it with: agentrunner attach %s\n", sid)
		return ExitOK
	default:
		fmt.Fprintln(stderr, "agentrunner: stream ended before the reply")
		return ExitRun
	}
}

// sendCmd delivers a user message to a live conversational session (v2 M1.2):
// `agentrunner send [--image f.png]... <session-id-or-prefix> "message"`.
// By default it follows the turn and RENDERS THE REPLY (INC-2 BB-me-4),
// detaching at idle; --detach restores the ack-only form ("delivered").
// Attached images ride the command line base64 (v2 M4.1); the agent stores
// them in the session CAS before journaling the input.
func sendCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var imagePaths repeatedFlag
	fs.Var(&imagePaths, "image", "attach an image file (repeatable)")
	var filePaths repeatedFlag
	fs.Var(&filePaths, "file", "attach a file of any type — PDF, etc. (repeatable, INC-9)")
	detach := fs.Bool("detach", false, "deliver the message and exit without waiting for the reply")
	steer := fs.Bool("steer", false, "steer: deliver into the CURRENT turn at its next safe boundary (default: queue to the next turn) — INC-43")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	rest, terr := completeTextArg(fs.Args(), 2)
	if terr != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", terr)
		return ExitUsage
	}
	if len(rest) != 2 {
		fmt.Fprintln(stderr, `usage: agentrunner send [flags] <session-id-or-prefix> "message"  (message may be piped: git diff | agentrunner send <sid> -)`)
		return ExitUsage
	}
	images, err := loadImageAttachments(imagePaths)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	files, err := loadFileAttachments(filePaths)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	if strings.TrimSpace(rest[1]) == "" && len(images) == 0 && len(files) == 0 {
		// Reject a whitespace-only message the same way `new` rejects an empty
		// opening message (QA Wave1 alice-01): a blank send would otherwise burn
		// a real turn on empty content. Attachments make a blank caption fine.
		fmt.Fprintln(stderr, "agentrunner: send needs a non-empty message (or an --image/--file attachment)")
		return ExitUsage
	}
	addr, aerr := resolveAddress(rest[0])
	if aerr != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", aerr)
		return ExitUsage
	}
	cmd := daemon.Command{Cmd: "send", Session: addr,
		Text: rest[1], Images: images, Files: files, CommandID: event.NewCommandID(),
		Principal: "local-user", Source: "cli", Trust: "local"}
	if *steer {
		cmd.Delivery = protocol.DeliverySteer
	}
	if *detach {
		return oneShot(stderr, cmd, stdout)
	}
	sock, serr := socketPath()
	if serr != nil {
		fmt.Fprintln(stderr, serr)
		return ExitRun
	}
	cmd.Follow = true
	return followTurn(sock, cmd, "delivered", stdout, stderr)
}

// repeatedFlag collects a repeatable string flag.
type repeatedFlag []string

func (r *repeatedFlag) String() string { return strings.Join(*r, ",") }
func (r *repeatedFlag) Set(v string) error {
	*r = append(*r, v)
	return nil
}

// maxAttachmentBytes caps a single --image/--file attachment. Base64 inflates
// bytes ~4/3 and every byte is billed as input tokens, so a large file
// silently balloons cost and can blow the provider's request limit (QA Wave2
// frank-02). 5 MiB is comfortably above real screenshots/PDFs while refusing
// an accidental multi-hundred-MB attach up front with a clear message.
const maxAttachmentBytes = 5 << 20

// readAttachment reads one attachment file, enforcing the size cap.
func readAttachment(path string) ([]byte, error) {
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxAttachmentBytes {
		return nil, fmt.Errorf("%s is %d bytes, over the %d-byte (%d MiB) attachment limit — shrink or split it",
			path, fi.Size(), int64(maxAttachmentBytes), maxAttachmentBytes>>20)
	}
	return os.ReadFile(path)
}

// loadImageAttachments reads each image file and sniffs its media type.
func loadImageAttachments(paths []string) ([]protocol.ImageAttachment, error) {
	var out []protocol.ImageAttachment
	for _, path := range paths {
		data, err := readAttachment(path)
		if err != nil {
			return nil, err
		}
		mt := http.DetectContentType(data)
		if !strings.HasPrefix(mt, "image/") {
			return nil, fmt.Errorf("%s: not an image (detected %s)", path, mt)
		}
		out = append(out, protocol.ImageAttachment{MediaType: mt, Data: data})
	}
	return out, nil
}

// loadFileAttachments reads each attached file and sniffs its media type
// (INC-9). Unlike --image it accepts any type; the sniffed MIME drives the
// provider mapping (Gemini inline_data / Anthropic document block).
// http.DetectContentType recognises the %PDF- magic as application/pdf.
func loadFileAttachments(paths []string) ([]protocol.FileAttachment, error) {
	var out []protocol.FileAttachment
	for _, path := range paths {
		data, err := readAttachment(path)
		if err != nil {
			return nil, err
		}
		out = append(out, protocol.FileAttachment{MediaType: http.DetectContentType(data), Data: data})
	}
	return out, nil
}

// killCmd cancels one running child/background work by handle (v2 M3.2):
// `agentrunner kill <session-id-or-prefix> <handle>`.
func killCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "usage: agentrunner kill <session-id-or-prefix> <handle>")
		return ExitUsage
	}
	code := oneShot(stderr, daemon.Command{Cmd: "kill", Session: resolvePrefixLenient(args[0]), Handle: args[1]}, stdout)
	if code != ExitOK {
		stuckHint(stderr, args[0])
	}
	return code
}

// interruptCmd delivers an out-of-band interrupt to a live session (v2
// M2.3): `agentrunner interrupt <session-id-or-prefix>` — steers a running
// turn; at idle it is a no-op (裁决 #11: interrupt never ends a session,
// close is its own command). It is NOT a message.
func interruptCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner interrupt <session-id-or-prefix>")
		return ExitUsage
	}
	code := oneShot(stderr, daemon.Command{Cmd: "interrupt", Session: resolvePrefixLenient(args[0])}, stdout)
	if code != ExitOK {
		stuckHint(stderr, args[0])
	}
	return code
}

// stopCmd remotely tears down a hosted run (G12): graceful cancel with a
// restartable stopped mark — `send` revives it.
// Distinct from interrupt (turn-level) and close (a mark).
func stopCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner stop <session-id-or-prefix>")
		return ExitUsage
	}
	addr, aerr := resolveAddress(args[0])
	if aerr != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", aerr)
		return ExitUsage
	}
	code := oneShot(stderr, daemon.Command{Cmd: "stop", Session: addr}, stdout)
	if code != ExitOK {
		stuckHint(stderr, args[0])
	}
	return code
}

// compactCmd manually summarizes a session's context now (G7):
// `agentrunner compact <session> [focus directive...]`. The rest of the args
// join into an optional focus for the summarizer.
func compactCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: agentrunner compact <session-id-or-prefix> [focus directive]")
		return ExitUsage
	}
	cmd := daemon.Command{Cmd: "compact", Session: resolvePrefixLenient(args[0])}
	if len(args) > 1 {
		cmd.Directive = strings.Join(args[1:], " ")
	}
	code := oneShot(stderr, cmd, stdout)
	if code != ExitOK {
		stuckHint(stderr, args[0])
	}
	return code
}

// clearCmd drops a session's context prefix (G7): `agentrunner clear <session>`.
// The full journal is kept; only the assembled view resets.
func clearCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner clear <session-id-or-prefix>")
		return ExitUsage
	}
	code := oneShot(stderr, daemon.Command{Cmd: "clear", Session: resolvePrefixLenient(args[0])}, stdout)
	if code != ExitOK {
		stuckHint(stderr, args[0])
	}
	return code
}

// rememberCmd writes a note to the workspace-root CLAUDE.md (G9, INC-14):
// `agentrunner remember <session> <text…>`. The note persists to project
// memory (next session picks it up in the frozen prefix) and enters the
// current conversation as a program-source message so this run honors it too.
func rememberCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: agentrunner remember <session-id-or-prefix> <text to remember>")
		return ExitUsage
	}
	cmd := daemon.Command{Cmd: "remember", Session: resolvePrefixLenient(args[0]),
		Directive: strings.Join(args[1:], " ")}
	code := oneShot(stderr, cmd, stdout)
	if code != ExitOK {
		stuckHint(stderr, args[0])
	}
	return code
}

// modeCmd switches a session's permission mode at its next safe boundary
// (INC-42, G29): `agentrunner mode <session> <default|acceptEdits>`. Runtime
// switching covers the user-sovereignty pair only — plan exits via the
// exit_plan_mode approval flow and bypass stays a process-start choice — so
// anything else is rejected here before a command is even sent.
func modeCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || (args[1] != "default" && args[1] != "acceptEdits") {
		fmt.Fprintln(stderr, "usage: agentrunner mode <session-id-or-prefix> <default|acceptEdits>\n(plan and bypass are start-time choices: spec `mode:` or --mode)")
		return ExitUsage
	}
	cmd := daemon.Command{Cmd: "mode", Session: resolvePrefixLenient(args[0]), Directive: args[1]}
	code := oneShot(stderr, cmd, stdout)
	if code != ExitOK {
		stuckHint(stderr, args[0])
	}
	return code
}

// goalCmd drives an in-session goal (INC-D1, G23/UJ-22):
//
//	agentrunner goal <session> attach "<goal>" --verify "<cmd>" [--verify …] [--max-checks N]
//	agentrunner goal <session> update ["<goal>"] [--verify …] [--max-checks N]
//	agentrunner goal <session> pause|resume|cancel
//
// The goal hangs on the conversational session and its context continues across
// checks; a verifier (command; exit 0 = pass) runs at each quiescence boundary.
// One goal per session (id "goal"); attach replaces any existing one.
func goalCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: agentrunner goal <session-id-or-prefix> <attach|update|status|pause|resume|cancel> [flags]")
		return ExitUsage
	}
	session := resolvePrefixLenient(args[0])
	sub, rest := args[1], args[2:]
	switch sub {
	case "status":
		// Reads the journal directly — no daemon round-trip, works on idle
		// and stopped sessions alike (QA Round4 F-I3/F-J2: goal state had
		// no first-class query; users grepped `events`).
		dir, err := resolveSessionDir(session)
		if err != nil {
			fmt.Fprintf(stderr, "agentrunner: %v\n", err)
			return ExitUsage
		}
		events, err := store.ReadEvents(dir)
		if err != nil {
			fmt.Fprintf(stderr, "agentrunner: %v\n", err)
			return ExitRun
		}
		fold, err := state.Fold(events)
		if err != nil {
			fmt.Fprintf(stderr, "agentrunner: fold: %v\n", err)
			return ExitRun
		}
		if fold.Goal == nil {
			fmt.Fprintln(stdout, "no active goal")
			return ExitOK
		}
		g := fold.Goal
		fmt.Fprintf(stdout, "goal      %s\n", g.Goal)
		max := g.Budget.MaxChecks
		if max > 0 {
			fmt.Fprintf(stdout, "checks    %d/%d\n", g.Checks, max)
		} else {
			fmt.Fprintf(stdout, "checks    %d\n", g.Checks)
		}
		// Mirror the checkpoint's three-way discriminator (INC-48): command
		// outranks llm_judge outranks self-cert.
		var nCmd, nLLM int
		for _, v := range g.Verifiers {
			switch {
			case v.Kind == "command" && v.Command != "":
				nCmd++
			case v.Kind == "llm_judge" && v.Rubric != "":
				nLLM++
			}
		}
		kind := "self-certified (the model claims completion via goal_complete)"
		switch {
		case nCmd > 0:
			kind = "command verifiers: " + strconv.Itoa(nCmd)
		case nLLM > 0:
			kind = "llm_judge (claim-gated: an independent LLM adjudicates goal_complete claims)"
		}
		fmt.Fprintf(stdout, "judge     %s\n", kind)
		if g.Claimed {
			fmt.Fprintln(stdout, "claim     pending adjudication at the next boundary")
		}
		if g.Paused {
			fmt.Fprintln(stdout, "paused    yes (goal resume to continue)")
		}
		return ExitOK
	case "pause", "resume", "cancel":
		return oneShot(stderr, daemon.Command{Cmd: "goal-" + sub, Session: session}, stdout)
	case "attach", "update":
		fs := flag.NewFlagSet("goal "+sub, flag.ContinueOnError)
		fs.SetOutput(stderr)
		var verifiers repeatedFlag
		fs.Var(&verifiers, "verify", "a command verifier — exit 0 = pass (repeatable); omit for a self-certified goal (the model claims completion via goal_complete)")
		var llmVerifiers repeatedFlag
		fs.Var(&llmVerifiers, "verify-llm", "an llm_judge verifier — a rubric an independent LLM scores the model's goal_complete claim against (INC-48); claim-gated")
		maxChecks := fs.Int("max-checks", 0, "goal-level budget: max verifier checks before a visible truncation (attach default 10)")
		if err := fs.Parse(reorderFlags(fs, rest)); err != nil {
			return ExitUsage
		}
		gc := &protocol.GoalControl{GoalID: "goal", Goal: strings.Join(fs.Args(), " ")}
		for _, v := range verifiers {
			gc.Verifiers = append(gc.Verifiers, event.GoalVerifier{Kind: "command", Command: v})
		}
		for _, r := range llmVerifiers {
			gc.Verifiers = append(gc.Verifiers, event.GoalVerifier{Kind: "llm_judge", Rubric: r})
		}
		// Only send a Budget when it should change: an update that omits
		// --max-checks must NOT silently reset the running goal's budget
		// (review Bug 2). Attach gets a default.
		switch {
		case sub == "attach":
			mc := *maxChecks
			if mc == 0 {
				mc = 10
			}
			gc.Budget = &event.GoalBudget{MaxChecks: mc}
		case *maxChecks > 0:
			gc.Budget = &event.GoalBudget{MaxChecks: *maxChecks}
		}
		if sub == "attach" && strings.TrimSpace(gc.Goal) == "" {
			fmt.Fprintln(stderr, "goal attach: a goal statement is required")
			return ExitUsage
		}
		return oneShot(stderr, daemon.Command{Cmd: "goal-" + sub, Session: session, Goal: gc}, stdout)
	default:
		fmt.Fprintf(stderr, "goal: unknown subcommand %q (attach|update|pause|resume|cancel)\n", sub)
		return ExitUsage
	}
}

// closeCmd ends a conversational session gracefully (v2 M1.2):
// `agentrunner close <session-id-or-prefix>`.
func closeCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner close <session-id-or-prefix>")
		return ExitUsage
	}
	code := oneShot(stderr, daemon.Command{Cmd: "close", Session: resolvePrefixLenient(args[0])}, stdout)
	if code != ExitOK {
		stuckHint(stderr, args[0])
	}
	return code
}

// stuckHint prints, after a stop command (interrupt/kill/close) could not
// reach a live hosted session, WHICH command to use instead — the audit's
// cross-cutting T1 finding is that these three only ever say "no such live
// session" and dead-end, never pointing at the way out. resume is the
// universal un-stick key: it re-enters the session IN-PROCESS (no daemon
// needed), settles interrupted work, and makes it live again; a following
// close ends it. Best-effort — any read failure just omits the hint and
// leaves the primary error intact.
func stuckHint(stderr io.Writer, sessionArg string) {
	dir, err := resolveSessionDir(sessionArg)
	if err != nil {
		return // unknown session: the primary error already said so
	}
	id := filepath.Base(dir)
	events, err := store.ReadEvents(dir)
	if err != nil {
		return
	}
	s, err := state.Fold(events)
	if err != nil {
		return
	}
	switch {
	case s.Session.Closed != nil:
		fmt.Fprintf(stderr, "  (%s already ended: %s — nothing to stop)\n", id, s.Session.Closed.Reason)
	case store.HasLiveWriter(dir):
		fmt.Fprintf(stderr, "  (%s is hosted by a foreground run/resume, not the daemon — stop it there with Ctrl-C)\n", id)
	default:
		fmt.Fprintf(stderr, "  (%s has no live host — recover it in-process: agentrunner resume %s ; then end it: agentrunner close %s)\n", id, id, id)
	}
}

// daemonDialErr reports a failed daemon dial and tells the user how to start
// one (INC-2 BB-me-7): the daemon is a prerequisite of new/send/attach that
// nothing else surfaces.
func daemonDialErr(stderr io.Writer, err error) {
	fmt.Fprintf(stderr, "agentrunner: %v\n  (no daemon running? start one with: agentrunner daemon --detach)\n", err)
}

// resolvePrefixLenient resolves a session prefix to a full id when possible;
// on any failure it returns the input unchanged (the daemon reports an
// unknown session clearly).
func resolvePrefixLenient(prefix string) string {
	if dir, err := resolveSessionDir(prefix); err == nil {
		// A child session id is a FULL tree address (INC-12.3): its
		// directory basename is only the last hop, so Base() would truncate
		// it — pass child references through verbatim. A child dir is
		// recognizable by its parent hop ("sub"); a TOP-LEVEL dir whose
		// name happens to contain "-sub-" (slug from free prompt text,
		// QA Round1 F-B2) resolves like any other top-level session.
		if filepath.Base(filepath.Dir(dir)) == "sub" {
			return prefix
		}
		return filepath.Base(dir)
	}
	return prefix
}

// resolveAddress resolves a session prefix to its full address, erroring when
// nothing on disk matches. Unlike resolvePrefixLenient it does NOT pass an
// unknown prefix through to the daemon — a send/stop to a session that has no
// journal is a "no session matches" not-found (canonical wording, so the webui
// maps it to a 404 like inspect/close do — QA Wave1 carol-02/dave-09), not a
// daemon-side revive failure surfaced as a 502. A session that exists on disk
// but isn't hosted still resolves here, so the daemon-restart revive path is
// unaffected.
func resolveAddress(prefix string) (string, error) {
	dir, err := resolveSessionDir(prefix)
	if err != nil {
		return "", err
	}
	// A child (tree) address must pass through verbatim — its dir basename is
	// only the last hop (mirrors resolvePrefixLenient).
	if filepath.Base(filepath.Dir(dir)) == "sub" {
		return prefix, nil
	}
	return filepath.Base(dir), nil
}

// oneShot sends a request/reply command and prints the daemon's single reply.
func oneShot(stderr io.Writer, cmd daemon.Command, stdout io.Writer) int {
	if cmd.Session != "" && cmd.CommandID == "" {
		cmd.CommandID = event.NewCommandID()
	}
	sock, err := socketPath()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	var replied, ok bool
	err = daemon.Dial(sock, cmd, func(e protocol.Event) {
		replied = true
		if e.Kind == protocol.KindError {
			fmt.Fprintf(stderr, "agentrunner: %s\n", e.Text)
			return
		}
		ok = true
		fmt.Fprintf(stdout, "%s\n", e.Text)
	})
	if err != nil {
		daemonDialErr(stderr, err)
		return ExitRun
	}
	if !replied || !ok {
		return ExitRun
	}
	return ExitOK
}

// dialUntilStart submits cmd and returns the session id from the first
// RunStart event, then closes the connection (detach). The daemon keeps the
// session running.
func dialUntilStart(sock string, cmd daemon.Command) (string, error) {
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return "", fmt.Errorf("daemon dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if err := json.NewEncoder(conn).Encode(cmd); err != nil {
		return "", err
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e protocol.Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return "", fmt.Errorf("daemon: bad event line: %w", err)
		}
		if e.Kind == protocol.KindError {
			return "", fmt.Errorf("%s", e.Text)
		}
		if e.Kind == protocol.KindSessionStart && e.Session != "" {
			return e.Session, nil // detach: defer closes the conn
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("daemon: read event: %w", err)
	}
	return "", fmt.Errorf("session did not start")
}

// psCmd lists a session's in-flight background work/sub-agents from the
// fold (v2 收口, QA-05/QA-09 观察面): handle, tool, and the spawn target.
// Pure journal read — works with or without a live daemon.
func psCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner ps <session-id-or-prefix>")
		return ExitUsage
	}
	dir, err := resolveSessionDir(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	events, err := store.ReadEvents(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	s, err := state.Fold(events)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	if len(s.Handles) == 0 {
		fmt.Fprintln(stdout, "no background work in flight")
		return ExitOK
	}
	handles := make([]string, 0, len(s.Handles))
	for h := range s.Handles {
		handles = append(handles, h)
	}
	sort.Strings(handles)
	for _, h := range handles {
		act := s.Handles[h]
		target := ""
		if act.Name == "spawn_agent" {
			var a struct {
				Agent  string `json:"agent"`
				Prompt string `json:"prompt"`
			}
			_ = json.Unmarshal(act.Args, &a)
			target = " agent=" + a.Agent + " prompt=" + truncateArg(a.Prompt, 60)
		}
		fmt.Fprintf(stdout, "%s\t%s\trunning%s\n", h, act.Name, target)
	}
	return ExitOK
}

func truncateArg(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
