package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// buildPrompt injects the series memory truncated AT the boundary — an agent
// that lets its own doc grow cannot bloat the next iteration's context.
func TestBuildPromptTruncatesSeriesMemory(t *testing.T) {
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", seriesMemoryMaxBytes+1000)
	if err := os.WriteFile(filepath.Join(root, "SERIES.md"), []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &Driver{
		Spec: &DriverSpec{Prompt: "base prompt", SeriesMemory: "SERIES.md"},
		Exec: &tool.Executor{WS: ws},
	}
	prompt := d.buildPrompt()
	if !strings.HasPrefix(prompt, "base prompt") || !strings.Contains(prompt, "<series-memory") {
		t.Fatalf("prompt = %.80q..., want base + memory block", prompt)
	}
	if !strings.Contains(prompt, "[truncated at") {
		t.Error("oversized memory must carry the truncation marker")
	}
	// The block never exceeds cap + wrapping by more than a small margin.
	if len(prompt) > len("base prompt")+seriesMemoryMaxBytes+300 {
		t.Errorf("prompt = %d bytes — truncation did not hold", len(prompt))
	}

	// No file → no block, base prompt only.
	d.Spec.SeriesMemory = "MISSING.md"
	if got := d.buildPrompt(); got != "base prompt" {
		t.Errorf("missing memory file: prompt = %q, want the bare prompt", got)
	}

	// A path escaping the workspace resolves to nothing — no block.
	d.Spec.SeriesMemory = "../outside.md"
	if got := d.buildPrompt(); got != "base prompt" {
		t.Errorf("escaping path: prompt = %q, want the bare prompt", got)
	}
}
