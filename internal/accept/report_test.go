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

func TestCheckEventsRequiresTerminalEvent(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(dir+"/"+name, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	good := `{"seq":1,"type":"run_started","ts":"t","payload":{}}
{"seq":2,"type":"run_ended","ts":"t","payload":{}}
`
	truncated := `{"seq":1,"type":"run_started","ts":"t","payload":{}}
{"seq":2,"type":"activity_started","ts":"t","payload":{}}
`
	gapped := `{"seq":1,"type":"run_started","ts":"t","payload":{}}
{"seq":3,"type":"run_ended","ts":"t","payload":{}}
`
	write("good.jsonl", good)
	if msg := checkEvents(dir + "/good.jsonl"); msg != "" {
		t.Errorf("good log rejected: %s", msg)
	}
	write("trunc.jsonl", truncated)
	if msg := checkEvents(dir + "/trunc.jsonl"); !strings.Contains(msg, "run_ended") {
		t.Errorf("truncated log accepted: %q", msg)
	}
	write("gap.jsonl", gapped)
	if msg := checkEvents(dir + "/gap.jsonl"); !strings.Contains(msg, "gapless") {
		t.Errorf("gapped log accepted: %q", msg)
	}
}
