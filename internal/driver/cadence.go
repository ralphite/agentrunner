package driver

import (
	"fmt"
	"time"

	"github.com/ralphite/agentrunner/internal/cron"
)

// Cadence renders the spec's schedule as one human phrase ("Every 30m",
// "Saturdays at 4:00 AM", "Best of 4", "Self-paced", "Runs once"). Never
// empty for a real spec: an unparseable cron degrades to `Cron <expr>`
// rather than inventing a rhythm the driver does not run on. This is the
// single authoritative renderer (PLAN 3.1): surfaces read it via
// `ar sessions --json`, never re-derive it.
func (s *DriverSpec) Cadence() string {
	switch s.schedule() {
	case ScheduleInterval:
		d, err := time.ParseDuration(s.Interval)
		if err != nil || d <= 0 {
			return "Continuously" // empty/zero interval = back-to-back iterations
		}
		return "Every " + humanDuration(d)
	case ScheduleCron:
		return cron.Phrase(s.Cron)
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

// NextRunAt computes when the series fires next: interval anchors on `last`
// (the most recent consumed tick), cron on `now`. ok=false means "not
// knowable" — an immediate/parallel/self_paced series has no future tick,
// and an interval series with no iteration yet has no anchor. Callers give
// no next run for an ended series, whatever its cron says.
func (s *DriverSpec) NextRunAt(last, now time.Time) (time.Time, bool) {
	switch s.schedule() {
	case ScheduleInterval:
		d, err := time.ParseDuration(s.Interval)
		if err != nil || d <= 0 || last.IsZero() {
			return time.Time{}, false
		}
		next := last.Add(d)
		if !next.After(now) {
			missed := now.Sub(next)/d + 1
			next = next.Add(missed * d)
		}
		return next, true
	case ScheduleCron:
		sched, err := cron.Parse(s.Cron)
		if err != nil {
			return time.Time{}, false
		}
		return sched.Next(now)
	default:
		return time.Time{}, false
	}
}

// humanDuration renders a cadence duration compactly: 30m, 2h, 1h30m, 1d.
func humanDuration(d time.Duration) string {
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
		return d.String()
	}
}
