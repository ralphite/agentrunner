package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// stdinSource reports whether stdin is a pipe/redirect (not an interactive
// terminal) and hands back the reader. Package-level so tests can simulate
// piped input without wiring a real pipe to the test process.
var stdinSource = func() (io.Reader, bool) {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return nil, false
	}
	return os.Stdin, true
}

// completeTextArg fills a command's trailing text argument from piped stdin,
// the Unix filter convention: `echo task | ar run spec.yaml` or
// `git diff | ar send <sid> -`. want is the command's full positional arity;
// the text is always the last position. Cases:
//   - all positions given, last == "-" → replace it with stdin; on a tty this
//     is an error rather than an ar that silently blocks waiting for EOF
//   - one short of want and stdin is piped → append stdin
//   - anything else → returned untouched, so the caller's usage and
//     non-empty checks stay authoritative
//
// Only trailing newlines are trimmed; the text itself may be multi-line.
func completeTextArg(rest []string, want int) ([]string, error) {
	explicit := len(rest) == want && rest[want-1] == "-"
	implicit := len(rest) == want-1
	if !explicit && !implicit {
		return rest, nil
	}
	r, piped := stdinSource()
	if !piped {
		if explicit {
			return rest, fmt.Errorf(`stdin is not a pipe; "-" reads piped text (echo task | agentrunner ... -) — or pass the text as an argument`)
		}
		return rest, nil
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return rest, fmt.Errorf("reading text from stdin: %w", err)
	}
	text := strings.TrimRight(string(b), "\r\n")
	if text == "" {
		return rest, fmt.Errorf("stdin provided no text")
	}
	out := append([]string{}, rest...)
	if explicit {
		out[want-1] = text
		return out, nil
	}
	return append(out, text), nil
}
