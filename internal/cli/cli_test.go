package cli

import (
	"bytes"
	"os"
	"path/filepath"
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

// -h/--help after a positional command must show help — never be swallowed
// as a session id or path, and never trigger the command's side effect
// (`init -h` used to write a file named "-h", `trust -h` trusted one).
// QA Round1 F-A07/F-A09.
func TestPositionalCommandsHonorHelpFlag(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, cmd := range []string{
		"init", "resume", "close", "interrupt", "stop", "compact", "clear",
		"goal", "schedule", "agent", "kill", "ps", "approve", "barrier", "sessions", "trust",
	} {
		for _, h := range []string{"-h", "--help"} {
			var out, errOut bytes.Buffer
			code := Run([]string{cmd, h}, "test", &out, &errOut)
			if code != ExitOK {
				t.Errorf("%s %s: exit %d, want 0 (stderr: %s)", cmd, h, code, errOut.String())
			}
			if !strings.Contains(out.String(), "usage: agentrunner "+cmd) {
				t.Errorf("%s %s: stdout missing usage line:\n%s", cmd, h, out.String())
			}
		}
	}
	// The old side effect: no file named -h may appear.
	if _, err := os.Stat(filepath.Join(dir, "-h")); err == nil {
		t.Fatal("init -h wrote a file named -h")
	}
}

// TestHelpSubcommand pins that `help <command>` prints that command's usage,
// not the global help, and that an unknown command falls back to global (QA
// Wave1 cli-life-05).
func TestHelpSubcommand(t *testing.T) {
	for _, cmd := range []string{"sessions", "goal", "schedule", "resume", "approve", "mode"} {
		var out, errOut bytes.Buffer
		if code := Run([]string{"help", cmd}, "test", &out, &errOut); code != ExitOK {
			t.Fatalf("help %s: exit %d", cmd, code)
		}
		if !strings.Contains(out.String(), "usage: agentrunner "+cmd) {
			t.Errorf("help %s: stdout missing that command's usage:\n%s", cmd, out.String())
		}
	}
	// Unknown command → global help (not an error, not silence).
	var out, errOut bytes.Buffer
	if code := Run([]string{"help", "nosuchcmd"}, "test", &out, &errOut); code != ExitOK {
		t.Fatalf("help nosuchcmd: exit %d", code)
	}
	if !strings.Contains(out.String(), "declarative LLM agents") {
		t.Errorf("help nosuchcmd should fall back to global help:\n%s", out.String())
	}
}
