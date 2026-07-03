package accept

import (
	"strings"
	"testing"
)

func TestLoadStage1(t *testing.T) {
	scenarios, err := LoadStage(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(scenarios) != 4 {
		t.Fatalf("scenarios = %d, want 4", len(scenarios))
	}
	for _, s := range scenarios {
		if s.ID == "" || s.Title == "" {
			t.Errorf("incomplete scenario: %+v", s)
		}
	}
	if _, err := LoadStage(99); err == nil {
		t.Error("stage 99 should not exist")
	}
}

func TestRunnerFailurePath(t *testing.T) {
	zero := 0
	r := &Runner{Bin: "/bin/true"}
	res := r.Run(Scenario{
		ID: "x", Title: "fails",
		Steps:  []Step{{Run: "echo actual-output"}},
		Expect: []Expect{{ExitCode: &zero}, {OutputContains: "never-there"}},
	})
	if res.Status != StatusFail || !strings.Contains(res.Detail, "never-there") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRunnerSkip(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	r := &Runner{Bin: "/bin/true"}
	res := r.Run(Scenario{
		ID: "x", Title: "live-gated", Requires: []string{"live"},
		Steps:  []Step{{Run: "true"}},
		Expect: []Expect{{OutputContains: "x"}},
	})
	if res.Status != StatusSkipped {
		t.Fatalf("res = %+v", res)
	}
}
