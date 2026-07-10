package store

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
)

func mustEnv(t *testing.T, turn int) event.Envelope {
	t.Helper()
	env, err := event.New(event.TypeGenerationStarted, &event.GenerationStarted{GenStep: turn})
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
	for _, name := range []string{eventsFile, indexFile, lockFile} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %o, want 0600", name, fi.Mode().Perm())
		}
	}
}

func TestIndexedCursorReadsOnlyTailAndRejectsMismatch(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	s, err := OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 80; i++ {
		if _, err := s.Append(mustEnv(t, i)); err != nil {
			t.Fatal(err)
		}
	}
	offset, hash, ok := EventCursorAt(dir, 60)
	if !ok || offset <= 0 || len(hash) != 64 {
		t.Fatalf("cursor = offset:%d hash:%q ok:%v", offset, hash, ok)
	}
	tail, err := ReadEventsAfter(dir, 60, offset, hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 20 || tail[0].Seq != 61 || tail[19].Seq != 80 {
		t.Fatalf("tail = len:%d %+v", len(tail), tail)
	}
	badHash := strings.Repeat("0", 64)
	if _, err := ReadEventsAfter(dir, 60, offset, badHash); !errors.Is(err, ErrCursorInvalid) {
		t.Fatalf("bad cursor err = %v", err)
	}
	_ = s.Close()
}

func TestCorruptEventIndexRebuildsFromJournal(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	s, err := OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		if _, err := s.Append(mustEnv(t, i)); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.Close()
	if err := os.WriteFile(filepath.Join(dir, indexFile), []byte("broken-index"), 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if reopened.LastSeq() != 5 {
		t.Fatalf("last seq after index rebuild = %d", reopened.LastSeq())
	}
	if _, _, ok := EventCursorAt(dir, 5); !ok {
		t.Fatal("rebuilt index has no cursor for journal tail")
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

// HasLiveWriter underpins the stranded-session detection (T1/T2b): a
// "running" session whose recorded writer is gone is stranded, not running.
func TestHasLiveWriter(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, lockFile)

	// No lock file yet: nothing ever held it.
	if HasLiveWriter(dir) {
		t.Error("absent lock file: want false")
	}

	// This process's pid is alive → a live writer.
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if !HasLiveWriter(dir) {
		t.Error("own pid recorded: want true (alive)")
	}

	// A reaped child's pid is dead → no live writer (the crashed-host case).
	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Run(); err != nil { // Run waits: the process is gone and reaped
		t.Fatal(err)
	}
	deadPID := cmd.Process.Pid
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", deadPID)), 0o600); err != nil {
		t.Fatal(err)
	}
	if HasLiveWriter(dir) {
		t.Errorf("dead pid %d recorded: want false (stranded host)", deadPID)
	}

	// Malformed content never reports a live writer.
	if err := os.WriteFile(lockPath, []byte("not-a-pid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if HasLiveWriter(dir) {
		t.Error("garbage lock content: want false")
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

// A write failure latches the store broken: no later append may glue onto
// a possible torn half-line. Reopen repairs instead.
func TestBrokenLatchAfterWriteFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	s, err := OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Append(mustEnv(t, 1)); err != nil {
		t.Fatal(err)
	}
	// Force the next write to fail by closing the fd behind the store's back.
	_ = s.f.Close()
	if _, err := s.Append(mustEnv(t, 2)); err == nil {
		t.Fatal("append on closed fd must fail")
	}
	if _, err := s.Append(mustEnv(t, 3)); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("err = %v, want broken latch", err)
	}
}

// A cleanly-closed store must not report a live writer even though the
// closing PROCESS is still alive: hosted sessions' lock pid is the
// daemon's, which outlives every individual loop — `stop` left sessions
// showing "running" forever because the pid probe stayed true
// (QA Round4 F-I2).
func TestClosedStoreHasNoLiveWriter(t *testing.T) {
	dir := t.TempDir()
	es, err := OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !HasLiveWriter(dir) {
		t.Fatal("open store must report a live writer")
	}
	if err := es.Close(); err != nil {
		t.Fatal(err)
	}
	if HasLiveWriter(dir) {
		t.Fatal("closed store must not report a live writer")
	}
}
