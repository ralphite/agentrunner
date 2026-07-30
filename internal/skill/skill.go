// Package skill discovers agent skills by the Claude Code convention
// (S5.2): <root>/.claude/skills/<name>/SKILL.md with a YAML frontmatter
// block, plus a SHIPPED layer embedded in the binary (builtin/*/SKILL.md) so
// core skills exist in every workspace. A workspace skill shadows a shipped
// one of the same name — the same override rule as the agent catalog. Only
// the DIRECTORY (name + description + path) is injected into the prompt
// prefix; the body is loaded on demand (skill tool, or read_file for
// workspace skills) — prefix stability and size both depend on bodies
// staying out.
package skill

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed builtin/*/SKILL.md
var builtinFS embed.FS

// BuiltinRaw returns the embedded SKILL.md for a shipped skill. Callers that
// read a workspace skill first fall back here on a miss, giving workspace
// files shadow precedence.
func BuiltinRaw(name string) ([]byte, bool) {
	raw, err := builtinFS.ReadFile("builtin/" + name + "/SKILL.md")
	if err != nil {
		return nil, false
	}
	return raw, true
}

// Builtin lists the shipped skills as directory entries. Path is the
// "builtin:<name>" marker (not a readable file path) — the skill tool loads
// these by name.
func Builtin() []Skill {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil
	}
	var out []Skill
	for _, e := range entries {
		raw, ok := BuiltinRaw(e.Name())
		if !ok {
			continue
		}
		fm, err := parseFrontmatter(raw)
		if err != nil {
			continue // a malformed shipped skill is a build defect; never break the prompt
		}
		name := fm.Name
		if name == "" {
			name = e.Name()
		}
		out = append(out, Skill{Name: name, Description: fm.Description, Path: "builtin:" + e.Name()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Skill is one discovered skill: directory-level metadata only.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Path is the SKILL.md location relative to the workspace root, so the
	// model can read the body on demand with the read_file tool.
	Path string `json:"path"`
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// Tools are unlocked for the session once this skill is loaded
	// (progressive disclosure applied to the tool face): they stay out of
	// the advertised set until the model actually loads the skill that
	// teaches them. Names must be registered built-in tools.
	Tools []string `yaml:"tools"`
}

// UnlockedTools parses a SKILL.md and returns the tools its frontmatter
// unlocks. Empty (and nil on malformed input) is the common case.
func UnlockedTools(raw []byte) []string {
	fm, err := parseFrontmatter(raw)
	if err != nil {
		return nil
	}
	return fm.Tools
}

// DiscoverWith walks <root>/.claude/skills for SKILL.md files and merges
// three shadow layers into one directory, nearest context first: workspace
// skills, then extra (an agent spec's bundled skills), then shipped
// builtins. A later layer never displaces an earlier name. A missing skills
// directory is not an error — most workspaces have none. Malformed workspace
// skills are skipped with an error listing them (caller decides whether to
// warn).
func DiscoverWith(root string, extra []Skill) ([]Skill, error) {
	dir := filepath.Join(root, ".claude", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("skills: %w", err)
	}
	var out []Skill
	var bad []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mdPath := filepath.Join(dir, e.Name(), "SKILL.md")
		raw, err := os.ReadFile(mdPath)
		if err != nil {
			continue // a skills/<name>/ without SKILL.md is not a skill
		}
		fm, err := parseFrontmatter(raw)
		if err != nil {
			bad = append(bad, e.Name())
			continue
		}
		name := fm.Name
		if name == "" {
			name = e.Name() // directory name is the fallback identity
		}
		rel, err := filepath.Rel(root, mdPath)
		if err != nil {
			rel = mdPath
		}
		out = append(out, Skill{Name: name, Description: fm.Description, Path: rel})
	}
	seen := make(map[string]bool, len(out))
	for _, s := range out {
		seen[s.Name] = true
	}
	for _, s := range extra {
		if !seen[s.Name] {
			seen[s.Name] = true
			out = append(out, s)
		}
	}
	for _, s := range Builtin() {
		if !seen[s.Name] {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(bad) > 0 {
		sort.Strings(bad)
		return out, fmt.Errorf("skills: malformed frontmatter in %s", strings.Join(bad, ", "))
	}
	return out, nil
}

// FromDir reads one skills directory (a dir containing SKILL.md) into a
// directory entry — the loader for an agent spec's path-based skills. Path
// is the absolute SKILL.md location.
func FromDir(dir string) (Skill, error) {
	mdPath := filepath.Join(dir, "SKILL.md")
	raw, err := os.ReadFile(mdPath)
	if err != nil {
		return Skill{}, fmt.Errorf("skill %s: %w", dir, err)
	}
	fm, err := parseFrontmatter(raw)
	if err != nil {
		return Skill{}, fmt.Errorf("skill %s: %v", mdPath, err)
	}
	name := fm.Name
	if name == "" {
		name = filepath.Base(dir)
	}
	return Skill{Name: name, Description: fm.Description, Path: mdPath}, nil
}

// parseFrontmatter extracts the YAML block between the leading "---" fences.
// A SKILL.md without frontmatter is malformed — the description is what the
// directory injection exists for.
func parseFrontmatter(raw []byte) (frontmatter, error) {
	s := string(raw)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return frontmatter{}, fmt.Errorf("missing frontmatter fence")
	}
	rest := s[strings.Index(s, "\n")+1:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return frontmatter{}, fmt.Errorf("unterminated frontmatter")
	}
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return frontmatter{}, err
	}
	return fm, nil
}

// RenderDirectory renders the skills directory block for the prompt prefix:
// one line per skill, byte-stable (sorted at discovery). Empty input renders
// empty (no block at all).
func RenderDirectory(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<skills>\nAvailable skills (call the skill tool with the name to get full instructions):\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "- %s: %s (%s)\n", s.Name, s.Description, s.Path)
	}
	b.WriteString("</skills>")
	return b.String()
}
