package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	mu   sync.Mutex
	f    *os.File
	lock *os.File
	seq  int64
	now  func() time.Time
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
	return &EventStore{f: f, lock: lock, seq: seq, now: time.Now}, nil
}

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
	s.seq++
	env.Seq = s.seq
	env.ID = event.EventID(s.seq)
	env.TS = s.now().UTC()
	line, err := json.Marshal(env)
	if err != nil {
		s.seq--
		return event.Envelope{}, fmt.Errorf("eventstore: marshal: %w", err)
	}
	if _, err := s.f.Write(append(line, '\n')); err != nil {
		return event.Envelope{}, fmt.Errorf("eventstore: append: %w", err)
	}
	if err := s.f.Sync(); err != nil {
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
