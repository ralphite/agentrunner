package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--version"}, "test-1.0", &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	got := out.String()
	if !strings.HasPrefix(got, "agentrunner test-1.0 (go") {
		t.Fatalf("version output = %q, want prefix %q", got, "agentrunner test-1.0 (go")
	}
}

func TestUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"bogus"}, "dev", &out, &errOut)
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Fatalf("stderr = %q, want to contain %q", errOut.String(), "unknown command")
	}
	// INC-2 BB-me-2: the short usage must point at `help`.
	if !strings.Contains(errOut.String(), "agentrunner help") {
		t.Fatalf("stderr = %q, want a pointer to `agentrunner help`", errOut.String())
	}
}

// INC-2 BB-me-1: a first-time user's very first gesture — top-level
// --help/-h/help — must succeed and include the quick start.
func TestTopLevelHelp(t *testing.T) {
	for _, arg := range []string{"--help", "-h", "help"} {
		var out, errOut bytes.Buffer
		code := Run([]string{arg}, "dev", &out, &errOut)
		if code != ExitOK {
			t.Fatalf("%s: exit code = %d, want %d (stderr: %s)", arg, code, ExitOK, errOut.String())
		}
		for _, want := range []string{"Quick start", "agentrunner init", "new <spec.yaml>", "attach <session>"} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("%s: stdout missing %q\n%s", arg, want, out.String())
			}
		}
	}
}

func TestNoArgs(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(nil, "dev", &out, &errOut)
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
}
