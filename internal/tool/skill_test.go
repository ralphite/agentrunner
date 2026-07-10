package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// skill(name) returns the SKILL.md body with the frontmatter stripped.
func TestSkillToolReturnsBody(t *testing.T) {
	e, root := newExec(t)
	writeSkill(t, root, "deploy",
		"---\nname: deploy\ndescription: deploy the app\n---\nRun `make ship`, then verify.\n")

	res := e.Execute(context.Background(), "skill", json.RawMessage(`{"name":"deploy"}`))
	if res.IsError {
		t.Fatalf("skill(deploy) errored: %s", res.Payload)
	}
	var body string
	if err := json.Unmarshal(res.Payload, &body); err != nil {
		t.Fatalf("payload not a JSON string: %s", res.Payload)
	}
	if !strings.Contains(body, "Run `make ship`, then verify.") {
		t.Errorf("body missing instructions: %q", body)
	}
	if strings.Contains(body, "description: deploy the app") {
		t.Errorf("frontmatter leaked into body: %q", body)
	}
}

// An unknown skill is an error result, not a panic.
func TestSkillToolUnknownName(t *testing.T) {
	e, _ := newExec(t)
	res := e.Execute(context.Background(), "skill", json.RawMessage(`{"name":"nope"}`))
	if !res.IsError {
		t.Errorf("unknown skill should be an error result: %s", res.Payload)
	}
	// empty/missing name too
	if res := e.Execute(context.Background(), "skill", json.RawMessage(`{}`)); !res.IsError {
		t.Error("missing name should error")
	}
}

// Path separators / traversal in the name are refused before any file access.
func TestSkillToolPathTraversalRefused(t *testing.T) {
	e, root := newExec(t)
	// A real skill exists, but traversal names must not reach outside.
	writeSkill(t, root, "ok", "---\nname: ok\n---\nbody\n")
	// Plant a file one level up to prove traversal can't reach it.
	_ = os.WriteFile(filepath.Join(root, "..", "secret.md"), []byte("secret"), 0o644)

	for _, bad := range []string{"../../etc", "..", ".", "a/b", "a/../b", `a\b`, "../secret"} {
		args, _ := json.Marshal(map[string]string{"name": bad})
		res := e.Execute(context.Background(), "skill", json.RawMessage(args))
		if !res.IsError {
			t.Errorf("traversal name %q should be refused, got %s", bad, res.Payload)
		}
	}
}
