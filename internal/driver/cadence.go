package driver

// Cadence is the PRESENTATION projection of a driver spec's schedule: the one
// phrase a user needs to know what a scheduled thing does ("Every 30m",
// "Saturdays at 4:00 AM", "Best of 4"), plus the next wall time it will fire.
// Pure functions over DriverSpec — no journal, no clock of their own — so the
// UI (webui's Scheduled page, INC-41 CX-3) and the CLI can render the same
// facts the driver actually schedules on.
//
// The cron dialect is the standard five-field one (minute hour dom month dow)
// with `*`, plain numbers, lists (`1,3`), ranges (`1-5`) and steps (`*/15`,
// `0-30/10`). Day-of-week is 0-6 with Sunday=0 (7 also accepted as Sunday).
// When BOTH dom and dow are restricted, a tick matches EITHER (Vixie cron's
// long-standing OR rule) — matching what the driver's own timer must do.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ScheduleOf returns the spec's effective schedule kind (empty = immediate).
func ScheduleOf(s *DriverSpec) string {
	if s == nil {
		return ""
	}
	return s.schedule()
}

// Cadence renders the spec's schedule as one human phrase. It never returns
// "" for a valid spec: an unparseable cron degrades to `Cron <expr>` rather
// than inventing a rhythm the driver does not run on.
func Cadence(s *DriverSpec) string {
	if s == nil {
		return ""
	}
	switch s.schedule() {
	case ScheduleInterval:
		d, err := s.interval()
		if err != nil || d <= 0 {
			// Empty/zero interval means back-to-back iterations (spec.go).
			return "Continuously"
		}
		return "Every " + HumanDuration(d)
	case ScheduleCron:
		return CronPhrase(s.Cron)
	case ScheduleSelfPaced:
		return "Self-paced"
	case ScheduleParallel:
		if s.N >= 2 {
			return fmt.Sprintf("Best of %d", s.N)
		}
		return "Best of N"
	default:
		return "Runs once"
	}
}

// NextRun computes when the series fires next: interval anchors on `last` (the
// most recent iteration's start), cron on `now`. ok=false means "not knowable"
// — an immediate/parallel/self_paced series has no future tick, and an interval
// series with no iteration yet has no anchor. A caller that has no honest
// answer must show none (never a fabricated time).
func NextRun(s *DriverSpec, last, now time.Time) (time.Time, bool) {
	if s == nil {
		return time.Time{}, false
	}
	switch s.schedule() {
	case ScheduleInterval:
		d, err := s.interval()
		if err != nil || d <= 0 || last.IsZero() {
			return time.Time{}, false
		}
		return last.Add(d), true
	case ScheduleCron:
		return CronNext(s.Cron, now)
	default:
		return time.Time{}, false
	}
}

// HumanDuration renders a cadence duration compactly: 30m, 2h, 1h30m, 1d.
func HumanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d <= 0 {
		return "0s"
	}
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case d%time.Minute == 0:
		if d > time.Hour {
			return fmt.Sprintf("%dh%dm", int(d/time.Hour), int((d%time.Hour)/time.Minute))
		}
		return fmt.Sprintf("%dm", int(d/time.Minute))
	default:
		return d.String() // sub-minute remainder: 45s, 1m30s
	}
}

// ---- cron ----

// cronSchedule is a parsed five-field expression. star records whether the
// field was literally `*` — the dom/dow OR rule needs that distinction (a `*`
// dom is "unrestricted", not "every day matched").
type cronSchedule struct {
	minute, hour, dom, month, dow map[int]bool
	domStar, dowStar              bool
}

// CronNext returns the first tick strictly after `after` (minute resolution,
// in after's location — a cron phrase means local wall time). ok=false for an
// unparseable expression or one that cannot fire within a year (e.g. Feb 30).
func CronNext(expr string, after time.Time) (time.Time, bool) {
	sc, err := parseCron(expr)
	if err != nil {
		return time.Time{}, false
	}
	t := after.Truncate(time.Minute).Add(time.Minute)
	// A year of minutes bounds the search: every satisfiable expression fires
	// at least once per year (Feb 29 aside, which we simply give up on rather
	// than pretend about).
	limit := t.AddDate(1, 0, 0)
	for ; t.Before(limit); t = t.Add(time.Minute) {
		if sc.match(t) {
			return t, true
		}
	}
	return time.Time{}, false
}

func (c *cronSchedule) match(t time.Time) bool {
	if !c.minute[t.Minute()] || !c.hour[t.Hour()] || !c.month[int(t.Month())] {
		return false
	}
	dom, dow := c.dom[t.Day()], c.dow[int(t.Weekday())]
	switch {
	case c.domStar && c.dowStar:
		return true
	case c.domStar:
		return dow
	case c.dowStar:
		return dom
	default:
		return dom || dow // Vixie cron: restricted dom AND dow match on EITHER
	}
}

func parseCron(expr string) (*cronSchedule, error) {
	f := strings.Fields(strings.TrimSpace(expr))
	if len(f) != 5 {
		return nil, fmt.Errorf("cron %q: want 5 fields (minute hour dom month dow), got %d", expr, len(f))
	}
	sc := &cronSchedule{}
	var err error
	if sc.minute, _, err = cronField(f[0], 0, 59); err != nil {
		return nil, fmt.Errorf("cron %q: minute: %w", expr, err)
	}
	if sc.hour, _, err = cronField(f[1], 0, 23); err != nil {
		return nil, fmt.Errorf("cron %q: hour: %w", expr, err)
	}
	if sc.dom, sc.domStar, err = cronField(f[2], 1, 31); err != nil {
		return nil, fmt.Errorf("cron %q: day-of-month: %w", expr, err)
	}
	if sc.month, _, err = cronField(f[3], 1, 12); err != nil {
		return nil, fmt.Errorf("cron %q: month: %w", expr, err)
	}
	if sc.dow, sc.dowStar, err = cronField(f[4], 0, 7); err != nil {
		return nil, fmt.Errorf("cron %q: day-of-week: %w", expr, err)
	}
	if sc.dow[7] { // 7 and 0 both mean Sunday
		sc.dow[0] = true
		delete(sc.dow, 7)
	}
	return sc, nil
}

