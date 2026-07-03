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
}

func TestNoArgs(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(nil, "dev", &out, &errOut)
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
}
