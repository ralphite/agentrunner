package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// stubDoctorProbe swaps the sandbox probe for this test; the real one
// depends on what the host happens to have installed.
func stubDoctorProbe(t *testing.T, backend string, openErr, restrictedErr error) {
	t.Helper()
	orig := doctorProbe
	doctorProbe = func() (string, error, error) { return backend, openErr, restrictedErr }
	t.Cleanup(func() { doctorProbe = orig })
}

func TestDoctorAllGreen(t *testing.T) {
	stubDoctorProbe(t, "bwrap", nil, nil)
	var out, errOut bytes.Buffer
	code := Run([]string{"doctor"}, "dev", &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, ExitOK, errOut.String())
	}
	for _, want := range []string{"backend: bwrap", "network=all:", "network=none:", "OK", "OS-contained"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stdout missing %q\n%s", want, out.String())
		}
	}
}

func TestDoctorFailurePrintsFixAndExitsNonZero(t *testing.T) {
	probeErr := errors.New(`bubblewrap unavailable: exec: "bwrap": executable file not found in $PATH — fix: install the distro bubblewrap package`)
	stubDoctorProbe(t, "bwrap", probeErr, probeErr)
	var out, errOut bytes.Buffer
	code := Run([]string{"doctor"}, "dev", &out, &errOut)
	if code != ExitRun {
		t.Fatalf("exit code = %d, want %d", code, ExitRun)
	}
	for _, want := range []string{"FAIL", "install the distro bubblewrap package", "fail-closed", "re-run `ar doctor`"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stdout missing %q\n%s", want, out.String())
		}
	}
}

// One mode can fail alone: a network=none probe needs --unshare-net on top
// of the baseline, and the doctor must attribute the failure to that mode.
func TestDoctorSingleModeFailure(t *testing.T) {
	stubDoctorProbe(t, "bwrap", nil, errors.New("bubblewrap probe: exit status 1"))
	var out, errOut bytes.Buffer
	code := Run([]string{"doctor"}, "dev", &out, &errOut)
	if code != ExitRun {
		t.Fatalf("exit code = %d, want %d", code, ExitRun)
	}
	if !strings.Contains(out.String(), "network=all:  OK") {
		t.Fatalf("stdout missing healthy network=all line\n%s", out.String())
	}
	if !strings.Contains(out.String(), "network=none: FAIL") {
		t.Fatalf("stdout missing failing network=none line\n%s", out.String())
	}
}

func TestDoctorUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"doctor", "extra"}, "dev", &out, &errOut); code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut.String(), "usage: agentrunner doctor") {
		t.Fatalf("stderr = %q, want usage text", errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"doctor", "-h"}, "dev", &out, &errOut); code != ExitOK {
		t.Fatalf("-h exit code = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(out.String(), "Preflight this environment") {
		t.Fatalf("-h stdout = %q, want help text", out.String())
	}
}