// cronField parses one field into its matching set; star reports the bare `*`
// (or an equivalent full-range) form.
func cronField(f string, min, max int) (map[int]bool, bool, error) {
	set := map[int]bool{}
	star := false
	for _, part := range strings.Split(f, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false, fmt.Errorf("empty term in %q", f)
		}
		step := 1
		if base, s, found := strings.Cut(part, "/"); found {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 {
				return nil, false, fmt.Errorf("bad step %q", part)
			}
			step, part = n, base
		}
		lo, hi := min, max
		switch {
		case part == "*":
			if step == 1 {
				star = true
			}
		default:
			a, b, isRange := strings.Cut(part, "-")
			n, err := strconv.Atoi(strings.TrimSpace(a))
			if err != nil {
				return nil, false, fmt.Errorf("bad value %q", part)
			}
			lo, hi = n, n
			if isRange {
				m, err := strconv.Atoi(strings.TrimSpace(b))
				if err != nil {
					return nil, false, fmt.Errorf("bad range %q", part)
				}
				hi = m
			}
			if lo < min || hi > max || lo > hi {
				return nil, false, fmt.Errorf("%q out of range %d-%d", part, min, max)
			}
		}
		for v := lo; v <= hi; v += step {
			set[v] = true
		}
	}
	if len(set) == 0 {
		return nil, false, fmt.Errorf("no values in %q", f)
	}
	// A single-term `*` is the only star; `*,5` is a restriction, not a star.
	return set, star && !strings.Contains(f, ","), nil
}

var dayNames = [7]string{"Sundays", "Mondays", "Tuesdays", "Wednesdays", "Thursdays", "Fridays", "Saturdays"}
var dayAbbrev = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// CronPhrase turns a cron expression into the sentence Codex's Scheduled rows
// use ("Saturdays at 4:00 AM", "Weekdays at 8:00 AM", "Every 15 minutes").
// Shapes it cannot phrase degrade to `Cron <expr>` — honest, never wrong.
func CronPhrase(expr string) string {
	raw := strings.Join(strings.Fields(strings.TrimSpace(expr)), " ")
	sc, err := parseCron(expr)
	if err != nil {
		if raw == "" {
			return "Cron"
		}
		return "Cron " + raw
	}
	fallback := "Cron " + raw
	mins, hours := sorted(sc.minute), sorted(sc.hour)
	monthStar := len(sc.month) == 12
	calendarStar := sc.domStar && sc.dowStar && monthStar

	// Sub-hourly rhythms: every minute / every N minutes.
	if calendarStar && len(hours) == 24 {
		if len(mins) == 60 {
			return "Every minute"
		}
		if n, ok := stepOf(mins, 60); ok {
			return fmt.Sprintf("Every %d minutes", n)
		}
		if len(mins) == 1 {
			return fmt.Sprintf("Hourly at :%02d", mins[0])
		}
	}
	// Every N hours (0 */6 * * *).
	if calendarStar && len(mins) == 1 && len(hours) > 1 {
		if n, ok := stepOf(hours, 24); ok && mins[0] == 0 {
			return fmt.Sprintf("Every %d hours", n)
		}
		return fallback
	}
	if len(mins) != 1 || len(hours) != 1 {
		return fallback
	}
	at := " at " + clockPhrase(hours[0], mins[0])

	// Weekly shapes (dow restricted, dom open).
	if sc.domStar && !sc.dowStar && monthStar {
		days := sorted(sc.dow)
		switch {
		case sameDays(days, []int{1, 2, 3, 4, 5}):
			return "Weekdays" + at
		case sameDays(days, []int{0, 6}):
			return "Weekends" + at
		case len(days) == 7:
			return "Daily" + at
		case len(days) == 1:
			return dayNames[days[0]] + at
		default:
			names := make([]string, 0, len(days))
			for _, d := range days {
				names = append(names, dayAbbrev[d])
			}
			return strings.Join(names, ", ") + at
		}
	}
	// Monthly shape (a single day-of-month, dow open).
	if sc.dowStar && !sc.domStar && monthStar {
		days := sorted(sc.dom)
		if len(days) == 1 {
			return fmt.Sprintf("Monthly on the %s%s", ordinal(days[0]), at)
		}
		return fallback
	}
	if calendarStar {
		return "Daily" + at
	}
	return fallback
}

// stepOf reports the stride of an evenly spaced set that starts at 0 and wraps
// at `size` (the shape `*/n` produces), e.g. {0,15,30,45} over 60 → 15.
func stepOf(vals []int, size int) (int, bool) {
	if len(vals) < 2 || vals[0] != 0 || size%len(vals) != 0 {
		return 0, false
	}
	step := size / len(vals)
	for i, v := range vals {
		if v != i*step {
			return 0, false
		}
	}
	return step, true
}

func sorted(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

func sameDays(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// clockPhrase renders a 24h wall time as "4:00 AM" / "12:30 PM".
func clockPhrase(h, m int) string {
	suffix := "AM"
	if h >= 12 {
		suffix = "PM"
	}
	hh := h % 12
	if hh == 0 {
		hh = 12
	}
	return fmt.Sprintf("%d:%02d %s", hh, m, suffix)
}

func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(n) + suffix
}
