package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestOptimizeRewritesDraft is the round-trip: the draft reaches the model as
// the user turn, the rewrite comes back on stdout alone, and the original draft
// is never mutated (the webui keeps it in memory for single-step undo).
func TestOptimizeRewritesDraft(t *testing.T) {
	draft := "fix the thing that broke"
	fake := &fakeHelperProvider{reply: "Fix the failing auth-token refresh in the login flow.\n"}
	var out, errb bytes.Buffer
	code := runOptimize(optimizeOptions{
		draft:   draft,
		model:   "test-model",
		context: "working in internal/auth",
		factory: fakeFactory(fake),
		stdout:  &out,
		stderr:  &errb,
	})
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errb.String())
	}
	if got := out.String(); got != "Fix the failing auth-token refresh in the login flow.\n" {
		t.Fatalf("stdout = %q, want the trimmed rewrite", got)
	}
	if len(fake.requests) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(fake.requests))
	}
	req := fake.requests[0]
	// The draft is the user turn, carried verbatim (not pre-mangled).
	if len(req.Messages) != 1 || len(req.Messages[0].Parts) != 1 || req.Messages[0].Parts[0].Text != draft {
		t.Fatalf("user turn = %+v, want the draft verbatim", req.Messages)
	}
	// Context rides the system prompt so vague references resolve.
	if !strings.Contains(req.System, "internal/auth") {
		t.Errorf("context missing from system prompt: %q", req.System)
	}
	// Rewrite is a tool-less turn.
	if len(req.Tools) != 0 {
		t.Errorf("optimize issued tools: %+v", req.Tools)
	}
}

// TestOptimizeNoContextStillWorks: context is optional; without it the system
// prompt carries no context block but the rewrite still happens.
func TestOptimizeNoContextStillWorks(t *testing.T) {
	fake := &fakeHelperProvider{reply: "Make the build script run in parallel."}
	var out, errb bytes.Buffer
	code := runOptimize(optimizeOptions{draft: "make it faster", factory: fakeFactory(fake), stdout: &out, stderr: &errb})
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errb.String())
	}
	if strings.Contains(fake.requests[0].System, "Context (") {
		t.Errorf("no-context call still injected a context block: %q", fake.requests[0].System)
	}
}

func TestOptimizeEmptyDraft(t *testing.T) {
	fake := &fakeHelperProvider{reply: "x"}
	var out, errb bytes.Buffer
	if code := runOptimize(optimizeOptions{draft: "   ", factory: fakeFactory(fake), stdout: &out, stderr: &errb}); code != ExitUsage {
		t.Fatalf("empty draft exit = %d, want ExitUsage", code)
	}
	if len(fake.requests) != 0 {
		t.Errorf("provider called on empty draft")
	}
}

// TestOptimizeSurfacesProviderError: a provider failure is a non-zero exit with
// the reason on stderr, never a silent empty rewrite the webui would paste.
func TestOptimizeSurfacesProviderError(t *testing.T) {
	fake := &fakeHelperProvider{err: errors.New("provider unreachable")}
	var out, errb bytes.Buffer
	code := runOptimize(optimizeOptions{draft: "clean it up", factory: fakeFactory(fake), stdout: &out, stderr: &errb})
	if code != ExitRun {
		t.Fatalf("exit = %d, want ExitRun on provider error", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want nothing on failure", out.String())
	}
	if !strings.Contains(errb.String(), "provider unreachable") {
		t.Errorf("stderr = %q, want the provider error surfaced", errb.String())
	}
}

func TestOptimizeCmdUsage(t *testing.T) {
	var out, errb bytes.Buffer
	if code := optimizeCmd(nil, &out, &errb); code != ExitUsage {
		t.Errorf("no-arg optimize exit = %d, want ExitUsage", code)
	}
	if !strings.Contains(errb.String(), "usage:") {
		t.Errorf("stderr = %q, want a usage line", errb.String())
	}
}
