package agent

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestLoadSpecErrors(t *testing.T) {
	files, err := filepath.Glob("testdata/spec_errors/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no error cases found")
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".yaml")
		t.Run(name, func(t *testing.T) {
			_, err := LoadSpec(f)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			golden := filepath.Join("testdata", "spec_errors", name+".golden")
			got := err.Error() + "\n"
			if *update {
				if werr := os.WriteFile(golden, []byte(got), 0o644); werr != nil {
					t.Fatal(werr)
				}
			}
			want, rerr := os.ReadFile(golden)
			if rerr != nil {
				t.Fatalf("missing golden (run with -update): %v", rerr)
			}
			if got != string(want) {
				t.Errorf("error mismatch\n got: %q\nwant: %q", got, string(want))
			}
		})
	}
}

func TestLoadSpecValid(t *testing.T) {
	spec, err := LoadSpec("testdata/valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "hello" {
		t.Errorf("name = %q", spec.Name)
	}
	if spec.Model.Provider != "gemini" || spec.Model.ID != "gemini-flash-latest" {
		t.Errorf("model = %+v", spec.Model)
	}
	if spec.MaxTurns != DefaultMaxTurns {
		t.Errorf("max_turns default = %d, want %d", spec.MaxTurns, DefaultMaxTurns)
	}
	if spec.Model.MaxTokens != DefaultMaxTokens {
		t.Errorf("max_tokens default = %d, want %d", spec.Model.MaxTokens, DefaultMaxTokens)
	}
	if len(spec.Tools) != 3 {
		t.Errorf("tools = %v", spec.Tools)
	}
}

func TestLoadSpecPromptFile(t *testing.T) {
	spec, err := LoadSpec("testdata/valid_file.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if want := "You are a test agent.\n"; spec.SystemPrompt != want {
		t.Errorf("resolved prompt = %q, want %q", spec.SystemPrompt, want)
	}
	if spec.SystemPromptFile != "" {
		t.Errorf("SystemPromptFile should be cleared after resolution, got %q", spec.SystemPromptFile)
	}
}

func TestSpecRejectsBypassMode(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/spec.yaml"
	if err := os.WriteFile(path, []byte("name: x\nmodel: {provider: scripted, id: y}\nsystem_prompt: hi\nmode: bypass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSpec(path)
	if err == nil || !strings.Contains(err.Error(), "bypass cannot be set from a spec") {
		t.Fatalf("err = %v, want spec-bypass rejection", err)
	}
}
