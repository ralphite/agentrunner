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
// sessions, handles, approval ids, or barriers. "#" is allowed because a
// broker id-collision suffixes with "#<n>" (apr-eff-tool-call_1_0#2) — the
// old character class rejected it, so a suffixed worker approval 400'd and
// the Team Lead UI had no way to answer it (QA Round4 F-K1). It is still a
// tight class: no path/shell metacharacters reach argv.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9._#-]+$`)

func validID(s string) bool { return s != "" && len(s) <= 200 && idPattern.MatchString(s) }

// specFileName accepts only bare *.yaml / *.yml names for sibling specs.
var specFileName = regexp.MustCompile(`^[A-Za-z0-9._-]+\.ya?ml$`)

// sessionIDLine matches a session id token (e.g. 20260708-230920-task-5913).
var sessionIDLine = regexp.MustCompile(`\b\d{8}-\d{6}-[A-Za-z0-9._-]+\b`)

// parseSessionID pulls the NEW session's id out of an `ar new`/`ar fork`
// result. The authoritative announcement is a `session <id>` line: `ar new`
// prints it on stderr, `ar fork` on stdout (its stderr instead says
// `forked <PARENT> @ <barrier>` — a red herring). So we first scan BOTH
// streams for a `session ` line (that is always the new id), and only fall
// back to a bare id token if neither stream has one.
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
	}
	for _, stream := range []string{res.Stdout, res.Stderr} {
		if m := sessionIDLine.FindString(stream); m != "" {
			return m
		}
	}
	return ""
}

// sessionExists probes whether the daemon actually hosts a session, via a
// side-effect-free `ar ps` (it prints "no session matches" for unknown ids).
func (s *server) sessionExists(ctx context.Context, id string) bool {
	res := s.runAR(ctx, 5*time.Second, "ps", id)
	return !strings.Contains(res.Stderr, "no session matches")
}

// daemonUnreachable classifies an ar failure: the CLI flags a failed socket
// dial with one of these phrasings ("no daemon running?", "daemon dial:",
// older "is the daemon running"). Any of them means unreachable.
func daemonUnreachable(stderr string) bool {
	return strings.Contains(stderr, "is the daemon running") ||
		strings.Contains(stderr, "no daemon running") ||
		strings.Contains(stderr, "daemon dial:")
}
