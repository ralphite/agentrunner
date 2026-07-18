package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/ralphite/agentrunner/internal/provider"
)

// optimizeOptions carries everything runOptimize needs; factored for tests.
type optimizeOptions struct {
	draft   string
	model   string
	prov    string
	context string // optional context to resolve ambiguous references
	factory providerFactory
	stdout  io.Writer
	stderr  io.Writer
}

// optimizeCmd rewrites a draft prompt into a clearer instruction via the
// provider (INC-56, HANDA-PARITY #19):
//
//	agentrunner optimize "fix the thing that broke"
//	echo "make it faster" | agentrunner optimize -
//	agentrunner optimize --context "editing the auth module" "clean it up"
//
// Like dictate, it is a one-shot provider call (no daemon/session/journal) and
// prints ONLY the rewritten prompt to stdout so the webui thin shell can drop
// it straight into the composer. Undo is the caller's concern: the original
// draft is never mutated here, so the webui keeps it in memory for a
// single-step revert.
func optimizeCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("optimize", flag.ContinueOnError)
	fs.SetOutput(stderr)
	model := fs.String("model", defaultHelperModel, "model id")
	prov := fs.String("provider", defaultHelperProvider, "provider name")
	contextHint := fs.String("context", "", "optional context to resolve ambiguous references in the draft")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	rest, terr := completeTextArg(fs.Args(), 1)
	if terr != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", terr)
		return ExitUsage
	}
	if len(rest) != 1 || strings.TrimSpace(rest[0]) == "" {
		fmt.Fprintln(stderr, "usage: agentrunner optimize [--context \"...\"] \"draft prompt\"  (draft may be piped: echo draft | agentrunner optimize)")
		return ExitUsage
	}
	return runOptimize(optimizeOptions{
		draft:   rest[0],
		model:   *model,
		prov:    *prov,
		context: *contextHint,
		factory: defaultProviderFactory,
		stdout:  stdout,
		stderr:  stderr,
	})
}

func runOptimize(opts optimizeOptions) int {
	loadDotEnv(".env")
	if opts.factory == nil {
		opts.factory = defaultProviderFactory
	}
	draft := strings.TrimSpace(opts.draft)
	if draft == "" {
		fmt.Fprintln(opts.stderr, "agentrunner: optimize needs a non-empty draft")
		return ExitUsage
	}

	ctx := context.Background()
	prov, err := opts.factory(ctx, opts.prov)
	if err != nil {
		fmt.Fprintln(opts.stderr, err)
		if errors.Is(err, errUnknownProvider) {
			return ExitUsage
		}
		return ExitRun
	}

	req := provider.CompleteRequest{
		Model:     opts.model,
		MaxTokens: 2048,
		System:    optimizeSystemPrompt(opts.context),
		Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{
			{Kind: provider.PartText, Text: draft},
		}}},
	}
	turn, err := provider.CollectTurnStreaming(prov.Complete(ctx, req), func(string) {})
	if err != nil {
		fmt.Fprintf(opts.stderr, "agentrunner: optimize failed: %v\n", err)
		return ExitRun
	}
	optimized := strings.TrimSpace(assistantMessageText(turn.Message))
	if optimized == "" {
		fmt.Fprintln(opts.stderr, "agentrunner: the model returned no rewrite")
		return ExitRun
	}
	fmt.Fprintln(opts.stdout, optimized)
	return ExitOK
}

// optimizeSystemPrompt instructs a rewrite that preserves intent, sharpens
// specificity, and resolves vague references using the caller's context.
func optimizeSystemPrompt(contextHint string) string {
	var b strings.Builder
	b.WriteString("You are a prompt optimizer for an AI coding agent. Rewrite the user's DRAFT instruction so it is clearer, ")
	b.WriteString("more specific, and unambiguous, while preserving the user's original intent and every concrete detail they gave. ")
	b.WriteString("Resolve vague references (\"it\", \"that\", \"the thing\") using the context when one is provided. ")
	b.WriteString("Keep it concise — do not invent requirements the draft doesn't imply. ")
	b.WriteString("Output ONLY the rewritten instruction: no preamble, no explanation, no quotation marks, no markdown code fences.")
	if h := strings.TrimSpace(contextHint); h != "" {
		b.WriteString("\n\nContext (what the user is currently working on):\n")
		b.WriteString(h)
	}
	return b.String()
}
