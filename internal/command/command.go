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
// Skills share the surface: a /<name> with no command file falls back to the
// skill of that name (workspace .claude/skills, then the shipped layer), so
// /create-agent works like a command — the SKILL.md body IS the injected
// prompt, Codex's skill-injection semantics. A command file shadows a
// same-named skill (an explicit workspace file outranks bundled content).
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

	"github.com/ralphite/agentrunner/internal/skill"
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
		// No command file → the skill of that name (workspace, then
		// shipped). Unknown names still pass through untouched.
		body, err = os.ReadFile(filepath.Join(root, ".claude", "skills", name, "SKILL.md"))
		if err != nil {
			var ok bool
			if body, ok = skill.BuiltinRaw(name); !ok {
				return text, false
			}
		}
	}
	return expandTemplate(string(body), args), true
}

// expandTemplate turns a command/skill body into the injected prompt:
// frontmatter stripped, $ARGUMENTS substituted (or args appended when the
// body has no placeholder).
func expandTemplate(body, args string) string {
	tmpl := stripFrontmatter(body)
	tmpl = strings.TrimRight(tmpl, "\n")
	if strings.Contains(tmpl, "$ARGUMENTS") {
		return strings.ReplaceAll(tmpl, "$ARGUMENTS", args)
	}
	if args != "" {
		return tmpl + "\n\n" + args
	}
	return tmpl
}

// Discover lists the workspace's custom commands (<root>/.claude/commands/
// *.md) for pickers. Description is the frontmatter `description:` line when
// present. A missing directory is not an error — most workspaces have none.
func Discover(root string) []Command {
	if root == "" {
		return nil
	}
	dir := filepath.Join(root, ".claude", "commands")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Command
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		if e.IsDir() || name == e.Name() || !nameRE.MatchString(name) {
			continue
		}
		c := Command{Name: name, Path: filepath.Join(".claude", "commands", e.Name())}
		if raw, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			c.Description = frontmatterDescription(string(raw))
		}
		out = append(out, c)
	}
	return out
}

// frontmatterDescription pulls a `description:` value out of a leading YAML
// block, best-effort — pickers degrade to name-only rows.
func frontmatterDescription(s string) string {
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return ""
	}
	rest := s[strings.Index(s, "\n")+1:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return ""
	}
	for _, line := range strings.Split(rest[:end], "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "description:"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
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
