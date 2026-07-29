package skill

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, dir, content string) {
	t.Helper()
	d := filepath.Join(root, ".claude", "skills", dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// byName indexes a Discover result for assertions that must not depend on
// how many shipped skills the binary carries.
func byName(skills []Skill) map[string]Skill {
	m := make(map[string]Skill, len(skills))
	for _, s := range skills {
		m[s.Name] = s
	}
	return m
}

func TestDiscoverAndRender(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "deploy", "---\nname: deploy\ndescription: ship it safely\n---\nFull instructions here.\n")
	writeSkill(t, root, "review", "---\ndescription: review code\n---\nBody.\n") // name falls back to dir

	skills, err := DiscoverWith(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := byName(skills)
	if m["deploy"].Description != "ship it safely" {
		t.Errorf("deploy = %+v", m["deploy"])
	}
	if _, ok := m["review"]; !ok {
		t.Errorf("name fallback to directory failed: %+v", skills)
	}
	if !strings.HasSuffix(m["deploy"].Path, filepath.Join("deploy", "SKILL.md")) ||
		filepath.IsAbs(m["deploy"].Path) {
		t.Errorf("path should be workspace-relative: %q", m["deploy"].Path)
	}
	if !sort.SliceIsSorted(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name }) {
		t.Errorf("directory not sorted: %+v", skills)
	}

	dir := RenderDirectory(skills)
	for _, want := range []string{"<skills>", "deploy: ship it safely", "review", "</skills>"} {
		if !strings.Contains(dir, want) {
			t.Errorf("directory missing %q:\n%s", want, dir)
		}
	}
	// The BODY must not leak into the directory (on-demand loading, S5.2).
	if strings.Contains(dir, "Full instructions") {
		t.Errorf("skill body leaked into the prefix directory:\n%s", dir)
	}
}

func TestDiscoverNoSkillsDirYieldsShippedLayer(t *testing.T) {
	skills, err := DiscoverWith(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	m := byName(skills)
	ca, ok := m["create-agent"]
	if !ok {
		t.Fatalf("shipped create-agent missing without a workspace skills dir: %+v", skills)
	}
	if ca.Path != "builtin:create-agent" || ca.Description == "" {
		t.Errorf("shipped entry = %+v", ca)
	}
}

func TestWorkspaceSkillShadowsShipped(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "create-agent", "---\nname: create-agent\ndescription: workspace override\n---\nLocal body.\n")

	skills, err := DiscoverWith(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	var hits []Skill
	for _, s := range skills {
		if s.Name == "create-agent" {
			hits = append(hits, s)
		}
	}
	if len(hits) != 1 || hits[0].Description != "workspace override" ||
		strings.HasPrefix(hits[0].Path, "builtin:") {
		t.Fatalf("workspace must shadow shipped exactly once: %+v", hits)
	}
}

func TestBuiltinRaw(t *testing.T) {
	raw, ok := BuiltinRaw("create-agent")
	if !ok || !strings.Contains(string(raw), "save_agent") {
		t.Fatalf("BuiltinRaw(create-agent) = ok=%v", ok)
	}
	if _, ok := BuiltinRaw("no-such-skill"); ok {
		t.Fatal("unknown shipped skill must miss")
	}
}

func TestDiscoverMalformedSkillSkipped(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "good", "---\nname: good\ndescription: fine\n---\n")
	writeSkill(t, root, "bad", "no frontmatter at all")

	skills, err := DiscoverWith(root, nil)
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Errorf("err = %v, want malformed listing 'bad'", err)
	}
	m := byName(skills)
	if _, ok := m["good"]; !ok {
		t.Errorf("well-formed skill should survive: %+v", skills)
	}
	if _, ok := m["bad"]; ok {
		t.Errorf("malformed skill should be skipped: %+v", skills)
	}
}

func TestRenderDirectoryEmpty(t *testing.T) {
	if RenderDirectory(nil) != "" {
		t.Error("no skills must render no block")
	}
}
