package cron

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Phrase turns a five-field expression into the sentence a Scheduled row
// carries ("Saturdays at 4:00 AM", "Weekdays at 8:00 AM", "Every 15
// minutes"). Shapes it cannot phrase degrade to `Cron <expr>` — honest,
// never wrong. This is the single authoritative renderer (INC-80/PLAN 3.1):
// webui consumes it via `ar sessions --json`, never re-implements it.
func Phrase(expr string) string {
	raw := strings.Join(strings.Fields(strings.TrimSpace(expr)), " ")
	fallback := strings.TrimSpace("Cron " + raw)
	sc, err := Parse(expr)
	if err != nil {
		return fallback
	}
	mins := maskKeys(sc.minute, 0, 59)
	hours := maskKeys(sc.hour, 0, 23)
	monthStar := len(maskKeys(sc.month, 1, 12)) == 12
	calendarStar := sc.domStar && sc.dowStar && monthStar

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

	if sc.domStar && !sc.dowStar && monthStar { // weekly shapes
		days := maskKeys(sc.dow, 0, 6)
		switch {
		case sameInts(days, []int{1, 2, 3, 4, 5}):
			return "Weekdays" + at
		case sameInts(days, []int{0, 6}):
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
	if sc.dowStar && !sc.domStar && monthStar { // monthly shape
		days := maskKeys(sc.dom, 1, 31)
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

var dayNames = [7]string{"Sundays", "Mondays", "Tuesdays", "Wednesdays", "Thursdays", "Fridays", "Saturdays"}
var dayAbbrev = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// maskKeys lists the set bits of a field mask within [min, max], ascending.
func maskKeys(mask uint64, min, max int) []int {
	out := make([]int, 0, max-min+1)
	for v := min; v <= max; v++ {
		if mask&(1<<uint(v)) != 0 {
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}

// stepOf reports the stride of an evenly spaced set starting at 0 that wraps
// at `size` (what `*/n` produces), e.g. {0,15,30,45} over 60 → 15.
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

func sameInts(a, b []int) bool {
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
