package main

import (
	"testing"
	"time"
)

// Every schedule kind the driver supports gets a phrase — and an unphrasable
// cron degrades to `Cron <expr>` rather than inventing a rhythm.
func TestCadenceOf(t *testing.T) {
	cases := []struct {
		name string
		spec driverSpec
		want string
	}{
		{"immediate default", driverSpec{}, "Runs once"},
		{"immediate explicit", driverSpec{Schedule: schedImmediate}, "Runs once"},
		{"interval minutes", driverSpec{Schedule: schedInterval, Interval: "30m"}, "Every 30m"},
		{"interval hours", driverSpec{Schedule: schedInterval, Interval: "2h"}, "Every 2h"},
		{"interval mixed", driverSpec{Schedule: schedInterval, Interval: "90m"}, "Every 1h30m"},
		{"interval back-to-back", driverSpec{Schedule: schedInterval}, "Continuously"},
		{"cron weekly", driverSpec{Schedule: schedCron, Cron: "0 4 * * 6"}, "Saturdays at 4:00 AM"},
		{"cron weekdays", driverSpec{Schedule: schedCron, Cron: "0 8 * * 1-5"}, "Weekdays at 8:00 AM"},
		{"cron steps", driverSpec{Schedule: schedCron, Cron: "*/15 * * * *"}, "Every 15 minutes"},
		{"cron hours", driverSpec{Schedule: schedCron, Cron: "0 */6 * * *"}, "Every 6 hours"},
		{"cron monthly", driverSpec{Schedule: schedCron, Cron: "0 4 1 * *"}, "Monthly on the 1st at 4:00 AM"},
		{"cron garbage", driverSpec{Schedule: schedCron, Cron: "wat"}, "Cron wat"},
		{"self paced", driverSpec{Schedule: schedSelfPaced}, "Self-paced"},
		{"parallel", driverSpec{Schedule: schedParallel, N: 4}, "Best of 4"},
		{"parallel no n", driverSpec{Schedule: schedParallel}, "Best of N"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cadenceOf(&tc.spec); got != tc.want {
				t.Fatalf("cadenceOf = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNextRunAndCronNext(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 5, 0, 0, time.Local) // Wednesday
	last := now.Add(-12 * time.Minute)

	got, ok := nextRun(&driverSpec{Schedule: schedInterval, Interval: "30m"}, last, now)
	if !ok || !got.Equal(last.Add(30*time.Minute)) {
		t.Fatalf("interval nextRun = %s (ok=%v)", got, ok)
	}
	if _, ok := nextRun(&driverSpec{Schedule: schedInterval, Interval: "30m"}, time.Time{}, now); ok {
		t.Fatal("interval with no iteration yet must be unknowable")
	}
	got, ok = nextRun(&driverSpec{Schedule: schedCron, Cron: "0 4 * * 6"}, last, now)
	if want := time.Date(2026, 7, 11, 4, 0, 0, 0, time.Local); !ok || !got.Equal(want) {
		t.Fatalf("cron nextRun = %s (ok=%v), want %s", got, ok, want)
	}
	for _, s := range []driverSpec{{}, {Schedule: schedParallel, N: 3}, {Schedule: schedSelfPaced}} {
		if _, ok := nextRun(&s, last, now); ok {
			t.Fatalf("nextRun(%+v) must be unknowable", s)
		}
	}
}

// A finished series never advertises a next run, whatever its cron says.
func TestScheduleForFinishedSeries(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 5, 0, 0, time.Local)
	spec := &driverSpec{Schedule: schedCron, Cron: "0 4 * * 6"}

	live := scheduleFor(spec, time.Time{}, true, now)
	if live.NextRunAt == "" {
		t.Fatal("a live cron series must report its next run")
	}
	if live.Cadence != "Saturdays at 4:00 AM" || live.Schedule != schedCron {
		t.Fatalf("live view = %+v", live)
	}
	dead := scheduleFor(spec, time.Time{}, false, now)
	if dead.NextRunAt != "" {
		t.Fatalf("finished series reported nextRunAt = %q", dead.NextRunAt)
	}
	if dead.Cadence != "Saturdays at 4:00 AM" {
		t.Fatalf("finished series lost its cadence: %+v", dead)
	}
	if got := scheduleFor(nil, time.Time{}, true, now); got != (scheduleView{}) {
		t.Fatalf("nil spec = %+v, want empty", got)
	}
}

func TestSpecFromYAML(t *testing.T) {
	spec := specFromYAML(`name: nightly
schedule: cron
cron: "0 4 * * 6"   # weekly deep check
agent_spec: child.yaml
prompt: |
  interval: not-a-field
verifiers:
  - kind: command
    command: ./check.sh
`)
	if spec.Schedule != "cron" || spec.Cron != "0 4 * * 6" {
		t.Fatalf("spec = %+v", spec)
	}
	// An indented `interval:` inside a block scalar is NOT the driver's own.
	if spec.Interval != "" {
		t.Fatalf("interval = %q, want empty", spec.Interval)
	}
	if got := cadenceOf(spec); got != "Saturdays at 4:00 AM" {
		t.Fatalf("cadence = %q", got)
	}
	best := specFromYAML("schedule: parallel\nn: 4\n")
	if got := cadenceOf(best); got != "Best of 4" {
		t.Fatalf("cadence = %q, want Best of 4", got)
	}
	loop := specFromYAML("schedule: interval\ninterval: 30m\n")
	if got := cadenceOf(loop); got != "Every 30m" {
		t.Fatalf("cadence = %q, want Every 30m", got)
	}
}

// The journaled spec is the Go struct marshalled with FIELD names; the newest
// iteration event is the interval anchor.
func TestParseDriverJournal(t *testing.T) {
	journal := `{"seq":1,"type":"driver_started","payload":{"driver_id":"d1","spec_name":"loop","spec":{"Name":"loop","Schedule":"interval","Interval":"30m","Cron":"","N":0}},"ts":"2026-07-08T10:00:00Z"}
{"seq":2,"type":"iteration_scheduled","payload":{"iter":1},"ts":"2026-07-08T10:00:01Z"}
{"seq":3,"type":"iteration_launched","payload":{"iter":1},"ts":"2026-07-08T10:00:02Z"}
not json at all
{"seq":4,"type":"iteration_scheduled","payload":{"iter":2},"ts":"2026-07-08T10:30:01Z"}
`
	info := parseDriverJournal(journal)
	if info.spec == nil || info.spec.Schedule != "interval" || info.spec.Interval != "30m" {
		t.Fatalf("spec = %+v", info.spec)
	}
	want := time.Date(2026, 7, 8, 10, 30, 1, 0, time.UTC)
	if !info.lastIter.Equal(want) {
		t.Fatalf("lastIter = %s, want %s", info.lastIter, want)
	}
	// A non-driver journal yields no spec — the row simply shows no cadence.
	if got := parseDriverJournal(`{"seq":1,"type":"session_started","payload":{}}`); got.spec != nil {
		t.Fatalf("session journal produced a spec: %+v", got.spec)
	}
}

func TestParseDriverRetryInfo(t *testing.T) {
	journal := `{"seq":1,"type":"driver_started","payload":{"spec_name":"nightly","workspace_root":"/work","spec":{"Name":"nightly","Schedule":"interval","Interval":"15m"}}}`
	info, ok := parseDriverRetryInfo(journal)
	if !ok || info.name != "nightly" || info.workspace != "/work" || info.spec == nil || info.spec.Interval != "15m" {
		t.Fatalf("retry info = %+v ok=%v", info, ok)
	}
	if _, ok := parseDriverRetryInfo(`{"type":"session_started","payload":{}}`); ok {
		t.Fatal("conversation journal was classified as a driver")
	}
}

// A drive run's /api/runs row carries the cadence and, while running, the next
// tick anchored on the last iteration the driver announced.
func TestRunViewSchedule(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 5, 0, 0, time.Local)
	r := &run{
		ID: "run1", Kind: "drive", Status: "running", SessionID: "20260708-100000-loop-ab12",
		StartedAt: now.Add(-40 * time.Minute).Format(time.RFC3339),
		spec:      &driverSpec{Schedule: schedInterval, Interval: "30m"},
		lastIter:  now.Add(-10 * time.Minute),
	}
	v := r.view(now)
	if v.Cadence != "Every 30m" || v.Schedule != schedInterval {
		t.Fatalf("view = %+v", v)
	}
	if v.SessionID != "20260708-100000-loop-ab12" {
		t.Fatalf("sessionId = %q", v.SessionID)
	}
	want := now.Add(20 * time.Minute).Format(time.RFC3339)
	if v.NextRunAt != want {
		t.Fatalf("nextRunAt = %q, want %q", v.NextRunAt, want)
	}
	// No iteration announced yet: the run's own start anchors the interval.
	r.lastIter = time.Time{}
	if got := r.view(now).NextRunAt; got != now.Add(20*time.Minute).Format(time.RFC3339) {
		t.Fatalf("nextRunAt without an iteration = %q", got)
	}
	// A finished run has no next tick; a submit run has no spec at all.
	r.Status = "done"
	if got := r.view(now).NextRunAt; got != "" {
		t.Fatalf("finished run nextRunAt = %q", got)
	}
	plain := &run{ID: "run2", Kind: "submit", Status: "running", StartedAt: r.StartedAt}
	if v := plain.view(now); v.Cadence != "" || v.NextRunAt != "" || v.Schedule != "" {
		t.Fatalf("submit run view = %+v, want no schedule", v)
	}
}

// The iteration announcement the driver writes to stderr is what anchors an
// interval cadence — pin the shape we match.
func TestIterationLineMatch(t *testing.T) {
	for _, line := range []string{
		"iteration 1 (20260708-100000-loop-ab12-iter-1)",
		"attempt 2 (20260708-100000-bon-ab12-iter-2) in /tmp/wt",
	} {
		if !iterationLine.MatchString(line) {
			t.Errorf("no match: %q", line)
		}
	}
	for _, line := range []string{
		`{"kind":"text","text":"iteration 1 (fake)"}`,
		"driver 20260708-100000-loop-ab12",
		"",
	} {
		if iterationLine.MatchString(line) {
			t.Errorf("unexpected match: %q", line)
		}
	}
}
