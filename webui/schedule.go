package main

// The Scheduled page's cadence contract (INC-41 CX-3). A scheduled thing has
// exactly two facts that justify its existence: what rhythm it runs on, and
// when it fires next. Both live in the driver spec — the one webui itself
// materialises for a `drive` run, and the one every driver journals into its
// own driver_started event (readable through `ar events --json`). We project
// them into {schedule, cadence, nextRunAt} for /api/runs and /api/sessions,
// replacing the type/project/last-start trivia the rows used to carry.
//
// Why the cron parsing lives HERE and not in internal/cron (which is what the
// driver actually schedules on): arwebui is a standalone module (webui/go.mod:
// zero dependencies) whose only contract with the system is the public `ar`
// CLI — it cannot import the engine's packages. So this is a presentation-side
// reader of the same five-field dialect, deliberately kept semantically
// identical to internal/cron (bare `*` is the only star for the dom/dow union
// rule; `n/step` means n..max; dom AND dow restricted → OR). It NEVER decides
// when anything runs; it only says what the driver will do. The end of this
// duplication is `ar sessions list --json` growing cadence/next_run fields
// straight off internal/cron — then this file shrinks to reading them.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Schedule kinds (mirrors internal/driver/spec.go).
const (
	schedImmediate = "immediate"
	schedInterval  = "interval"
	schedCron      = "cron"
	schedSelfPaced = "self_paced"
	schedParallel  = "parallel"
)

// driverSpec is the slice of the driver spec that decides a cadence. The
// journaled spec is the Go struct marshalled with its FIELD names (DriverSpec
// carries no json tags), so these names decode it directly.
type driverSpec struct {
	Schedule string
	Interval string
	Cron     string
	N        int
}

func (d *driverSpec) kind() string {
	if d.Schedule == "" {
		return schedImmediate
	}
	return d.Schedule
}

// scheduleView is the wire projection attached to a run / driver session row.
// An empty field means "we cannot know" — the UI must then say so, never guess.
type scheduleView struct {
	Schedule  string `json:"schedule,omitempty"`  // immediate | interval | cron | self_paced | parallel
	Cadence   string `json:"cadence,omitempty"`   // "Every 30m", "Saturdays at 4:00 AM", "Best of 4"
	NextRunAt string `json:"nextRunAt,omitempty"` // RFC3339; only when a future tick is computable
}

// scheduleFor projects a spec (+ the last iteration's start, which anchors an
// interval cadence) into the view. live=false (a finished series) yields no
// next run: a driver that ended will not fire again, whatever its cron says.
func scheduleFor(spec *driverSpec, lastIter time.Time, live bool, now time.Time) scheduleView {
	if spec == nil {
		return scheduleView{}
	}
	v := scheduleView{Schedule: spec.kind(), Cadence: cadenceOf(spec)}
	if live {
		if t, ok := nextRun(spec, lastIter, now); ok {
			v.NextRunAt = t.Format(time.RFC3339)
		}
	}
	return v
}

// cadenceOf renders the spec's schedule as one human phrase. Never empty for a
// real spec: an unparseable cron degrades to `Cron <expr>` rather than
// inventing a rhythm the driver does not run on.
func cadenceOf(s *driverSpec) string {
	switch s.kind() {
	case schedInterval:
		d, err := time.ParseDuration(s.Interval)
		if err != nil || d <= 0 {
			return "Continuously" // empty/zero interval = back-to-back iterations
		}
		return "Every " + humanDuration(d)
	case schedCron:
		return cronPhrase(s.Cron)
	case schedSelfPaced:
		return "Self-paced"
	case schedParallel:
		if s.N >= 2 {
			return fmt.Sprintf("Best of %d", s.N)
		}
		return "Best of N"
	default:
		return "Runs once"
	}
}

