package crash

import (
	"reflect"
	"sync"
	"testing"
)

// resetForTest re-arms parsing from the current env.
func resetForTest(t *testing.T, env string) (fired *bool) {
	t.Helper()
	t.Setenv(EnvVar, env)
	parseOnce = sync.Once{}
	armed = nil
	counts = map[string]int{}
	f := false
	exit = func() { f = true }
	t.Cleanup(func() {
		parseOnce = sync.Once{}
		armed = nil
		counts = map[string]int{}
		exit = func() { panic("exit called after test") }
	})
	return &f
}

// The registry is the harness's contract: deleting a point must fail here.
func TestRegistryPinsS2Points(t *testing.T) {
	want := []string{
		"after_blob_before_event", // S5.5
		"after_exec_before_journal",
		"after_journal_input",
		"after_snapshot_write",
		"before_run_end",
		"between_gate_and_resolved", // S3.2
	}
	if got := Points(); !reflect.DeepEqual(got, want) {
		t.Fatalf("points = %v, want %v", got, want)
	}
}

func TestAfterCountsToN(t *testing.T) {
	fired := resetForTest(t, "after:activity_started:2")
	After("activity_started")
	if *fired {
		t.Fatal("fired on 1st, want 2nd")
	}
	After("generation_started") // other types don't count
	if *fired {
		t.Fatal("other event type counted")
	}
	After("activity_started")
	if !*fired {
		t.Fatal("did not fire on 2nd matching append")
	}
}

func TestPointFiresOnlyOnMatch(t *testing.T) {
	fired := resetForTest(t, "point:after_journal_input")
	Point(PointBeforeRunEnd)
	if *fired {
		t.Fatal("wrong point fired")
	}
	Point(PointAfterJournalInput)
	if !*fired {
		t.Fatal("armed point did not fire")
	}
}

func TestUnarmedIsNoop(t *testing.T) {
	fired := resetForTest(t, "")
	Point(PointAfterJournalInput)
	After("session_started")
	if *fired {
		t.Fatal("unarmed process crashed")
	}
}

func TestUnregisteredPointPanics(t *testing.T) {
	resetForTest(t, "")
	defer func() {
		if recover() == nil {
			t.Fatal("unregistered point name must panic")
		}
	}()
	Point("typo_point")
}

func TestMalformedSpecPanics(t *testing.T) {
	for _, bad := range []string{"nonsense", "after:x", "after:x:zero", "point:not_registered"} {
		t.Run(bad, func(t *testing.T) {
			resetForTest(t, bad)
			defer func() {
				if recover() == nil {
					t.Fatalf("%q must panic", bad)
				}
			}()
			After("x")
		})
	}
}

func TestPointCountsToN(t *testing.T) {
	fired := resetForTest(t, "point:after_exec_before_journal:2")
	Point(PointAfterExecBeforeJournal)
	if *fired {
		t.Fatal("fired on 1st hit, want 2nd")
	}
	Point(PointAfterExecBeforeJournal)
	if !*fired {
		t.Fatal("did not fire on 2nd hit")
	}
}
