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
	"strings"

	"github.com/ralphite/agentrunner/internal/daemon"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
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
	if err := fs.Parse(reorderFlags(fs, args)); err != nil {
		return ExitUsage
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintln(stderr, `usage: agentrunner new [flags] <spec.yaml> "opening message"`)
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
	sock, err := socketPath()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRun
	}
	cmd := daemon.Command{
		Cmd: "run", SpecPath: specPath, Task: rest[1],
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
	if err := fs.Parse(reorderFlags(fs, args)); err != nil {
		return ExitUsage
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintln(stderr, `usage: agentrunner send [flags] <session-id-or-prefix> "message"`)
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
	cmd := daemon.Command{Cmd: "send", Session: resolvePrefixLenient(rest[0]),
		Text: rest[1], Images: images, Files: files, CommandID: event.NewCommandID(),
		Principal: "local-user", Source: "cli", Trust: "local"}
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

// loadImageAttachments reads each image file and sniffs its media type.
func loadImageAttachments(paths []string) ([]protocol.ImageAttachment, error) {
	var out []protocol.ImageAttachment
	for _, path := range paths {
		data, err := os.ReadFile(path)
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
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, protocol.FileAttachment{MediaType: http.DetectContentType(data), Data: data})
	}
	return out, nil
}

// killCmd cancels one running child/task by handle (v2 M3.2):
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

// stopCmd remotely tears down a hosted run (G12): graceful cancel with no
// mark — the session lands in durable standby and `send` revives it.
// Distinct from interrupt (turn-level) and close (a mark).
func stopCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner stop <session-id-or-prefix>")
		return ExitUsage
	}
	code := oneShot(stderr, daemon.Command{Cmd: "stop", Session: resolvePrefixLenient(args[0])}, stdout)
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
		fmt.Fprintln(stderr, "usage: agentrunner goal <session-id-or-prefix> <attach|update|pause|resume|cancel> [flags]")
		return ExitUsage
	}
	session := resolvePrefixLenient(args[0])
	sub, rest := args[1], args[2:]
	switch sub {
	case "pause", "resume", "cancel":
		return oneShot(stderr, daemon.Command{Cmd: "goal-" + sub, Session: session}, stdout)
	case "attach", "update":
		fs := flag.NewFlagSet("goal "+sub, flag.ContinueOnError)
		fs.SetOutput(stderr)
		var verifiers repeatedFlag
		fs.Var(&verifiers, "verify", "a command verifier — exit 0 = pass (repeatable); omit for a self-certified goal (the model claims completion via goal_complete)")
		maxChecks := fs.Int("max-checks", 0, "goal-level budget: max verifier checks before a visible truncation (attach default 10)")
		if err := fs.Parse(reorderFlags(fs, rest)); err != nil {
			return ExitUsage
		}
		gc := &protocol.GoalControl{GoalID: "goal", Goal: strings.Join(fs.Args(), " ")}
		for _, v := range verifiers {
			gc.Verifiers = append(gc.Verifiers, event.GoalVerifier{Kind: "command", Command: v})
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
		return filepath.Base(dir)
	}
	return prefix
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
	return "", fmt.Errorf("session did not start")
}

// psCmd lists a session's in-flight background tasks/sub-agents from the
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
		fmt.Fprintln(stdout, "no tasks in flight")
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
				Agent string `json:"agent"`
				Task  string `json:"task"`
			}
			_ = json.Unmarshal(act.Args, &a)
			target = " agent=" + a.Agent + " task=" + truncateArg(a.Task, 60)
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
