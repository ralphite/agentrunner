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
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// newCmd starts a daemon-hosted CONVERSATIONAL session and detaches once it
// has an id (v2 M1.2): `agentrunner new <spec.yaml> "opening message"
// [--workspace dir]`. The session outlives this client — `send`/`attach`/
// `close` address it by the printed id.
func newCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceDir := fs.String("workspace", ".", "workspace root")
	mode := fs.String("mode", "", "run mode: default|plan|acceptEdits")
	if err := fs.Parse(args); err != nil {
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
	// Detach after RunStart: the conversational session never ends on its
	// own, so streaming-until-end would block. Read just the id and leave.
	sid, derr := dialUntilStart(sock, daemon.Command{
		Cmd: "run", Conversational: true, SpecPath: specPath, Task: rest[1],
		Workspace: wsAbs, Mode: *mode,
	})
	if derr != nil {
		fmt.Fprintf(stderr, "agentrunner: %v (is the daemon running?)\n", derr)
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

// sendCmd delivers a user message to a live conversational session (v2 M1.2):
// `agentrunner send [--image f.png]... <session-id-or-prefix> "message"`.
// Attached images ride the command line base64 (v2 M4.1); the agent stores
// them in the session CAS before journaling the input.
func sendCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var imagePaths repeatedFlag
	fs.Var(&imagePaths, "image", "attach an image file (repeatable)")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintln(stderr, `usage: agentrunner send [--image file]... <session-id-or-prefix> "message"`)
		return ExitUsage
	}
	images, err := loadImageAttachments(imagePaths)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	return oneShot(stderr, daemon.Command{Cmd: "send", Session: resolvePrefixLenient(rest[0]),
		Text: rest[1], Images: images}, stdout)
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

// killCmd cancels one running child/task by handle (v2 M3.2):
// `agentrunner kill <session-id-or-prefix> <handle>`.
func killCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "usage: agentrunner kill <session-id-or-prefix> <handle>")
		return ExitUsage
	}
	return oneShot(stderr, daemon.Command{Cmd: "kill", Session: resolvePrefixLenient(args[0]), Handle: args[1]}, stdout)
}

// interruptCmd delivers an out-of-band interrupt to a live session (v2
// M2.3): `agentrunner interrupt <session-id-or-prefix>` — steers a running
// turn or closes an idle one; it is NOT a message.
func interruptCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner interrupt <session-id-or-prefix>")
		return ExitUsage
	}
	return oneShot(stderr, daemon.Command{Cmd: "interrupt", Session: resolvePrefixLenient(args[0])}, stdout)
}

// closeCmd ends a conversational session gracefully (v2 M1.2):
// `agentrunner close <session-id-or-prefix>`.
func closeCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner close <session-id-or-prefix>")
		return ExitUsage
	}
	return oneShot(stderr, daemon.Command{Cmd: "close", Session: resolvePrefixLenient(args[0])}, stdout)
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
		fmt.Fprintf(stderr, "agentrunner: %v (is the daemon running?)\n", err)
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
		if e.Kind == protocol.KindRunStart && e.Session != "" {
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
	if len(s.Tasks) == 0 {
		fmt.Fprintln(stdout, "no tasks in flight")
		return ExitOK
	}
	handles := make([]string, 0, len(s.Tasks))
	for h := range s.Tasks {
		handles = append(handles, h)
	}
	sort.Strings(handles)
	for _, h := range handles {
		act := s.Tasks[h]
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
