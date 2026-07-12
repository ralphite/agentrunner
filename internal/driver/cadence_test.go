package driver

import (
	"testing"
	"time"
)

func TestCadence(t *testing.T) {
	cases := []struct {
		name string
		spec DriverSpec
		want string
	}{
		{"immediate default", DriverSpec{}, "Runs once"},
		{"immediate explicit", DriverSpec{Schedule: ScheduleImmediate}, "Runs once"},
		{"interval minutes", DriverSpec{Schedule: ScheduleInterval, Interval: "30m"}, "Every 30m"},
		{"interval hours", DriverSpec{Schedule: ScheduleInterval, Interval: "2h"}, "Every 2h"},
		{"interval mixed", DriverSpec{Schedule: ScheduleInterval, Interval: "90m"}, "Every 1h30m"},
		{"interval day", DriverSpec{Schedule: ScheduleInterval, Interval: "24h"}, "Every 1d"},
		{"interval seconds", DriverSpec{Schedule: ScheduleInterval, Interval: "45s"}, "Every 45s"},
		{"interval empty is back-to-back", DriverSpec{Schedule: ScheduleInterval}, "Continuously"},
		{"interval unparseable", DriverSpec{Schedule: ScheduleInterval, Interval: "nonsense"}, "Continuously"},
		{"cron weekly", DriverSpec{Schedule: ScheduleCron, Cron: "0 4 * * 6"}, "Saturdays at 4:00 AM"},
		{"cron weekdays", DriverSpec{Schedule: ScheduleCron, Cron: "0 8 * * 1-5"}, "Weekdays at 8:00 AM"},
		{"cron garbage", DriverSpec{Schedule: ScheduleCron, Cron: "wat"}, "Cron wat"},
		{"self paced", DriverSpec{Schedule: ScheduleSelfPaced}, "Self-paced"},
		{"parallel", DriverSpec{Schedule: ScheduleParallel, N: 4}, "Best of 4"},
		{"parallel no n", DriverSpec{Schedule: ScheduleParallel}, "Best of N"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Cadence(&tc.spec); got != tc.want {
				t.Fatalf("Cadence = %q, want %q", got, tc.want)
			}
		})
	}
	if got := Cadence(nil); got != "" {
		t.Fatalf("Cadence(nil) = %q, want empty", got)
	}
}

func TestScheduleOf(t *testing.T) {
	if got := ScheduleOf(&DriverSpec{}); got != ScheduleImmediate {
		t.Fatalf("empty schedule = %q, want %q", got, ScheduleImmediate)
	}
	if got := ScheduleOf(&DriverSpec{Schedule: ScheduleCron}); got != ScheduleCron {
		t.Fatalf("cron schedule = %q", got)
	}
	if got := ScheduleOf(nil); got != "" {
		t.Fatalf("ScheduleOf(nil) = %q, want empty", got)
	}
}

