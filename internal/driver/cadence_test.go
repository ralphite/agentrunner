package driver

import (
	"testing"
	"time"
)

// Cadence renders one human phrase per schedule shape — ported verbatim from
// the webui mirror this package replaced (PLAN 3.1): the engine is now the
// single authority, `ar sessions --json` carries the result.
func TestCadencePhrases(t *testing.T) {
	cases := []struct {
		spec DriverSpec
		want string
	}{
		{DriverSpec{Schedule: ScheduleInterval, Interval: "30m"}, "Every 30m"},
		{DriverSpec{Schedule: ScheduleInterval, Interval: "90m"}, "Every 1h30m"},
		{DriverSpec{Schedule: ScheduleInterval, Interval: "24h"}, "Every 1d"},
		{DriverSpec{Schedule: ScheduleInterval, Interval: ""}, "Continuously"},
		{DriverSpec{Schedule: ScheduleCron, Cron: "0 4 * * 6"}, "Saturdays at 4:00 AM"},
		{DriverSpec{Schedule: ScheduleCron, Cron: "0 8 * * 1-5"}, "Weekdays at 8:00 AM"},
		{DriverSpec{Schedule: ScheduleCron, Cron: "*/15 * * * *"}, "Every 15 minutes"},
		{DriverSpec{Schedule: ScheduleCron, Cron: "30 12 * * *"}, "Daily at 12:30 PM"},
		{DriverSpec{Schedule: ScheduleCron, Cron: "not a cron"}, "Cron not a cron"},
		{DriverSpec{Schedule: ScheduleSelfPaced}, "Self-paced"},
		{DriverSpec{Schedule: ScheduleParallel, N: 4}, "Best of 4"},
		{DriverSpec{}, "Runs once"},
	}
	for _, tc := range cases {
		if got := tc.spec.Cadence(); got != tc.want {
			t.Errorf("Cadence(%+v) = %q, want %q", tc.spec, got, tc.want)
		}
	}
}

// NextRunAt: interval anchors on the last consumed tick and rolls forward
// past `now`; cron computes the next matching minute; shapes with no future
// tick answer ok=false.
func TestNextRunAt(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 20, 0, 0, time.UTC)

	iv := DriverSpec{Schedule: ScheduleInterval, Interval: "30m"}
	last := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	next, ok := iv.NextRunAt(last, now)
	if !ok || !next.Equal(time.Date(2026, 7, 8, 10, 30, 0, 0, time.UTC)) {
		t.Fatalf("interval next = %s ok=%v, want 10:30", next, ok)
	}
	if _, ok := iv.NextRunAt(time.Time{}, now); ok {
		t.Fatal("interval with no anchor must answer ok=false")
	}

	cr := DriverSpec{Schedule: ScheduleCron, Cron: "0 4 * * 6"}
	next, ok = cr.NextRunAt(time.Time{}, now)
	if !ok || next.Weekday() != time.Saturday || next.Hour() != 4 {
		t.Fatalf("cron next = %s ok=%v, want next Saturday 4:00", next, ok)
	}

	for _, s := range []DriverSpec{{}, {Schedule: ScheduleSelfPaced}, {Schedule: ScheduleParallel, N: 3}} {
		if _, ok := s.NextRunAt(now, now); ok {
			t.Errorf("%q must have no computable next run", s.Schedule)
		}
	}
}
