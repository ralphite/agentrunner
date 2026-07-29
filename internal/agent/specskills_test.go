package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSpecSkill(t *testing.T, dir, name, content string) string {
	t.Helper()
	d := filepath.Join(dir, name)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return d
}

func writeSpecFile(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "spec.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadSpecMaterializesSkillPaths(t *testing.T) {
	dir := t.TempDir()
	writeSpecSkill(t, dir, "release-ritual", "---\nname: release-ritual\ndescription: cut a release\n---\nSteps.\n")
	p := writeSpecFile(t, dir, "name: rel\nsystem_prompt: hi\ntools: []\nskills: [create-agent, ./release-ritual]\n")

	spec, err := LoadSpec(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Skills) != 2 || spec.Skills[0] != "create-agent" {
		t.Fatalf("skills = %v", spec.Skills)
	}
	if !filepath.IsAbs(spec.Skills[1]) || filepath.Base(spec.Skills[1]) != "release-ritual" {
		t.Fatalf("path entry not materialized: %v", spec.Skills)
	}

	// The runtime shapes: directory entry (frontmatter name) + body map.
	entries := specSkillEntries(spec.Skills)
	if len(entries) != 1 || entries[0].Name != "release-ritual" ||
		filepath.Base(entries[0].Path) != "SKILL.md" {
		t.Fatalf("entries = %+v", entries)
	}
	paths := specSkillPaths(spec.Skills)
	if _, ok := paths["release-ritual"]; !ok {
		t.Fatalf("paths = %v", paths)
	}
}

func TestLoadSpecSkillErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadSpec(writeSpecFile(t, dir, "name: a\nsystem_prompt: hi\ntools: []\nskills: [no-such-shipped]\n")); err == nil ||
		!strings.Contains(err.Error(), "unknown shipped skill") {
		t.Fatalf("unknown shipped name: err = %v", err)
	}
	if _, err := LoadSpec(writeSpecFile(t, dir, "name: a\nsystem_prompt: hi\ntools: []\nskills: [./missing-dir]\n")); err == nil ||
		!strings.Contains(err.Error(), "SKILL.md") {
		t.Fatalf("missing dir: err = %v", err)
	}
}
