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
// sessions, handles, approval ids, or barriers.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validID(s string) bool { return s != "" && len(s) <= 200 && idPattern.MatchString(s) }

// specFileName accepts only bare *.yaml / *.yml names for sibling specs.
var specFileName = regexp.MustCompile(`^[A-Za-z0-9._-]+\.ya?ml$`)

// sessionIDLine matches a session id token (e.g. 20260708-230920-task-5913).
var sessionIDLine = regexp.MustCompile(`\b\d{8}-\d{6}-[A-Za-z0-9._-]+\b`)

// parseSessionID pulls the new session's id out of an `ar new`/`ar fork`
// result. The CLI announces it on stderr as `session <id>` (the reply goes to
// stdout); we scan both streams for the id token to stay robust to wording.
func parseSessionID(res arResult) string {
	for _, stream := range []string{res.Stderr, res.Stdout} {
		for _, line := range strings.Split(stream, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "session ") {
				if m := sessionIDLine.FindString(line); m != "" {
					return m
				}
			}
		}
		if m := sessionIDLine.FindString(stream); m != "" {
			return m
		}
	}
	return ""
}

// daemonUnreachable classifies an ar failure: the CLI flags a failed socket
// dial with one of these phrasings ("no daemon running?", "daemon dial:",
// older "is the daemon running"). Any of them means unreachable.
func daemonUnreachable(stderr string) bool {
	return strings.Contains(stderr, "is the daemon running") ||
		strings.Contains(stderr, "no daemon running") ||
		strings.Contains(stderr, "daemon dial:")
}
