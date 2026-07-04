// Package cron is the minimal five-field cron parser (S6 模块③, PLAN: 最小
// 自实现,无依赖). Fields: minute hour day-of-month month day-of-week.
// Supported syntax per field: * , - / and plain numbers — the classic subset.
// Seconds, names (JAN/MON), @yearly macros, and L/W/# extensions are out of
// scope; a spec needing them fails loudly at parse time.
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed five-field cron expression.
type Schedule struct {
	minute, hour, dom, month, dow uint64 // bitmasks
	// domStar/dowStar record whether the field was '*': standard cron
	// matches day-of-month OR day-of-week when BOTH are restricted, and
	// AND-with-star otherwise.
	domStar, dowStar bool
}

// field describes one position's valid range.
type field struct {
	name     string
	min, max int
}

var fields = [5]field{
	{"minute", 0, 59},
	{"hour", 0, 23},
	{"day-of-month", 1, 31},
	{"month", 1, 12},
	{"day-of-week", 0, 6}, // 0 = Sunday; 7 normalizes to 0
}

// Parse compiles a five-field expression.
func Parse(expr string) (*Schedule, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron %q: want 5 fields (minute hour dom month dow), got %d", expr, len(parts))
	}
	var masks [5]uint64
	var stars [5]bool
	for i, part := range parts {
		mask, star, err := parseField(part, fields[i])
		if err != nil {
			return nil, fmt.Errorf("cron %q: %w", expr, err)
		}
		masks[i], stars[i] = mask, star
	}
	return &Schedule{
		minute: masks[0], hour: masks[1], dom: masks[2], month: masks[3], dow: masks[4],
		domStar: stars[2], dowStar: stars[4],
	}, nil
}

// parseField compiles one field into a bitmask. star reports a bare "*"
// (a "*/n" step is NOT a star for the dom/dow union rule).
func parseField(s string, f field) (mask uint64, star bool, err error) {
	if s == "*" {
		return rangeMask(f.min, f.max, 1), true, nil
	}
	for _, part := range strings.Split(s, ",") {
		body, stepStr, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			step, err = strconv.Atoi(stepStr)
			if err != nil || step < 1 {
				return 0, false, fmt.Errorf("field %s: bad step %q", f.name, part)
			}
		}
		lo, hi := f.min, f.max
		switch {
		case body == "*":
			// full range with step
		case strings.Contains(body, "-"):
			loStr, hiStr, _ := strings.Cut(body, "-")
			if lo, err = parseNum(loStr, f); err != nil {
				return 0, false, err
			}
			if hi, err = parseNum(hiStr, f); err != nil {
				return 0, false, err
			}
			if lo > hi {
				return 0, false, fmt.Errorf("field %s: inverted range %q", f.name, part)
			}
		default:
			if lo, err = parseNum(body, f); err != nil {
				return 0, false, err
			}
			if hasStep {
				hi = f.max // "n/step" means n..max by step (vixie cron)
			} else {
				hi = lo
			}
		}
		mask |= rangeMask(lo, hi, step)
	}
	return mask, false, nil
}

func parseNum(s string, f field) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("field %s: bad number %q", f.name, s)
	}
	if f.name == "day-of-week" && n == 7 {
		n = 0 // both 0 and 7 mean Sunday
	}
	if n < f.min || n > f.max {
		return 0, fmt.Errorf("field %s: %d out of range %d-%d", f.name, n, f.min, f.max)
	}
	return n, nil
}

func rangeMask(lo, hi, step int) uint64 {
	var m uint64
	for v := lo; v <= hi; v += step {
		m |= 1 << uint(v)
	}
	return m
}

func (s *Schedule) matches(t time.Time) bool {
	if s.minute&(1<<uint(t.Minute())) == 0 ||
		s.hour&(1<<uint(t.Hour())) == 0 ||
		s.month&(1<<uint(t.Month())) == 0 {
		return false
	}
	domOK := s.dom&(1<<uint(t.Day())) != 0
	dowOK := s.dow&(1<<uint(t.Weekday())) != 0
	// Standard cron: both restricted → OR; otherwise the restricted one
	// (a star always matches) decides.
	if !s.domStar && !s.dowStar {
		return domOK || dowOK
	}
	return domOK && dowOK
}

// lookahead bounds Next's scan. NINE years covers every satisfiable
// five-field expression: the worst reachable gap is Feb 29 across a non-leap
// century year (2096 → 2104 is eight years). Anything unmatched within it is
// unsatisfiable (e.g. "0 0 31 2 *").
const lookahead = 9 * 366 * 24 * time.Hour

// Next returns the first fire time strictly after t, or false when the
// expression can never fire (scan bounded by the lookahead).
func (s *Schedule) Next(t time.Time) (time.Time, bool) {
	// Cron resolves to minutes: start at the next whole minute.
	cur := t.Truncate(time.Minute).Add(time.Minute)
	limit := t.Add(lookahead)
	for !cur.After(limit) {
		if s.matches(cur) {
			return cur, true
		}
		cur = cur.Add(time.Minute)
	}
	return time.Time{}, false
}
