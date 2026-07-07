package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// arResult is one finished ar invocation.
type arResult struct {
	Stdout string
	Stderr string
	Err    error // non-nil on non-zero exit or spawn failure
}

// runAR executes one ar subcommand with a timeout. Direct argv, no shell.
func (s *server) runAR(ctx context.Context, timeout time.Duration, args ...string) arResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.arPath, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("ar %s: timed out after %s", args[0], timeout)
	}
	return arResult{Stdout: out.String(), Stderr: errb.String(), Err: err}
}

// idPattern guards everything we splice into argv positions that name
// sessions, handles, or approval ids.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validID(s string) bool { return s != "" && len(s) <= 200 && idPattern.MatchString(s) }

// specFileName accepts only bare *.yaml / *.yml names for sibling specs.
var specFileName = regexp.MustCompile(`^[A-Za-z0-9._-]+\.ya?ml$`)

// daemonUnreachable classifies an ar failure: the CLI prints
// "(is the daemon running?)" exactly when the socket dial failed.
func daemonUnreachable(stderr string) bool {
	return strings.Contains(stderr, "is the daemon running")
}
