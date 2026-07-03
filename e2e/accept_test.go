package e2e_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The S1 completion criterion itself must be reproducible under go test:
// build the real binary and run the stage-1 acceptance suite through it.
func TestAcceptStage1EndToEnd(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "agentrunner")

	build := exec.Command("go", "build", "-o", bin, "../cmd/agentrunner")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	report := filepath.Join(tmp, "report.json")
	cmd := exec.Command(bin, "accept", "--stage", "1", "--plain", "--report", report)
	cmd.Dir = tmp
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("accept --stage 1 failed: %v\n%s", err, out)
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
	if rep.Fail != 0 || rep.Aborted != 0 || rep.Pass < 4 {
		t.Fatalf("report = %+v (output:\n%s)", rep, out)
	}
}