// nextRun computes when the series fires next: interval anchors on `last` (the
// most recent iteration's start), cron on `now`. ok=false means "not knowable"
// — an immediate/parallel/self_paced series has no future tick, and an interval
// series with no iteration yet has no anchor.
func nextRun(s *driverSpec, last, now time.Time) (time.Time, bool) {
	switch s.kind() {
	case schedInterval:
		d, err := time.ParseDuration(s.Interval)
		if err != nil || d <= 0 || last.IsZero() {
			return time.Time{}, false
		}
		return last.Add(d), true
	case schedCron:
		return cronNext(s.Cron, now)
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

// ---- cron (five fields: minute hour dom month dow; *, n, a-b, a,b, */n) ----

type cronSchedule struct {
	minute, hour, dom, month, dow map[int]bool
	domStar, dowStar              bool
}

// cronNext returns the first tick strictly after `after` (minute resolution,
// in after's location — a cron phrase means local wall time). ok=false for an
// unparseable expression or one that cannot fire within a year.
func cronNext(expr string, after time.Time) (time.Time, bool) {
	sc, err := parseCron(expr)
	if err != nil {
		return time.Time{}, false
	}
	t := after.Truncate(time.Minute).Add(time.Minute)
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
		return nil, fmt.Errorf("cron %q: want 5 fields, got %d", expr, len(f))
	}
	sc := &cronSchedule{}
	var err error
	if sc.minute, _, err = cronField(f[0], 0, 59); err != nil {
		return nil, err
	}
	if sc.hour, _, err = cronField(f[1], 0, 23); err != nil {
		return nil, err
	}
	if sc.dom, sc.domStar, err = cronField(f[2], 1, 31); err != nil {
		return nil, err
	}
	if sc.month, _, err = cronField(f[3], 1, 12); err != nil {
		return nil, err
	}
	if sc.dow, sc.dowStar, err = cronField(f[4], 0, 7); err != nil {
		return nil, err
	}
	if sc.dow[7] { // 7 and 0 both mean Sunday
		sc.dow[0] = true
		delete(sc.dow, 7)
	}
	return sc, nil
}

// cronField parses one field into its matching set; star reports the bare `*`.
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
		if part != "*" {
			a, b, isRange := strings.Cut(part, "-")
			n, err := strconv.Atoi(strings.TrimSpace(a))
			if err != nil {
				return nil, false, fmt.Errorf("bad value %q", part)
			}
			lo, hi = n, n
			switch {
			case isRange:
				m, err := strconv.Atoi(strings.TrimSpace(b))
				if err != nil {
					return nil, false, fmt.Errorf("bad range %q", part)
				}
				hi = m
			case step > 1:
				hi = max // "n/step" means n..max by step (Vixie cron, as internal/cron parses it)
			}
			if lo < min || hi > max || lo > hi {
				return nil, false, fmt.Errorf("%q out of range %d-%d", part, min, max)
			}
		} else if step == 1 {
			star = true
		}
		for v := lo; v <= hi; v += step {
			set[v] = true
		}
	}
	if len(set) == 0 {
		return nil, false, fmt.Errorf("no values in %q", f)
	}
	return set, star && !strings.Contains(f, ","), nil
}

