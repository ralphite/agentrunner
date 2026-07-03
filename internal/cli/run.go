package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
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
		return nil, fmt.Errorf("unknown provider %q (available: gemini, scripted)", name)
	}
}

// runOptions carries everything runAgent needs; factored for testability.
type runOptions struct {
	specPath   string
	task       string
	workspace  string
	maxTurns   int
	fixtureOut string // record-fixture mode when non-empty
	factory    providerFactory
	stdout     io.Writer
	stderr     io.Writer
}

// runCmd parses `run` / `record-fixture` args and executes the agent.
func runCmd(args []string, recordMode bool, stdout, stderr io.Writer) int {
	name := "run"
	if recordMode {
		name = "record-fixture"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceDir := fs.String("workspace", ".", "workspace root (default: current directory)")
	maxTurns := fs.Int("max-turns", 0, "override spec max_turns")
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
		fixtureOut: *fixtureOut,
		factory:    defaultProviderFactory,
		stdout:     stdout,
		stderr:     stderr,
	})
}

func runAgent(opts runOptions) int {
	ctx := context.Background()
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
		return ExitUsage
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
	journal, err := store.OpenJournal(filepath.Join(sessionDir, "journal.jsonl"))
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitRun
	}
	defer func() { _ = journal.Close() }()

	if err := journal.RecordRunMeta(store.RunMeta{
		SpecName: spec.Name, Model: spec.Model.ID, Task: opts.task, Version: "dev",
	}); err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitRun
	}

	fmt.Fprintf(opts.stderr, "session %s\n", sessionID)

	loop := &agent.Loop{
		Spec:     spec,
		Provider: prov,
		Exec:     &tool.Executor{WS: ws},
		Journal:  journal,
		Sink:     &textSink{out: opts.stdout},
	}
	result, err := loop.Run(ctx, opts.task)
	if err != nil {
		fmt.Fprintf(opts.stderr, "run failed: %v\n", err)
		return ExitRun
	}

	if recorder != nil {
		if err := recorder.WriteFixture(opts.fixtureOut); err != nil {
			fmt.Fprintln(opts.stderr, err)
			return ExitRun
		}
		fmt.Fprintf(opts.stderr, "fixture written to %s\n", opts.fixtureOut)
	}

	fmt.Fprintf(opts.stderr, "run %s: %d turns, %d in / %d out tokens\n",
		result.Reason, result.Turns, result.Usage.InputTokens, result.Usage.OutputTokens)
	return ExitOK
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
		k, v, ok := strings.Cut(line, "=")
		if ok && os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}
