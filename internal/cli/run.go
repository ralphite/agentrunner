package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/gemini"
	"github.com/ralphite/agentrunner/internal/provider/record"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// providerFactory builds the provider named by the spec; tests override it.
type providerFactory func(ctx context.Context, name string) (provider.Provider, error)

// errUnknownProvider marks a spec/usage error (exit 2) as opposed to a
// provider construction failure (exit 1, e.g. missing credentials).
var errUnknownProvider = errors.New("unknown provider")

func defaultProviderFactory(ctx context.Context, name string) (provider.Provider, error) {
	switch name {
	case "gemini":
		return gemini.New(ctx)
	case "scripted":
		// Test seam for acceptance scenarios and offline demos.
		path := os.Getenv("AGENTRUNNER_SCRIPTED_FIXTURE")
		if path == "" {
			return nil, fmt.Errorf("provider scripted: AGENTRUNNER_SCRIPTED_FIXTURE not set")
		}
		return scripted.Load(path)
	default:
		return nil, fmt.Errorf("%w %q (available: gemini, scripted)", errUnknownProvider, name)
	}
}

// runOptions carries everything runAgent needs; factored for testability.
type runOptions struct {
	specPath   string
	task       string
	workspace  string
	maxTurns   int
	mode       string
	fixtureOut string // record-fixture mode when non-empty
	version    string
	factory    providerFactory
	stdout     io.Writer
	stderr     io.Writer
}

// runCmd parses `run` / `record-fixture` args and executes the agent.
func runCmd(args []string, recordMode bool, version string, stdout, stderr io.Writer) int {
	name := "run"
	if recordMode {
		name = "record-fixture"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceDir := fs.String("workspace", ".", "workspace root (default: current directory)")
	maxTurns := fs.Int("max-turns", 0, "override spec max_turns")
	mode := fs.String("mode", "", "run mode: default|plan|acceptEdits|bypass (overrides spec)")
	fixtureOut := fs.String("o", "", "fixture output path (record-fixture only)")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintf(stderr, "usage: agentrunner %s [flags] <spec.yaml> \"task\"\n", name)
		return ExitUsage
	}
	if recordMode && *fixtureOut == "" {
		fmt.Fprintf(stderr, "record-fixture: -o <file> is required\n")
		return ExitUsage
	}
	if !recordMode {
		*fixtureOut = ""
	}

	return runAgent(runOptions{
		specPath:   rest[0],
		task:       rest[1],
		workspace:  *workspaceDir,
		maxTurns:   *maxTurns,
		mode:       *mode,
		fixtureOut: *fixtureOut,
		version:    version,
		factory:    defaultProviderFactory,
		stdout:     stdout,
		stderr:     stderr,
	})
}

func runAgent(opts runOptions) int {
	ctx, interrupts, stop := signalContext()
	defer stop()
	loadDotEnv(".env")

	spec, err := agent.LoadSpec(opts.specPath)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitUsage
	}
	if opts.maxTurns > 0 {
		spec.MaxTurns = opts.maxTurns
	}

	ws, err := workspace.New(opts.workspace)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitUsage
	}

	prov, err := opts.factory(ctx, spec.Model.Provider)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		if errors.Is(err, errUnknownProvider) {
			return ExitUsage
		}
		return ExitRun // construction failure (e.g. missing credentials)
	}
	var recorder *record.Recorder
	if opts.fixtureOut != "" {
		recorder = record.New(prov)
		prov = recorder
	}

	sessionID := runtime.NewSessionID(time.Now(), opts.task)
	sessionDir, err := runtime.SessionDir(sessionID)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitRun
	}
	events, err := store.OpenEventStore(sessionDir)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitRun
	}
	defer func() { _ = events.Close() }()

	fmt.Fprintf(opts.stderr, "session %s\n", sessionID)

	mode := spec.Mode
	if opts.mode != "" {
		mode = opts.mode
	}
	pipe, err := buildPipeline(ws, spec.Permissions, mode, spec.Budget.MaxTotalTokens, opts.stderr)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitRun
	}

	loop := &agent.Loop{
		Spec:       spec,
		Provider:   prov,
		Exec:       &tool.Executor{WS: ws, Session: sessionID},
		Store:      events,
		Clock:      clock.Real{},
		Sink:       &textSink{out: opts.stdout},
		SessionID:  sessionID,
		Version:    opts.version,
		Interrupts: interrupts,
		Pipeline:   pipe,
		Mode:       mode,
	}
	result, runErr := loop.Run(ctx, opts.task)

	// A recorded session is valuable even when the run errored (real tokens
	// were spent); write the fixture regardless.
	if recorder != nil {
		if err := recorder.WriteFixture(opts.fixtureOut); err != nil {
			fmt.Fprintln(opts.stderr, err)
			return ExitRun
		}
		fmt.Fprintf(opts.stderr, "fixture written to %s\n", opts.fixtureOut)
	}

	if runErr != nil {
		fmt.Fprintf(opts.stderr, "run failed: %v\n", runErr)
		return ExitRun
	}

	fmt.Fprintf(opts.stderr, "run %s: %d turns, %d in / %d out tokens\n",
		result.Reason, result.Turns, result.Usage.InputTokens, result.Usage.OutputTokens)
	if result.Reason != "completed" {
		// max_turns 等强制停止不算成功完成（review 修订：脚本/CI 不应
		// 把卡死的 agent 当成功）。
		return ExitRun
	}
	return ExitOK
}

// signalContext maps terminal signals onto run semantics: the FIRST
// Ctrl-C is an interrupt (denies a pending approval, the run continues);
// the second Ctrl-C — or any SIGTERM — cancels the run outright (tool
// process groups are killed via ctx).
func signalContext() (context.Context, <-chan struct{}, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	interrupts := make(chan struct{})
	sigc := make(chan os.Signal, 2)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	go func() {
		first := true
		for sig := range sigc {
			if sig == os.Interrupt && first {
				first = false
				close(interrupts)
				continue
			}
			cancel()
			return
		}
	}()
	return ctx, interrupts, func() {
		signal.Stop(sigc)
		close(sigc)
		cancel()
	}
}

// textSink renders turn-granularity output to stdout (S1; streaming in S4).
type textSink struct {
	out io.Writer
}

func (s *textSink) AssistantText(turn int, text string) {
	fmt.Fprintf(s.out, "\n[turn %d]\n%s\n", turn, text)
}

func (s *textSink) ToolCall(turn int, call provider.ToolCall) {
	fmt.Fprintf(s.out, "  → %s %s\n", call.Name, compactJSON(call.Args, 120))
}

func (s *textSink) ToolResult(_ int, _ string, res tool.Result) {
	status := "ok"
	if res.IsError {
		status = "error"
	}
	fmt.Fprintf(s.out, "  ← %s %s\n", status, compactJSON(res.Payload, 200))
}

func compactJSON(raw json.RawMessage, max int) string {
	s := string(raw)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// loadDotEnv populates missing env vars from a .env file in the cwd
// (local convenience per PLAN §0; never overrides existing values).
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok || os.Getenv(k) != "" {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		_ = os.Setenv(k, v)
	}
}
