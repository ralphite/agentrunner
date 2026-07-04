// Package skill discovers agent skills by the Claude Code convention
// (S5.2): <root>/.claude/skills/<name>/SKILL.md with a YAML frontmatter
// block. Only the DIRECTORY (name + description + path) is injected into the
// prompt prefix; the body is loaded on demand by the model via read_file —
// prefix stability and size both depend on bodies staying out.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

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
}

// Discover walks <root>/.claude/skills for SKILL.md files. A missing skills
// directory is not an error — most workspaces have none. Malformed skills
// are skipped with an error listing them (caller decides whether to warn).
func Discover(root string) ([]Skill, error) {
	dir := filepath.Join(root, ".claude", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
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
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(bad) > 0 {
		sort.Strings(bad)
		return out, fmt.Errorf("skills: malformed frontmatter in %s", strings.Join(bad, ", "))
	}
	return out, nil
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
	b.WriteString("<skills>\nAvailable skills (read the file at the given path for full instructions):\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "- %s: %s (%s)\n", s.Name, s.Description, s.Path)
	}
	b.WriteString("</skills>")
	return b.String()
}
