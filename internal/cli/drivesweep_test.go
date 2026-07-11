package cli

import (
	"encoding/json"
	"testing"

	"github.com/ralphite/agentrunner/internal/driver"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/store"
)

func appendDriveEv(t *testing.T, es *store.EventStore, typ string, p any) {
	t.Helper()
	env, err := event.New(typ, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := es.Append(env); err != nil {
		t.Fatal(err)
	}
}

// writeDriveJournal lays down a minimal drive stream: a DriverStarted header
// carrying the spec, one completed iteration, and — when ended — a terminal
// DriverCompleted (the drive's explicit-end mark).
func writeDriveJournal(t *testing.T, id, schedule string, ended bool) {
	t.Helper()
	dir, err := runtime.SessionDir(id)
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = es.Close() }()
	specJSON, _ := json.Marshal(&driver.DriverSpec{Name: id, Schedule: schedule, Cron: "0 * * * *"})
	appendDriveEv(t, es, event.TypeDriverStarted, &event.DriverStarted{
		DriverID: id, SpecName: id, Spec: specJSON, FoldVersion: driver.FoldVersion})
	appendDriveEv(t, es, event.TypeIterationScheduled, &event.IterationScheduled{DriverID: id, Iter: 1})
	appendDriveEv(t, es, event.TypeIterationCompleted, &event.IterationCompleted{DriverID: id, Iter: 1, ChildReason: "completed"})
	if ended {
		appendDriveEv(t, es, event.TypeDriverCompleted, &event.DriverCompleted{DriverID: id, Reason: "max_iterations", Iterations: 1})
	}
}

// INC-54 (决策 #30): the drive boot-sweep index includes only LOOP-MODE drives
// still running. A terminal DriverCompleted (the drive's explicit-end mark), a
// goal/immediate schedule (out of the cron scope), and an agent session (a
// SessionStarted header) are all excluded — automatic resume never crosses an
// ended series, and never mistakes a conversation for a drive.
func TestScanDriveSessionsGate(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeDriveJournal(t, "cron-running", driver.ScheduleCron, false)
	writeDriveJournal(t, "interval-running", driver.ScheduleInterval, false)
	writeDriveJournal(t, "selfpaced-running", driver.ScheduleSelfPaced, false)
	writeDriveJournal(t, "cron-ended", driver.ScheduleCron, true)
	writeDriveJournal(t, "goal-running", driver.ScheduleImmediate, false)

	// An agent session (SessionStarted header) must never appear in the index.
	adir, err := runtime.SessionDir("agent-sess")
	if err != nil {
		t.Fatal(err)
	}
	aes, err := store.OpenEventStore(adir)
	if err != nil {
		t.Fatal(err)
	}
	appendDriveEv(t, aes, event.TypeSessionStarted, &event.SessionStarted{SpecName: "a"})
	_ = aes.Close()

	ids, err := scanDriveSessions()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	for _, want := range []string{"cron-running", "interval-running", "selfpaced-running"} {
		if !got[want] {
			t.Errorf("%s (a running loop drive) missing from the sweep index: %v", want, ids)
		}
	}
	for _, bad := range []string{"cron-ended", "goal-running", "agent-sess"} {
		if got[bad] {
			t.Errorf("%s must be excluded from the sweep index, got %v", bad, ids)
		}
	}
	if len(ids) != 3 {
		t.Errorf("scanDriveSessions = %v, want exactly the 3 running loop drives", ids)
	}
}
