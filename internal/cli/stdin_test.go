package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// withStdin swaps the stdinSource seam for one test: piped=true simulates
// `echo s | ar ...`, piped=false an interactive terminal.
func withStdin(t *testing.T, s string, piped bool) {
	t.Helper()
	old := stdinSource
	stdinSource = func() (io.Reader, bool) {
		if !piped {
			return nil, false
		}
		return strings.NewReader(s), true
	}
	t.Cleanup(func() { stdinSource = old })
}

func TestCompleteTextArgPipedAppendsMissingText(t *testing.T) {
	withStdin(t, "fix the failing test\n", true)
	rest, err := completeTextArg([]string{"spec.yaml"}, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rest) != 2 || rest[1] != "fix the failing test" {
		t.Fatalf("want appended trimmed text, got %#v", rest)
	}
}

func TestCompleteTextArgExplicitDashReplaces(t *testing.T) {
	withStdin(t, "line one\nline two\n\n", true)
	rest, err := completeTextArg([]string{"sid123", "-"}, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Trailing newlines trimmed, interior newlines preserved.
	if rest[1] != "line one\nline two" {
		t.Fatalf("want multi-line text with trailing newlines trimmed, got %q", rest[1])
	}
}

func TestCompleteTextArgEmptyPipeErrors(t *testing.T) {
	withStdin(t, "\n", true)
	if _, err := completeTextArg([]string{"spec.yaml"}, 2); err == nil {
		t.Fatal("want error for empty piped stdin, got nil")
	}
}

func TestCompleteTextArgNoPipeMissingArgUntouched(t *testing.T) {
	withStdin(t, "", false)
	rest, err := completeTextArg([]string{"spec.yaml"}, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rest) != 1 {
		t.Fatalf("want args untouched so caller prints usage, got %#v", rest)
	}
}

func TestCompleteTextArgDashOnTTYErrors(t *testing.T) {
	withStdin(t, "", false)
	if _, err := completeTextArg([]string{"sid123", "-"}, 2); err == nil {
		t.Fatal(`want error for "-" on a terminal (must not block), got nil`)
	}
}

func TestCompleteTextArgFullArgsUntouched(t *testing.T) {
	withStdin(t, "should not be read", true)
	rest, err := completeTextArg([]string{"spec.yaml", "do it"}, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rest[1] != "do it" {
		t.Fatalf("explicit text argument must win over piped stdin, got %q", rest[1])
	}
}

// TestRunCmdPipedTaskSkipsUsage proves the stdin path is wired into `run`:
// with a piped task and only the spec argument, the command must get past the
// usage check and fail on the (nonexistent) spec instead.
func TestRunCmdPipedTaskSkipsUsage(t *testing.T) {
	withStdin(t, "do the thing\n", true)
	var out, errBuf bytes.Buffer
	code := runCmd([]string{"definitely-missing-spec.yaml"}, false, "test", &out, &errBuf)
	if code != ExitUsage && code != ExitRun {
		t.Fatalf("unexpected exit code %d (stderr: %s)", code, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "usage:") {
		t.Fatalf("piped task must satisfy the arity check, still got usage: %s", errBuf.String())
	}
}
