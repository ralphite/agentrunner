// Package crash is the injection harness for the S2 crash matrix. A
// process armed via AGENTRUNNER_CRASH aborts hard (os.Exit 137, mimicking
// SIGKILL) at the matching point:
//
//	AGENTRUNNER_CRASH=after:<EventType>:<n>  — after the n-th append of that event type
//	AGENTRUNNER_CRASH=point:<name>[:<n>]     — at the n-th hit (default 1st) of a named injection point
//
// Named points form a closed registry: calling Point with an unregistered
// name panics (catches typos), and the registry test pins the expected
// set, so deleting a point — or the call site the matrix exercises —
// fails loudly.
package crash

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	EnvVar   = "AGENTRUNNER_CRASH"
	ExitCode = 137
)

// Named injection points (S2 set + S3 additions).
const (
	PointAfterJournalInput      = "after_journal_input"
	PointAfterExecBeforeJournal = "after_exec_before_journal"
	PointAfterSnapshotWrite     = "after_snapshot_write"
	PointBeforeTerminal         = "before_run_end"
	PointBetweenGateAndResolved = "between_gate_and_resolved" // S3.2
	PointAfterBlobBeforeEvent   = "after_blob_before_event"   // S5.5
)

var registry = map[string]struct{}{
	PointAfterJournalInput:      {},
	PointAfterExecBeforeJournal: {},
	PointAfterSnapshotWrite:     {},
	PointBeforeTerminal:         {},
	PointBetweenGateAndResolved: {},
	PointAfterBlobBeforeEvent:   {},
}

// Points returns the registered point names, sorted.
func Points() []string {
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type spec struct {
	kind string // "after" | "point"
	name string
	n    int
}

var (
	parseOnce sync.Once
	armed     *spec

	mu     sync.Mutex
	counts = map[string]int{}

	// exit is swappable in white-box tests of the counting logic; the
	// harness self-test exercises the real thing in a subprocess.
	exit = func() { os.Exit(ExitCode) }
)

func armedSpec() *spec {
	parseOnce.Do(func() {
		v := os.Getenv(EnvVar)
		if v == "" {
			return
		}
		parts := strings.Split(v, ":")
		switch {
		case len(parts) == 3 && parts[0] == "after":
			n, err := strconv.Atoi(parts[2])
			if err != nil || n < 1 {
				panic(fmt.Sprintf("crash: bad count in %s=%q", EnvVar, v))
			}
			armed = &spec{kind: "after", name: parts[1], n: n}
		case (len(parts) == 2 || len(parts) == 3) && parts[0] == "point":
			if _, ok := registry[parts[1]]; !ok {
				panic(fmt.Sprintf("crash: unregistered point in %s=%q (known: %s)",
					EnvVar, v, strings.Join(Points(), ", ")))
			}
			n := 1
			if len(parts) == 3 {
				var err error
				if n, err = strconv.Atoi(parts[2]); err != nil || n < 1 {
					panic(fmt.Sprintf("crash: bad count in %s=%q", EnvVar, v))
				}
			}
			armed = &spec{kind: "point", name: parts[1], n: n}
		default:
			panic(fmt.Sprintf("crash: malformed %s=%q", EnvVar, v))
		}
	})
	return armed
}

// Point aborts the process if armed with point:<name>. Unregistered names
// panic unconditionally — a typo must never silently never-fire.
func Point(name string) {
	if _, ok := registry[name]; !ok {
		panic("crash: Point called with unregistered name " + name)
	}
	sp := armedSpec()
	if sp == nil || sp.kind != "point" || sp.name != name {
		return
	}
	mu.Lock()
	counts["point:"+name]++
	hit := counts["point:"+name] >= sp.n
	mu.Unlock()
	if hit {
		exit()
	}
}

// After aborts the process once the n-th event of the armed type has been
// appended. Called by the EventStore after a successful fsynced append.
func After(eventType string) {
	sp := armedSpec()
	if sp == nil || sp.kind != "after" || sp.name != eventType {
		return
	}
	mu.Lock()
	counts[eventType]++
	hit := counts[eventType] >= sp.n
	mu.Unlock()
	if hit {
		exit()
	}
}
