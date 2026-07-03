package accept

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Report is the machine-readable outcome (always written — loop-mode agents
// read it to self-judge).
type Report struct {
	Stage      int       `json:"stage"`
	Results    []Result  `json:"results"`
	Pass       int       `json:"pass"`
	Fail       int       `json:"fail"`
	Skipped    int       `json:"skipped"`
	Aborted    int       `json:"aborted"`
	FinishedAt time.Time `json:"finished_at"`
}

// Green reports whether the stage gate is satisfied: no failures and no
// aborted/unrun scenarios (an aborted TUI run must not read as success).
func (rep Report) Green() bool {
	return rep.Fail == 0 && rep.Aborted == 0 && rep.Pass+rep.Fail+rep.Skipped == len(rep.Results)
}

// BuildReport aggregates results.
func BuildReport(stage int, results []Result) Report {
	rep := Report{Stage: stage, Results: results, FinishedAt: time.Now().UTC()}
	for _, r := range results {
		switch r.Status {
		case StatusPass:
			rep.Pass++
		case StatusFail:
			rep.Fail++
		case StatusSkipped:
			rep.Skipped++
		default:
			rep.Aborted++
		}
	}
	return rep
}

// WriteJSON writes the report file (0644; gitignored).
func (rep Report) WriteJSON(path string) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// RenderPlain prints one line per scenario plus a summary (non-TTY path).
func RenderPlain(w io.Writer, rep Report) {
	for _, r := range rep.Results {
		mark := map[Status]string{StatusPass: "✓", StatusFail: "✗", StatusSkipped: "–", StatusAborted: "!"}[r.Status]
		fmt.Fprintf(w, "%s %-7s %-28s %s (%.1fs)\n", mark, r.Status, r.ID, r.Title, r.Duration.Seconds())
		if r.Status == StatusFail {
			fmt.Fprintf(w, "  %s\n", r.Detail)
		}
		if r.Status == StatusSkipped {
			fmt.Fprintf(w, "  (%s)\n", r.Detail)
		}
	}
	fmt.Fprintf(w, "\nstage %d: %d PASS, %d FAIL, %d SKIPPED, %d ABORTED\n", rep.Stage, rep.Pass, rep.Fail, rep.Skipped, rep.Aborted)
}
