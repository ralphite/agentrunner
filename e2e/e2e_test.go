// Package e2e_test drives the full S1 stack — loop, scripted provider,
// tools, event log — against the committed sample repo (PLAN 1.10). The
// sample repo is copied to a temp workspace per test; the checked-in copy
// is never touched.
package e2e_test

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFixFailingTestEndToEnd(t *testing.T) {
	// Sanity: the pristine sample repo really has a failing test.
	repo := t.TempDir()
	copyDir(t, "testdata/samplerepo", repo)
	if out, err := goTest(repo); err == nil {
		t.Fatalf("sample repo tests should fail before the fix:\n%s", out)
	}

	prov, err := scripted.Load("testdata/fixtures/fix-samplerepo.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(filepath.Join(t.TempDir(), "sess"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()

	loop := &agent.Loop{
		Spec: &agent.AgentSpec{
			Name:               "fixer",
			Model:              agent.ModelSpec{Provider: "scripted", ID: "x", MaxTokens: 512},
			SystemPrompt:       "You are a coding agent.",
			Tools:              []string{"read_file", "edit_file", "bash"},
			MaxGenerationSteps: 10,
		},
		Provider:  prov,
		Exec:      &tool.Executor{WS: ws},
		Store:     es,
		SessionID: "e2e-sess",
	}

	res, err := loop.Run(context.Background(), "Fix the failing test in this repo.")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "completed" || res.GenSteps != 4 {
		t.Errorf("result = %+v", res)
	}
	if d, ok := prov.(interface{ Done() error }); ok {
		if err := d.Done(); err != nil {
			t.Error(err)
		}
	}

	// The fix landed…
	src, err := os.ReadFile(filepath.Join(repo, "mathx", "mathx.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "return a + b") {
		t.Errorf("fix not applied:\n%s", src)
	}
	// …and the repo's own tests pass now.
	if out, err := goTest(repo); err != nil {
		t.Errorf("tests still failing after fix:\n%s", out)
	}
}

func goTest(dir string) (string, error) {
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
