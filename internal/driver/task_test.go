package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// buildTask injects the series memory truncated AT the boundary — an agent
// that lets its own doc grow cannot bloat the next iteration's context.
func TestBuildTaskTruncatesSeriesMemory(t *testing.T) {
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
		Spec: &DriverSpec{Task: "base task", SeriesMemory: "SERIES.md"},
		Exec: &tool.Executor{WS: ws},
	}
	task := d.buildTask()
	if !strings.HasPrefix(task, "base task") || !strings.Contains(task, "<series-memory") {
		t.Fatalf("task = %.80q..., want base + memory block", task)
	}
	if !strings.Contains(task, "[truncated at") {
		t.Error("oversized memory must carry the truncation marker")
	}
	// The block never exceeds cap + wrapping by more than a small margin.
	if len(task) > len("base task")+seriesMemoryMaxBytes+300 {
		t.Errorf("task = %d bytes — truncation did not hold", len(task))
	}

	// No file → no block, base task only.
	d.Spec.SeriesMemory = "MISSING.md"
	if got := d.buildTask(); got != "base task" {
		t.Errorf("missing memory file: task = %q, want the bare task", got)
	}

	// A path escaping the workspace resolves to nothing — no block.
	d.Spec.SeriesMemory = "../outside.md"
	if got := d.buildTask(); got != "base task" {
		t.Errorf("escaping path: task = %q, want the bare task", got)
	}
}
