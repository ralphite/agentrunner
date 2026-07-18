package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// INC-71 (G22a) scan criteria: only a mid-turn RUNNING journal with no live
// writer is stranded. Cleanly parked, closed, and driver-stream sessions are
// not the sweep's business.
func TestScanStrandedSessions(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	seed := func(name string, evs ...any) {
		t.Helper()
		dir := filepath.Join(os.Getenv("XDG_DATA_HOME"), "agentrunner", "sessions", name)
		es, err := store.OpenEventStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = es.Close() }()
		for _, pair := range evs {
			p := pair.([2]any)
			env, err := event.New(p[0].(string), p[1])
			if err != nil {
				t.Fatal(err)
			}
			if _, err := es.Append(env); err != nil {
				t.Fatal(err)
			}
		}
	}
	started := &event.SessionStarted{SpecName: "t", SubStateVersions: state.SubStateVersions()}
	seed("s-stranded",
		[2]any{event.TypeSessionStarted, started},
		[2]any{event.TypeInputReceived, &event.InputReceived{Text: "go", Source: "user"}})
	seed("s-parked",
		[2]any{event.TypeSessionStarted, started},
		[2]any{event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitInput}})
	seed("s-closed",
		[2]any{event.TypeSessionStarted, started},
		[2]any{event.TypeSessionClosed, &event.SessionClosed{Reason: "closed", Source: "user"}})
	seed("s-driver",
		[2]any{event.TypeDriverStarted, &event.DriverStarted{SpecName: "loop"}})

	ids, err := scanStrandedSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "s-stranded" {
		t.Fatalf("scan = %v, want exactly [s-stranded]", ids)
	}
}
