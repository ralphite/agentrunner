package pipeline

import "strings"

// (adjudication order lives in permission.go: rules first — a deny/ask rule
// must beat the read-only set — then read-only as a no-rule fallback.)

// Command-granularity matching (INC-16, #53). A bash rule must NOT wave
// through a whole compound command because one segment matched: an allow of
// `git *` may not silently permit the `rm -rf x` glued after `&&`. The three
// helpers here split a command into independently-adjudicated segments, strip
// benign wrapper prefixes so a rule still matches under `timeout`/`nice`, and
// recognize a fixed set of read-only commands that are safe without a rule.
//
// SECURITY: every transform here is monotonically TIGHTENING except the
// read-only set (a controlled relaxation over provably-harmless commands).
// The splitter is conservative — when it cannot confidently tokenize, it
// returns the whole command as one segment (fail-safe: stricter, never
// looser). deny/ask rules and the hard floor always precede any relaxation.

// splitCompound breaks a shell command into top-level segments at the
// separators `&&` `||` `;` `|` `|&` `&` and newlines. Separators INSIDE
// single or double quotes are ignored (so `echo "a && b"` is one segment).
// A `\`-escaped quote does not open/close a quote. Empty segments are
// dropped. The result is used only to make matching STRICTER: each segment
// must independently clear the rules.
func splitCompound(command string) []string {
	var segs []string
	var cur strings.Builder
	var quote byte // 0, '\'', or '"'
	flush := func() {
		s := strings.TrimSpace(cur.String())
		if s != "" {
			segs = append(segs, s)
		}
		cur.Reset()
	}
	for i := 0; i < len(command); i++ {
		c := command[i]
		if quote != 0 {
			// Inside single quotes nothing escapes; inside double quotes a
			// backslash escapes the next byte.
			if quote == '"' && c == '\\' && i+1 < len(command) {
				cur.WriteByte(c)
				cur.WriteByte(command[i+1])
				i++
				continue
			}
			cur.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
			cur.WriteByte(c)
		case '\\':
			if i+1 < len(command) {
				cur.WriteByte(c)
				cur.WriteByte(command[i+1])
				i++
			}
		case '\n':
			flush()
		case ';':
			flush()
		case '&':
			// `&&` or a trailing/backgrounding `&` — both are top-level breaks.
			flush()
			if i+1 < len(command) && command[i+1] == '&' {
				i++
			}
		case '|':
			// `||`, `|&`, or a pipe `|` — all break into a new segment.
			flush()
			if i+1 < len(command) && (command[i+1] == '|' || command[i+1] == '&') {
				i++
			}
		default:
			cur.WriteByte(c)
		}
	}
	if quote != 0 {
		// Unbalanced quote: we could not tokenize confidently. Fail safe —
		// treat the ENTIRE command as one segment so a stray separator can
		// never split a dangerous tail into an unmatched (and thus
		// default-adjudicated) segment.
		return []string{strings.TrimSpace(command)}
	}
	flush()
	if len(segs) == 0 {
		return []string{strings.TrimSpace(command)}
	}
	return segs
}

// wrapperPrefixes are benign process wrappers whose presence must not defeat
// a command rule: `timeout 60 npm test` should still match `Bash(npm test)`.
// Each entry says how many leading tokens the wrapper itself consumes BEFORE
// the real command begins (the wrapper word plus its own args). Only fixed,
// well-understood shapes are stripped; anything else is left in place
// (fail-safe: an un-stripped wrapper just makes the rule match less, never
// more).
var wrapperConsumes = map[string]func(tokens []string) int{
	"time":   func(t []string) int { return 1 },
	"nohup":  func(t []string) int { return 1 },
	"stdbuf": func(t []string) int { return stdbufConsumes(t) },
	"xargs":  func(t []string) int { return xargsConsumes(t) },
	"nice":   func(t []string) int { return niceConsumes(t) },
	"timeout": func(t []string) int {
		// timeout [OPTIONS] DURATION CMD — strip the word, any leading
		// `-`options (with -k/-s taking a separate value), and the DURATION
		// token. Bail (0) if malformed.
		n := 1
		for n < len(t) && strings.HasPrefix(t[n], "-") {
			opt := t[n]
			n++
			// -k/-s (and long forms) take a separate DURATION/SIGNAL value
			// unless it was attached with `=`.
			if (opt == "-k" || opt == "-s" || opt == "--kill-after" || opt == "--signal") &&
				n < len(t) && !strings.HasPrefix(t[n], "-") {
				n++
			}
		}
		if n >= len(t) {
			return 0
		}
		return n + 1 // + the duration
	},
}

