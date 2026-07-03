package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
)

func mustEnv(t *testing.T, turn int) event.Envelope {
	t.Helper()
	env, err := event.New(event.TypeTurnStarted, &event.TurnStarted{Turn: turn})
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func TestAppendReadRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	s, err := OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	for i := 1; i <= 3; i++ {
		got, err := s.Append(mustEnv(t, i))
		if err != nil {
			t.Fatal(err)
		}
		if got.Seq != int64(i) || got.ID != fmt.Sprintf("evt-%d", i) || got.TS.IsZero() {
			t.Fatalf("appended = %+v", got)
		}
	}

	events, err := ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[2].Seq != 3 {
		t.Fatalf("events = %+v", events)
	}
}

func TestPermissionBits(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	s, err := OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Append(mustEnv(t, 1)); err != nil {
		t.Fatal(err)
	}

	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %o, want 0700", di.Mode().Perm())
	}
	for _, name := range []string{eventsFile, lockFile} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %o, want 0600", name, fi.Mode().Perm())
		}
	}
}

func TestLockConflict(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	s, err := OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	// Second writer (separate fd = separate open file description → flock
	// conflicts even in-process) must fail loudly with the holder pid.
	_, err = OpenEventStore(dir)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
	if want := fmt.Sprintf("held by pid %d", os.Getpid()); !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want %q", err, want)
	}
}

func TestLockReleasedOnClose(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	s, err := OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(mustEnv(t, 1)); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := OpenEventStore(dir)
	if err != nil {
		t.Fatalf("reopen after close: %v", err)
	}
	defer func() { _ = s2.Close() }()
	got, err := s2.Append(mustEnv(t, 2))
	if err != nil {
		t.Fatal(err)
	}
	if got.Seq != 2 {
		t.Errorf("seq after reopen = %d, want 2 (recovered from log)", got.Seq)
	}
}

func TestTornTailTruncatedOnReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	s, err := OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(mustEnv(t, 1)); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate a crash mid-write: partial JSON, no trailing newline.
	f, err := os.OpenFile(filepath.Join(dir, eventsFile), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"seq":2,"type":"turn_st`); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	// Reader skips the torn tail.
	events, err := ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %+v, want the 1 complete event", events)
	}

	// Writer truncates it; the next append reuses seq 2 on a clean line.
	s2, err := OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()
	got, err := s2.Append(mustEnv(t, 2))
	if err != nil {
		t.Fatal(err)
	}
	if got.Seq != 2 {
		t.Errorf("seq = %d, want 2", got.Seq)
	}
	events, err = ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Seq != 2 {
		t.Fatalf("events after repair = %+v", events)
	}
}

func TestCorruptCompleteLineIsError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	bad := "not json at all\n"
	if err := os.WriteFile(filepath.Join(dir, eventsFile), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEvents(dir); err == nil {
		t.Error("reader must reject newline-terminated corruption")
	}
	if _, err := OpenEventStore(dir); err == nil {
		t.Error("writer must reject newline-terminated corruption")
	}
}

func TestConcurrentAppendsAreSerial(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	s, err := OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(turn int) {
			defer wg.Done()
			if _, err := s.Append(mustEnv(t, turn)); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	events, err := ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != n {
		t.Fatalf("events = %d, want %d", len(events), n)
	}
	for i, e := range events {
		if e.Seq != int64(i+1) {
			t.Fatalf("seq at line %d = %d, want %d (monotonic, gapless)", i, e.Seq, i+1)
		}
	}
}
