package e2e_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Each stage's completion criterion must be reproducible under go test:
// build the real binary and run its acceptance suite through it.
func TestAcceptStagesEndToEnd(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "agentrunner")

	build := exec.Command("go", "build", "-o", bin, "../cmd/agentrunner")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	for stage, minPass := range map[int]int{1: 4, 2: 4} {
		t.Run(fmt.Sprintf("stage%d", stage), func(t *testing.T) {
			report := filepath.Join(tmp, fmt.Sprintf("report-%d.json", stage))
			cmd := exec.Command(bin, "accept", "--stage", fmt.Sprint(stage), "--plain", "--report", report)
			cmd.Dir = tmp
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("accept --stage %d failed: %v\n%s", stage, err, out)
			}

			raw, err := os.ReadFile(report)
			if err != nil {
				t.Fatal(err)
			}
			var rep struct {
				Pass    int `json:"pass"`
				Fail    int `json:"fail"`
				Aborted int `json:"aborted"`
			}
			if err := json.Unmarshal(raw, &rep); err != nil {
				t.Fatal(err)
			}
			if rep.Fail != 0 || rep.Aborted != 0 || rep.Pass < minPass {
				t.Fatalf("report = %+v (output:\n%s)", rep, out)
			}
		})
	}
}