func niceConsumes(t []string) int {
	// nice [-n N] or nice [-N] — strip the word plus an optional adjustment.
	n := 1
	if n < len(t) && t[n] == "-n" {
		n += 2
	} else if n < len(t) && strings.HasPrefix(t[n], "-") {
		n++
	}
	return n
}

func stdbufConsumes(t []string) int {
	// stdbuf takes -i/-o/-e options (bare, no attached value form handled
	// conservatively): strip the word plus leading `-`options.
	n := 1
	for n < len(t) && strings.HasPrefix(t[n], "-") {
		n++
	}
	return n
}

func xargsConsumes(t []string) int {
	// Only strip a BARE `xargs` (no flags): with flags its argument handling
	// is too subtle to reason about safely, so leave it for the rule.
	if len(t) >= 2 && strings.HasPrefix(t[1], "-") {
		return 0
	}
	return 1
}

// stripWrappers removes benign wrapper prefixes so the underlying command is
// what the rule sees. It strips at most a few layers (a real command is not
// wrapped a dozen deep) and never strips into emptiness.
func stripWrappers(segment string) string {
	s := strings.TrimSpace(segment)
	for depth := 0; depth < 4; depth++ {
		tokens := strings.Fields(s)
		if len(tokens) == 0 {
			return s
		}
		consume, ok := wrapperConsumes[tokens[0]]
		if !ok {
			return s
		}
		n := consume(tokens)
		if n <= 0 || n >= len(tokens) {
			return s // malformed or nothing left — leave it for the rule
		}
		s = strings.Join(tokens[n:], " ")
	}
	return s
}

// readOnlyCommands are builtins whose invocation cannot mutate state or run
// arbitrary code. A segment whose (wrapper-stripped) command word is one of
// these needs no rule — it is allowed without a prompt. `find` is included
// but guarded: `find -exec`/`-execdir`/`-delete` RUN things, so those forms
// are excluded below.
var readOnlyCommands = map[string]bool{
	"ls": true, "cat": true, "echo": true, "pwd": true, "head": true,
	"tail": true, "wc": true, "which": true, "diff": true, "stat": true,
	"du": true, "cd": true, "grep": true, "find": true, "file": true,
	"basename": true, "dirname": true, "realpath": true, "readlink": true,
	"true": true, "false": true,
}

// isReadOnlyCommand reports whether a single (already wrapper-stripped)
// segment is a provably-safe read-only invocation. It is deliberately strict:
// a redirection (`>`), command substitution, or a find-that-executes
// disqualifies the segment, which then falls through to the normal rules.
func isReadOnlyCommand(segment string) bool {
	s := strings.TrimSpace(segment)
	if s == "" {
		return false
	}
	// Any output redirection or substitution means the segment can write or
	// run code — not read-only. (Splitting already removed pipes; a bare `<`
	// input redirect is still a read, but `>` `>>` `$(` `` ` `` are not.)
	if strings.ContainsAny(s, ">`") || strings.Contains(s, "$(") {
		return false
	}
	tokens := strings.Fields(s)
	if len(tokens) == 0 || !readOnlyCommands[tokens[0]] {
		return false
	}
	if tokens[0] == "find" {
		for _, t := range tokens[1:] {
			switch t {
			case "-exec", "-execdir", "-delete", "-ok", "-okdir", "-fprintf", "-fprint":
				return false
			}
		}
	}
	return true
}
