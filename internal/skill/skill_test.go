package skill

import (
	"os"
	"path/filepath"
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

func TestDiscoverAndRender(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "deploy", "---\nname: deploy\ndescription: ship it safely\n---\nFull instructions here.\n")
	writeSkill(t, root, "review", "---\ndescription: review code\n---\nBody.\n") // name falls back to dir

	skills, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("skills = %+v", skills)
	}
	// Sorted by name: deploy, review.
	if skills[0].Name != "deploy" || skills[0].Description != "ship it safely" {
		t.Errorf("skills[0] = %+v", skills[0])
	}
	if skills[1].Name != "review" {
		t.Errorf("name fallback to directory failed: %+v", skills[1])
	}
	if !strings.HasSuffix(skills[0].Path, filepath.Join("deploy", "SKILL.md")) ||
		filepath.IsAbs(skills[0].Path) {
		t.Errorf("path should be workspace-relative: %q", skills[0].Path)
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

func TestDiscoverNoSkillsDir(t *testing.T) {
	skills, err := Discover(t.TempDir())
	if err != nil || skills != nil {
		t.Fatalf("missing skills dir must be (nil, nil): %v, %v", skills, err)
	}
}

func TestDiscoverMalformedSkillSkipped(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "good", "---\nname: good\ndescription: fine\n---\n")
	writeSkill(t, root, "bad", "no frontmatter at all")

	skills, err := Discover(root)
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Errorf("err = %v, want malformed listing 'bad'", err)
	}
	if len(skills) != 1 || skills[0].Name != "good" {
		t.Errorf("well-formed skill should survive: %+v", skills)
	}
}

func TestRenderDirectoryEmpty(t *testing.T) {
	if RenderDirectory(nil) != "" {
		t.Error("no skills must render no block")
	}
}
