package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/clock"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/provider/anthropic"
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

// knownProviderName reports whether name is a provider the CLI can construct.
// Kept in lock-step with defaultProviderFactory's switch so callers (e.g.
// `ar new`) can reject an unknown provider before minting a session.
func knownProviderName(name string) bool {
	switch name {
	case "gemini", "anthropic", "scripted":
		return true
	default:
		return false
	}
}

func defaultProviderFactory(ctx context.Context, name string) (provider.Provider, error) {
	switch name {
	case "gemini":
		return gemini.New(ctx)
	case "anthropic":
		return anthropic.New(ctx)
	case "scripted":
		// Test seam for acceptance scenarios and offline demos.
		path := os.Getenv("AGENTRUNNER_SCRIPTED_FIXTURE")
		if path == "" {
			return nil, fmt.Errorf("provider scripted: AGENTRUNNER_SCRIPTED_FIXTURE not set")
		}
		return scripted.Load(path)
	default:
		return nil, fmt.Errorf("%w %q (available: gemini, anthropic, scripted)", errUnknownProvider, name)
	}
}

// siblingSpecResolver resolves a sub-agent name to <name>.yaml next to the
// parent spec (S5.3), OR to a shipped built-in agent (explore/plan, INC-25).
// The spec.Agents whitelist gates WHO may be spawned; this only answers WHERE
// the spec lives. Built-in agents are tried first (a workspace file of the
// same name would otherwise shadow the shipped read-only agent), and they
// inherit the PARENT's model so a built-in explore runs on the same provider
// the user chose — the shipped default is only a fallback.
func siblingSpecResolver(parentSpecPath string) agent.SubSpecResolver {
	dir := filepath.Dir(parentSpecPath)
	return func(name string) (*agent.AgentSpec, error) {
		if spec, ok := agent.BuiltinSpec(name); ok {
			if parent, err := agent.LoadSpec(parentSpecPath); err == nil && parent.Model.Provider != "" {
				spec.Model = parent.Model
			}
			return spec, nil
		}
		return agent.LoadSpec(filepath.Join(dir, name+".yaml"))
	}
}

// runOptions carries everything runAgent needs; factored for testability.
type runOptions struct {
	specPath           string
	prompt             string
	workspace          string
	maxGenerationSteps int
	mode               string
	fixtureOut         string // record-fixture mode when non-empty
	version            string
	factory            providerFactory
	stdout             io.Writer
	stderr             io.Writer
	sink               protocol.Sink // output protocol renderer (nil → text on stdout)
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
	maxGenerationSteps := fs.Int("max-generation-steps", 0, "override spec max_generation_steps")
	mode := fs.String("mode", "", "run mode: default|plan|acceptEdits|bypass (overrides spec)")
	jsonOut := fs.Bool("json", false, "emit the output event stream as JSON lines")
	fixtureOut := fs.String("o", "", "fixture output path (record-fixture only)")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	rest, terr := completeTextArg(fs.Args(), 2)
	if terr != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", terr)
		return ExitUsage
	}
	if len(rest) != 2 {
		fmt.Fprintf(stderr, "usage: agentrunner %s [flags] <spec.yaml> \"prompt\"  (prompt may be piped: echo prompt | agentrunner %s spec.yaml)\n", name, name)
		return ExitUsage
	}
	if strings.TrimSpace(rest[1]) == "" {
		// Catch empty AND whitespace-only here like `new`/`send` do — otherwise a
		// blank prompt reaches the provider as a raw 400, or (whitespace-only)
		// silently creates a junk session (QA Round1 F-A05, Wave1 cli-life-09).
		fmt.Fprintf(stderr, "agentrunner: %s needs a non-empty prompt\n", name)
		return ExitUsage
	}
	if recordMode && *fixtureOut == "" {
		fmt.Fprintf(stderr, "record-fixture: -o <file> is required\n")
		return ExitUsage
	}
	if !recordMode && *fixtureOut != "" {
		// Refuse loudly instead of silently ignoring it (PLAN 5.4): the user
		// asked for a fixture and `run` does not record one.
		fmt.Fprintf(stderr, "run: -o records nothing here — use `agentrunner record-fixture -o %s <spec.yaml> \"prompt\"`\n", *fixtureOut)
		return ExitUsage
	}

	var sink protocol.Sink
	if *jsonOut {
		sink = protocol.NewJSONSink(stdout)
	} else {
		sink = newTextRenderer(stdout)
	}
	return runAgent(runOptions{
		specPath:           rest[0],
		prompt:             rest[1],
		workspace:          *workspaceDir,
		maxGenerationSteps: *maxGenerationSteps,
		mode:               *mode,
		fixtureOut:         *fixtureOut,
		version:            version,
		factory:            defaultProviderFactory,
		stdout:             stdout,
		sink:               sink,
		stderr:             stderr,
	})
}

func runAgent(opts runOptions) int {
	if opts.sink == nil {
		opts.sink = newTextRenderer(opts.stdout)
	}
	ctx, interrupts, stop := signalContext()
	defer stop()
	loadDotEnv(".env")

	spec, err := agent.LoadSpec(opts.specPath)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitUsage
	}
	if opts.maxGenerationSteps > 0 {
		spec.MaxGenerationSteps = opts.maxGenerationSteps
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

	sessionID := runtime.NewSessionID(time.Now(), opts.prompt)
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
	pipe, hooks, err := buildPipeline(ws, spec.Permissions, mode, spec.Budget.MaxTotalTokens, opts.stderr)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		return ExitRun
	}

	loop := &agent.Loop{
		Spec:           spec,
		Provider:       prov,
		Exec:           &tool.Executor{WS: ws, Session: sessionID},
		Store:          events,
		Clock:          clock.Real{},
		Out:            opts.sink,
		SessionID:      sessionID,
		Version:        opts.version,
		Interrupts:     interrupts,
		Pipeline:       pipe,
		Mode:           mode,
		Hooks:          hooks,
		Approvals:      approvalResolver(opts.stderr),
		SubSpecs:       siblingSpecResolver(opts.specPath),
		Snapshots:      snapshotStoreFor(ws, opts.stderr),
		DurableOpening: true,
	}
	result, runErr := loop.Run(ctx, opts.prompt)

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
		result.Reason, result.GenSteps, result.Usage.InputTokens, result.Usage.OutputTokens)
	switch result.Reason {
	case "completed", "handoff":
		// handoff 是一次正常的控制权移交（子 agent 已 completed），与 completed
		// 同属成功终止。只有 max_generation_steps 等强制停止才算失败（review 修订：
		// 脚本/CI 不应把卡死的 agent 当成功，也不应把成功的 handoff 当失败）。
		return ExitOK
	default:
		return ExitRun
	}
}

// signalContext maps terminal signals onto run semantics: the FIRST
// Ctrl-C is an interrupt (denies a pending approval, the run continues);
// the second Ctrl-C — or any SIGTERM — cancels the run outright (tool
// process groups are killed via ctx).
func signalContext() (context.Context, <-chan struct{}, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	// Buffered 1: the first Ctrl-C delivers ONE steering interrupt (the loop
	// cancels the current activity and continues); a second Ctrl-C — or any
	// SIGTERM — is a hard quit that cancels the run context.
	interrupts := make(chan struct{}, 1)
	sigc := make(chan os.Signal, 2)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	go func() {
		first := true
		for sig := range sigc {
			if sig == os.Interrupt && first {
				first = false
				select {
				case interrupts <- struct{}{}:
				default:
				}
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