var dayNames = [7]string{"Sundays", "Mondays", "Tuesdays", "Wednesdays", "Thursdays", "Fridays", "Saturdays"}
var dayAbbrev = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// cronPhrase turns a cron expression into the sentence Codex's Scheduled rows
// carry ("Saturdays at 4:00 AM", "Weekdays at 8:00 AM", "Every 15 minutes").
// Shapes it cannot phrase degrade to `Cron <expr>` — honest, never wrong.
func cronPhrase(expr string) string {
	raw := strings.Join(strings.Fields(strings.TrimSpace(expr)), " ")
	sc, err := parseCron(expr)
	if err != nil {
		return strings.TrimSpace("Cron " + raw)
	}
	fallback := "Cron " + raw
	mins, hours := sortedKeys(sc.minute), sortedKeys(sc.hour)
	monthStar := len(sc.month) == 12
	calendarStar := sc.domStar && sc.dowStar && monthStar

	if calendarStar && len(hours) == 24 {
		if len(mins) == 60 {
			return "Every minute"
		}
		if n, ok := cronStep(mins, 60); ok {
			return fmt.Sprintf("Every %d minutes", n)
		}
		if len(mins) == 1 {
			return fmt.Sprintf("Hourly at :%02d", mins[0])
		}
	}
	if calendarStar && len(mins) == 1 && len(hours) > 1 {
		if n, ok := cronStep(hours, 24); ok && mins[0] == 0 {
			return fmt.Sprintf("Every %d hours", n)
		}
		return fallback
	}
	if len(mins) != 1 || len(hours) != 1 {
		return fallback
	}
	at := " at " + clockPhrase(hours[0], mins[0])

	if sc.domStar && !sc.dowStar && monthStar { // weekly shapes
		days := sortedKeys(sc.dow)
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
		days := sortedKeys(sc.dom)
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

// cronStep reports the stride of an evenly spaced set starting at 0 that wraps
// at `size` (what `*/n` produces), e.g. {0,15,30,45} over 60 → 15.
func cronStep(vals []int, size int) (int, bool) {
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

func sortedKeys(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
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

// ---- spec sources ----

// specFromYAML scrapes the cadence fields off the driver spec webui is about to
// launch. Same line-based approach as yamlName (this module carries no YAML
// dependency); a spec the CLI would reject fails loudly in the run itself, and
// a cadence we cannot read simply goes unshown.
func specFromYAML(src string) *driverSpec {
	spec := &driverSpec{
		Schedule: yamlScalar(src, "schedule"),
		Interval: yamlScalar(src, "interval"),
		Cron:     yamlScalar(src, "cron"),
	}
	spec.N, _ = strconv.Atoi(yamlScalar(src, "n"))
	return spec
}

// yamlScalar reads a top-level `key: value` scalar (unindented lines only, so
// a nested `interval:` under some other block cannot be mistaken for the
// driver's own), stripping quotes and trailing comments.
func yamlScalar(src, key string) string {
	for _, line := range strings.Split(src, "\n") {
		if line != strings.TrimLeft(line, " \t") { // indented: not top level
			continue
		}
		v, ok := strings.CutPrefix(strings.TrimSpace(line), key+":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		return strings.Trim(v, `"'`)
	}
	return ""
}

// ---- driver sessions (the persistent rows on the Scheduled page) ----

// driverInfoTTL bounds how stale a LIVE driver's last-iteration anchor may get
// before we re-read its journal. A finished driver is cached indefinitely: its
// journal cannot change and it has no next run.
const driverInfoTTL = 15 * time.Second

type driverInfo struct {
	spec     *driverSpec
	lastIter time.Time // newest iteration_scheduled/launched event time
	fetched  time.Time
}

// driverCache memoises the journal read behind /api/sessions. The zero value is
// ready to use.
type driverCache struct {
	mu sync.Mutex
	m  map[string]driverInfo
}

func (c *driverCache) get(sid string) (driverInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	info, ok := c.m[sid]
	return info, ok
}

func (c *driverCache) put(sid string, info driverInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]driverInfo{}
	}
	c.m[sid] = info
}

// driverLive reports whether a driver session can still fire another
// iteration. `ar sessions list` reports a terminal reason (satisfied, stalled,
// max_iterations, stopped, child_failed…) or "stranded" for a driver whose host
// process is gone — only a plain "running" one has a next tick.
func driverLive(status string) bool { return status == "running" }

// driverSchedule returns the cadence view for one driver session, reading its
// spec + iteration anchor out of the journal via the public CLI (cached).
func (s *server) driverSchedule(ctx context.Context, sid, status string, now time.Time) scheduleView {
	live := driverLive(status)
	info, ok := s.drivers.get(sid)
	if !ok || (live && now.Sub(info.fetched) > driverInfoTTL) {
		res := s.runAR(ctx, 10*time.Second, "events", sid, "--json")
		if res.Err != nil {
			return scheduleView{}
		}
		fresh := parseDriverJournal(res.Stdout)
		if fresh.spec == nil {
			return scheduleView{}
		}
		fresh.fetched = now
		s.drivers.put(sid, fresh)
		info = fresh
	}
	return scheduleFor(info.spec, info.lastIter, live, now)
}

// parseDriverJournal pulls the spec (driver_started) and the newest iteration
// timestamp (the interval anchor) out of `ar events --json` output.
func parseDriverJournal(stdout string) driverInfo {
	var info driverInfo
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var env struct {
			Type    string    `json:"type"`
			TS      time.Time `json:"ts"`
			Payload struct {
				Spec json.RawMessage `json:"spec"`
			} `json:"payload"`
		}
		if json.Unmarshal([]byte(line), &env) != nil {
			continue
		}
		switch env.Type {
		case "driver_started":
			var spec driverSpec
			if len(env.Payload.Spec) > 0 && json.Unmarshal(env.Payload.Spec, &spec) == nil {
				info.spec = &spec
			}
		case "iteration_scheduled", "iteration_launched":
			if env.TS.After(info.lastIter) {
				info.lastIter = env.TS
			}
		}
	}
	return info
}