func TestCronPhrase(t *testing.T) {
	cases := []struct{ expr, want string }{
		{"0 4 * * 6", "Saturdays at 4:00 AM"},
		{"0 16 * * 5", "Fridays at 4:00 PM"},
		{"30 0 * * 0", "Sundays at 12:30 AM"},
		{"0 12 * * *", "Daily at 12:00 PM"},
		{"0 8 * * 1-5", "Weekdays at 8:00 AM"},
		{"0 9 * * 0,6", "Weekends at 9:00 AM"},
		{"0 9 * * 1,3", "Mon, Wed at 9:00 AM"},
		{"0 4 1 * *", "Monthly on the 1st at 4:00 AM"},
		{"0 4 2 * *", "Monthly on the 2nd at 4:00 AM"},
		{"0 4 23 * *", "Monthly on the 23rd at 4:00 AM"},
		{"*/15 * * * *", "Every 15 minutes"},
		{"* * * * *", "Every minute"},
		{"0 */6 * * *", "Every 6 hours"},
		{"15 * * * *", "Hourly at :15"},
		{"0 4 * 3 *", "Cron 0 4 * 3 *"},                              // month-restricted: not phrasable
		{"0 4 * * 1-5,0", "Sun, Mon, Tue, Wed, Thu, Fri at 4:00 AM"}, // range + list, no collective name
		{"0 4 15 * 5", "Cron 0 4 15 * 5"},                            // dom AND dow restricted (OR rule): no phrase
		{"0 4 * *", "Cron 0 4 * *"},                                  // too few fields
		{"99 4 * * *", "Cron 99 4 * * *"},                            // out of range
		{"", "Cron"},
	}
	for _, tc := range cases {
		if got := CronPhrase(tc.expr); got != tc.want {
			t.Errorf("CronPhrase(%q) = %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestCronNext(t *testing.T) {
	// Wednesday 2026-07-08 10:05 local.
	base := time.Date(2026, 7, 8, 10, 5, 0, 0, time.Local)
	cases := []struct {
		expr string
		want time.Time
		ok   bool
	}{
		{"0 4 * * 6", time.Date(2026, 7, 11, 4, 0, 0, 0, time.Local), true},  // next Saturday 04:00
		{"0 4 * * *", time.Date(2026, 7, 9, 4, 0, 0, 0, time.Local), true},   // tomorrow 04:00
		{"0 11 * * *", time.Date(2026, 7, 8, 11, 0, 0, 0, time.Local), true}, // later today
		{"*/15 * * * *", time.Date(2026, 7, 8, 10, 15, 0, 0, time.Local), true},
		{"0 8 * * 1-5", time.Date(2026, 7, 9, 8, 0, 0, 0, time.Local), true}, // Thursday 08:00
		{"0 4 1 * *", time.Date(2026, 8, 1, 4, 0, 0, 0, time.Local), true},   // 1st of next month
		{"nope", time.Time{}, false},
		{"0 4 30 2 *", time.Time{}, false}, // Feb 30 never fires
	}
	for _, tc := range cases {
		got, ok := CronNext(tc.expr, base)
		if ok != tc.ok {
			t.Errorf("CronNext(%q) ok = %v, want %v", tc.expr, ok, tc.ok)
			continue
		}
		if ok && !got.Equal(tc.want) {
			t.Errorf("CronNext(%q) = %s, want %s", tc.expr, got, tc.want)
		}
	}
}

// Vixie cron's OR rule: with BOTH dom and dow restricted, either matches.
func TestCronNextDomDowOr(t *testing.T) {
	base := time.Date(2026, 7, 8, 10, 5, 0, 0, time.Local) // Wednesday
	// 15th of the month OR any Friday, at 04:00 — Friday the 10th comes first.
	got, ok := CronNext("0 4 15 * 5", base)
	if !ok {
		t.Fatal("CronNext: not ok")
	}
	if want := time.Date(2026, 7, 10, 4, 0, 0, 0, time.Local); !got.Equal(want) {
		t.Fatalf("CronNext = %s, want %s", got, want)
	}
}

func TestNextRun(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 5, 0, 0, time.Local)
	last := now.Add(-12 * time.Minute)

	// interval anchors on the last iteration's start.
	got, ok := NextRun(&DriverSpec{Schedule: ScheduleInterval, Interval: "30m"}, last, now)
	if !ok || !got.Equal(last.Add(30*time.Minute)) {
		t.Fatalf("interval NextRun = %s (ok=%v), want %s", got, ok, last.Add(30*time.Minute))
	}
	// No iteration yet = no anchor = no honest answer.
	if _, ok := NextRun(&DriverSpec{Schedule: ScheduleInterval, Interval: "30m"}, time.Time{}, now); ok {
		t.Fatal("interval NextRun without an anchor must not be knowable")
	}
	// cron anchors on now, not on the last iteration.
	got, ok = NextRun(&DriverSpec{Schedule: ScheduleCron, Cron: "0 4 * * 6"}, last, now)
	if !ok || !got.Equal(time.Date(2026, 7, 11, 4, 0, 0, 0, time.Local)) {
		t.Fatalf("cron NextRun = %s (ok=%v)", got, ok)
	}
	// Schedules with no future tick.
	for _, s := range []DriverSpec{
		{},
		{Schedule: ScheduleImmediate},
		{Schedule: ScheduleParallel, N: 3},
		{Schedule: ScheduleSelfPaced},
		{Schedule: ScheduleInterval}, // back-to-back: no timer to report
	} {
		if _, ok := NextRun(&s, last, now); ok {
			t.Fatalf("NextRun(%+v) must be unknowable", s)
		}
	}
	if _, ok := NextRun(nil, last, now); ok {
		t.Fatal("NextRun(nil) must be unknowable")
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "30m"},
		{2 * time.Hour, "2h"},
		{90 * time.Minute, "1h30m"},
		{48 * time.Hour, "2d"},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m30s"},
		{0, "0s"},
	}
	for _, tc := range cases {
		if got := HumanDuration(tc.d); got != tc.want {
			t.Errorf("HumanDuration(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
