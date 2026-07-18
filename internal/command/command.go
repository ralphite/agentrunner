// Package command implements user-invoked, repo-defined prompt macros
// (custom commands / slash surface, GAPS G21) by the Claude Code convention:
// <root>/.claude/commands/<name>.md holds a prompt template. A user message
// whose first token is /<name> expands INTO the prompt text at INGEST time
// (before journaling) — so the journaled InputReceived carries the expanded
// body, the fold stays pure (decision #3: fold never reads the store), and a
// resume is self-contained. Commands are model-invisible shortcuts: the model
// only ever sees the expanded prompt, never a command tool, so there is no
// prefix injection and no prefix-stability concern.
//
// Trust: the .md body is untrusted repo content (decision #19), but it only
// expands on an explicit user /invoke and injects TEXT (not executable code),
// exactly like memory and skills — so no additional trust gate is needed.
package command

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// nameRE bounds a command name to a safe basename — no path separators or
// dots, so /name can never traverse out of the commands directory.
var nameRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Command is one discovered command: name + optional description + path.
type Command struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Path is the .md location relative to the workspace root.
	Path string `json:"path"`
}

// Expand resolves a leading-slash command macro. When the first token of
// text is /<name> and <root>/.claude/commands/<name>.md exists, it returns
// (body, true) with the command body's $ARGUMENTS replaced by the remaining
// args (or the args appended on a new line if the body has no placeholder).
// Otherwise it returns (text, false) unchanged: non-slash text and unknown
// or malformed /commands pass through untouched (the model then just sees
// the literal text).
func Expand(root, text string) (string, bool) {
	if root == "" {
		return text, false
	}
	trimmed := strings.TrimLeft(text, " \t")
	if !strings.HasPrefix(trimmed, "/") {
		return text, false
	}
	rest := trimmed[1:]
	name, args := rest, ""
	if i := strings.IndexAny(rest, " \t\n"); i >= 0 {
		name, args = rest[:i], strings.TrimSpace(rest[i+1:])
	}
	if !nameRE.MatchString(name) {
		return text, false
	}
	body, err := os.ReadFile(filepath.Join(root, ".claude", "commands", name+".md"))
	if err != nil {
		return text, false
	}
	tmpl := stripFrontmatter(string(body))
	tmpl = strings.TrimRight(tmpl, "\n")
	if strings.Contains(tmpl, "$ARGUMENTS") {
		return strings.ReplaceAll(tmpl, "$ARGUMENTS", args), true
	}
	if args != "" {
		return tmpl + "\n\n" + args, true
	}
	return tmpl, true
}

// stripFrontmatter drops an optional leading YAML block so it does not end up
// in the expanded prompt.
func stripFrontmatter(s string) string {
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return s
	}
	rest := s[strings.Index(s, "\n")+1:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return s
	}
	after := rest[end+len("\n---"):]
	return strings.TrimPrefix(strings.TrimLeft(after, "-"), "\n")
}
