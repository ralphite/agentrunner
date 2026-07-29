package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ar slash lists commands and skills for a workspace; shipped skills appear
// even in an empty workspace, and workspace commands/skills join them.
func TestSlashCmdListsCommandsAndSkills(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude", "commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude", "commands", "ship.md"),
		[]byte("---\ndescription: Ship it\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := slashCmd([]string{"--workspace", root, "--json"}, &out, &errb); code != ExitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	var got struct {
		Commands []struct{ Name, Description string } `json:"commands"`
		Skills   []struct{ Name, Source string }      `json:"skills"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out.String())
	}
	if len(got.Commands) != 1 || got.Commands[0].Name != "ship" || got.Commands[0].Description != "Ship it" {
		t.Fatalf("commands = %+v", got.Commands)
	}
	foundShipped := false
	for _, s := range got.Skills {
		if s.Name == "create-agent" && s.Source == "shipped" {
			foundShipped = true
		}
	}
	if !foundShipped {
		t.Fatalf("shipped create-agent missing: %+v\n%s", got.Skills, out.String())
	}
	if strings.Contains(out.String(), "builtin:") {
		t.Fatalf("raw builtin path leaked: %s", out.String())
	}
}
