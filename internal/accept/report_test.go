package accept

import (
	"os"
	"strings"
	"testing"
)

func TestReportGreenGates(t *testing.T) {
	pass := Result{ID: "a", Status: StatusPass}
	skip := Result{ID: "b", Status: StatusSkipped}
	fail := Result{ID: "c", Status: StatusFail}
	aborted := Result{ID: "d", Status: StatusAborted}

	cases := []struct {
		name    string
		results []Result
		green   bool
	}{
		{"all pass", []Result{pass, pass}, true},
		{"pass + skip", []Result{pass, skip}, true},
		{"any fail", []Result{pass, fail}, false},
		{"aborted is not green", []Result{pass, aborted}, false},
		{"zero-valued result is not green", []Result{pass, {ID: "e"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := BuildReport(1, tc.results)
			if rep.Green() != tc.green {
				t.Errorf("Green() = %v, want %v (report %+v)", rep.Green(), tc.green, rep)
			}
		})
	}
}

func TestParseScenarioStrictness(t *testing.T) {
	base := `
id: x
title: t
steps: [{run: "true"}]
`
	// Typo'd expect key must be a load error, never a silent pass.
	if _, err := parseScenario("typo.yaml", []byte(base+`
expect:
  - output_contain: "oops"
`)); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("typo'd key: err = %v, want strict decode error", err)
	}

	// An expect with zero assertions set must be rejected.
	if _, err := parseScenario("empty.yaml", []byte(base+`
expect:
  - {}
`)); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("empty expect: err = %v, want exactly-one validation", err)
	}

	// Two assertions in one expect entry must be rejected.
	if _, err := parseScenario("double.yaml", []byte(base+`
expect:
  - exit_code: 0
    output_contains: "x"
`)); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("double expect: err = %v, want exactly-one validation", err)
	}
}

func TestCheckJournalsRequiresTerminalRecord(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(dir+"/"+name, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	good := `{"type":"run_meta","ts":"t","data":{}}
{"type":"run_end","ts":"t","data":{}}
`
	truncated := `{"type":"run_meta","ts":"t","data":{}}
{"type":"tool_call","ts":"t","data":{}}
`
	write("good.jsonl", good)
	if msg := checkJournals(dir + "/good.jsonl"); msg != "" {
		t.Errorf("good journal rejected: %s", msg)
	}
	write("trunc.jsonl", truncated)
	if msg := checkJournals(dir + "/trunc.jsonl"); !strings.Contains(msg, "run_end") {
		t.Errorf("truncated journal accepted: %q", msg)
	}
}
