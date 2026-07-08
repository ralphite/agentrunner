package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
)

// EventStore is the S2 source of truth: an append-only JSONL event log for
// one session, exclusive-writer via flock. Readers never take the lock.
type EventStore struct {
	mu     sync.Mutex
	dir    string
	f      *os.File
	lock   *os.File
	seq    int64
	broken bool
	now    func() time.Time
}

const (
	eventsFile = "events.jsonl"
	lockFile   = "lock"
)

// ErrLocked reports a live writer on the session. flock is released by the
// kernel when the holder dies, so a stale lock file never blocks: if the
// pid in the file is dead, the flock itself succeeds and we overwrite.
var ErrLocked = errors.New("session locked")

// OpenEventStore opens (creating if needed, dir 0700 / files 0600) the
// event log under sessionDir and acquires the writer lock. A torn tail
// left by a crash mid-write is truncated: the event was never acked
// (fsync precedes ack), so dropping it is safe.
func OpenEventStore(sessionDir string) (*EventStore, error) {
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return nil, fmt.Errorf("eventstore: %w", err)
	}

	lock, err := acquireLock(filepath.Join(sessionDir, lockFile))
	if err != nil {
		return nil, err
	}

	path := filepath.Join(sessionDir, eventsFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("eventstore: %w", err)
	}
	seq, end, err := scanLog(f)
	if err != nil {
		_ = f.Close()
		_ = lock.Close()
		return nil, err
	}
	if err := f.Truncate(end); err != nil {
		_ = f.Close()
		_ = lock.Close()
		return nil, fmt.Errorf("eventstore: truncate torn tail: %w", err)
	}
	if _, err := f.Seek(end, 0); err != nil {
		_ = f.Close()
		_ = lock.Close()
		return nil, fmt.Errorf("eventstore: %w", err)
	}
	// fsync the directory so the log's dir entry is as durable as its
	// contents — otherwise power loss can vanish an entire acked log.
	if d, derr := os.Open(sessionDir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return &EventStore{dir: sessionDir, f: f, lock: lock, seq: seq, now: time.Now}, nil
}

// Dir returns the session directory this store writes under.
func (s *EventStore) Dir() string { return s.dir }

func acquireLock(path string) (*os.File, error) {
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("eventstore: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		holder, _ := os.ReadFile(path)
		_ = lock.Close()
		pid := strings.TrimSpace(string(holder))
		if pid == "" {
			pid = "unknown"
		}
		return nil, fmt.Errorf("%w: held by pid %s", ErrLocked, pid)
	}
	if err := lock.Truncate(0); err == nil {
		_, _ = fmt.Fprintf(lock, "%d\n", os.Getpid())
	}
	return lock, nil
}

// HasLiveWriter reports whether the process that last acquired a session's
// writer lock is still alive. It is a CONTENTION-FREE liveness probe: it
// reads the pid recorded in the lock file and signals 0 to test existence —
// it NEVER touches the flock, so unlike a try-lock it can never make a real
// writer's OpenEventStore fail with ErrLocked (an observability command must
// not be able to break a live run).
//
// A session whose journal folds to "running" but for which this reports
// false is STRANDED (T1/T2b, 状态撒谎): its host — the daemon, or a
// foreground run/resume — crashed or was restarted, so nothing is advancing
// it even though the last journaled status says otherwise. `resume` re-enters
// and recovers it.
//
// Correctness note: the recorded pid is the last successful opener's, which
// the kernel-released flock guarantees is the current holder for a live
// session and a dead pid for a crashed one. Pid reuse can yield a rare false
// "alive"; that degrades to the pre-existing "shows running" and never breaks
// an operation — which is the whole reason this reads instead of locks.
func HasLiveWriter(sessionDir string) bool {
	raw, err := os.ReadFile(filepath.Join(sessionDir, lockFile))
	if err != nil {
		return false // no lock file: no writer ever held it
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return false // empty/torn during an open, or malformed: treat as none
	}
	// Signal 0 delivers nothing but performs the permission/existence check:
	// nil = alive; EPERM = alive but owned elsewhere (still a live holder);
	// ESRCH = gone.
	err = syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// scanLog returns the last seq and the byte offset just past the last
// complete line. A final segment without a trailing newline is a torn tail.
// A malformed line that IS newline-terminated is corruption — an error.
func scanLog(f *os.File) (lastSeq, end int64, err error) {
	data, err := os.ReadFile(f.Name())
	if err != nil {
		return 0, 0, fmt.Errorf("eventstore: %w", err)
	}
	var off int64
	for len(data) > 0 {
		nl := bytes.IndexByte(data, '\n')
		if nl < 0 {
			break // torn tail — caller truncates to `end`
		}
		line := data[:nl]
		var env event.Envelope
		if uerr := json.Unmarshal(line, &env); uerr != nil {
			return 0, 0, fmt.Errorf("eventstore: corrupt line at offset %d: %w", off, uerr)
		}
		lastSeq = env.Seq
		off += int64(nl) + 1
		end = off
		data = data[nl+1:]
	}
	return lastSeq, end, nil
}

// Append assigns seq/id/ts, writes one line, and fsyncs before returning.
// The returned envelope is the appended fact.
func (s *EventStore) Append(env event.Envelope) (event.Envelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return event.Envelope{}, errors.New("eventstore: closed")
	}
	if s.broken {
		return event.Envelope{}, errors.New("eventstore: broken by earlier write failure")
	}
	s.seq++
	env.Seq = s.seq
	env.ID = event.EventID(s.seq)
	env.TS = s.now().UTC()
	line, err := json.Marshal(env)
	if err != nil {
		s.seq--
		return event.Envelope{}, fmt.Errorf("eventstore: marshal: %w", err)
	}
	// A failed write may leave a torn half-line at the tail; latch the
	// store broken so no later append can glue onto it — the next open
	// repairs the tail instead.
	if _, err := s.f.Write(append(line, '\n')); err != nil {
		s.broken = true
		return event.Envelope{}, fmt.Errorf("eventstore: append: %w", err)
	}
	if err := s.f.Sync(); err != nil {
		s.broken = true
		return event.Envelope{}, fmt.Errorf("eventstore: fsync: %w", err)
	}
	// Crash matrix counting predicate: the fact is durable, the ack is not.
	crash.After(env.Type)
	return env, nil
}

// LastSeq returns the seq of the most recently appended event.
func (s *EventStore) LastSeq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seq
}

// Close releases the writer lock and closes the log.
func (s *EventStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	if s.lock != nil {
		_ = s.lock.Close() // releases the flock
		s.lock = nil
	}
	return err
}

// ReadEvents loads all complete events from a session dir without taking
// the lock. A torn tail is skipped; corruption mid-file is an error.
func ReadEvents(sessionDir string) ([]event.Envelope, error) {
	path := filepath.Join(sessionDir, eventsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("eventstore: %w", err)
	}
	var out []event.Envelope
	var off int64
	for len(data) > 0 {
		nl := bytes.IndexByte(data, '\n')
		if nl < 0 {
			break
		}
		var env event.Envelope
		if uerr := json.Unmarshal(data[:nl], &env); uerr != nil {
			return nil, fmt.Errorf("eventstore: corrupt line at offset %d: %w", off, uerr)
		}
		out = append(out, env)
		off += int64(nl) + 1
		data = data[nl+1:]
	}
	return out, nil
}
