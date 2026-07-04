package cron

import (
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestNext(t *testing.T) {
	cases := []struct {
		expr string
		from string
		want string
	}{
		// every minute
		{"* * * * *", "2026-07-04 10:30", "2026-07-04 10:31"},
		// hourly on the hour
		{"0 * * * *", "2026-07-04 10:30", "2026-07-04 11:00"},
		// daily at 03:00
		{"0 3 * * *", "2026-07-04 10:30", "2026-07-05 03:00"},
		// exactly at a boundary: strictly after
		{"0 3 * * *", "2026-07-04 03:00", "2026-07-05 03:00"},
		// every 15 minutes
		{"*/15 * * * *", "2026-07-04 10:31", "2026-07-04 10:45"},
		// range with step
		{"0 9-17/4 * * *", "2026-07-04 10:00", "2026-07-04 13:00"},
		// comma list
		{"0 6,18 * * *", "2026-07-04 10:00", "2026-07-04 18:00"},
		// monthly on the 1st
		{"30 0 1 * *", "2026-07-04 10:00", "2026-08-01 00:30"},
		// weekly on Sunday (2026-07-04 is a Saturday)
		{"0 12 * * 0", "2026-07-04 10:00", "2026-07-05 12:00"},
		// dow=7 normalizes to Sunday
		{"0 12 * * 7", "2026-07-04 10:00", "2026-07-05 12:00"},
		// dom AND dow both restricted → OR (vixie): the 10th or a Monday
		{"0 0 10 * 1", "2026-07-04 10:00", "2026-07-06 00:00"},
		// month restriction
		{"0 0 1 9 *", "2026-07-04 10:00", "2026-09-01 00:00"},
		// Feb 29 exists in 2028 (leap)
		{"0 0 29 2 *", "2026-07-04 10:00", "2028-02-29 00:00"},
	}
	for _, c := range cases {
		t.Run(c.expr+" from "+c.from, func(t *testing.T) {
			s, err := Parse(c.expr)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := s.Next(at(c.from))
			if !ok {
				t.Fatal("Next found nothing")
			}
			if got != at(c.want) {
				t.Errorf("Next = %s, want %s", got.Format("2006-01-02 15:04"), c.want)
			}
		})
	}
}

func TestNextUnsatisfiable(t *testing.T) {
	s, err := Parse("0 0 31 2 *") // Feb 31 never exists
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Next(at("2026-07-04 10:00")); ok {
		t.Fatal("Feb 31 must be unsatisfiable")
	}
}

func TestParseErrors(t *testing.T) {
	bad := []string{
		"",            // empty
		"* * * *",     // 4 fields
		"* * * * * *", // 6 fields
		"60 * * * *",  // minute out of range
		"* 24 * * *",  // hour out of range
		"* * 0 * *",   // dom out of range
		"* * * 13 *",  // month out of range
		"* * * * 8",   // dow out of range
		"*/0 * * * *", // zero step
		"5-1 * * * *", // inverted range
		"a * * * *",   // not a number
		"0 3 * * mon", // names unsupported (documented cut)
	}
	for _, expr := range bad {
		if _, err := Parse(expr); err == nil {
			t.Errorf("Parse(%q) must fail", expr)
		}
	}
}

// "n/step" (vixie): from n to max by step.
func TestNumWithStep(t *testing.T) {
	s, err := Parse("20/15 * * * *") // 20, 35, 50
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Next(at("2026-07-04 10:36"))
	if got != at("2026-07-04 10:50") {
		t.Errorf("Next = %s, want 10:50", got.Format("15:04"))
	}
	got2, _ := s.Next(at("2026-07-04 10:51"))
	if got2 != at("2026-07-04 11:20") {
		t.Errorf("Next = %s, want 11:20", got2.Format("15:04"))
	}
}
