package runtime

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/store"
)

func TestIngestAppendsBeforeReturning(t *testing.T) {
	dir := t.TempDir() + "/sess"
	s, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	env, err := IngestInput(s, "sess-1", "hello", "cli")
	if err != nil {
		t.Fatal(err)
	}
	if env.Seq != 1 || env.ID != "evt-1" || env.CorrelationID != "sess-1" {
		t.Fatalf("appended = %+v", env)
	}
	if env.CausationID == "" {
		t.Error("input fact must be caused by its command id")
	}

	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != event.TypeInputReceived {
		t.Fatalf("events = %+v", events)
	}
	decoded, err := event.DecodePayload(events[0])
	if err != nil {
		t.Fatal(err)
	}
	in := decoded.(*event.InputReceived)
	if in.TurnID == "" || in.ItemID == "" || in.Principal != "local-user" ||
		in.Source != "cli" || in.Trust != "local" || len(in.Content) != 1 {
		t.Fatalf("typed ingress metadata = %+v", in)
	}
}

// The 2.7 crash-matrix scenario: the process is killed immediately after
// the input's fsynced append (counting predicate) — on "resume" the input
// is still in the log. This doubles as the harness skeleton's self-test:
// the subprocess really exits 137 at the armed predicate.
func TestJournalInputsFirstSurvivesCrash(t *testing.T) {
	if os.Getenv("GO_CRASH_HELPER") == "1" {
		s, err := store.OpenEventStore(os.Getenv("CRASH_DIR"))
		if err != nil {
			fmt.Println("helper:", err)
			os.Exit(1)
		}
		_, _ = IngestInput(s, "sess-1", "hello from the past", "cli")
		fmt.Println("UNREACHABLE: predicate did not fire")
		os.Exit(0)
	}

	dir := t.TempDir() + "/sess"
	cmd := exec.Command(os.Args[0], "-test.run=TestJournalInputsFirstSurvivesCrash")
	cmd.Env = append(os.Environ(),
		"GO_CRASH_HELPER=1",
		"CRASH_DIR="+dir,
		crash.EnvVar+"=after:input_received:1",
	)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != crash.ExitCode {
		t.Fatalf("subprocess: err = %v, out = %s (want exit %d)", err, out, crash.ExitCode)
	}

	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != event.TypeInputReceived {
		t.Fatalf("after crash, events = %+v, want the journaled input", events)
	}
	decoded, err := event.DecodePayload(events[0])
	if err != nil {
		t.Fatal(err)
	}
	if in := decoded.(*event.InputReceived); in.Text != "hello from the past" {
		t.Errorf("input = %+v", in)
	}

	// And the store is reopenable — the flock died with the subprocess.
	s, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatalf("resume open: %v", err)
	}
	_ = s.Close()
}

// Named-point injection fires for real (exit 137) in a subprocess.
func TestNamedPointSubprocess(t *testing.T) {
	if os.Getenv("GO_CRASH_HELPER") == "1" {
		crash.Point(crash.PointBeforeCloseMark)
		fmt.Println("UNREACHABLE: point did not fire")
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestNamedPointSubprocess")
	cmd.Env = append(os.Environ(),
		"GO_CRASH_HELPER=1",
		crash.EnvVar+"=point:"+crash.PointBeforeCloseMark,
	)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != crash.ExitCode {
		t.Fatalf("subprocess: err = %v, out = %s (want exit %d)", err, out, crash.ExitCode)
	}
}
