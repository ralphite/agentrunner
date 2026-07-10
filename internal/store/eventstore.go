package store

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	index  *os.File
	lock   *os.File
	seq    int64
	offset int64
	hash   [sha256.Size]byte
	broken bool
	now    func() time.Time
}

const (
	eventsFile = "events.jsonl"
	indexFile  = "events.idx"
	lockFile   = "lock"
	// seq + byte offset + rolling prefix hash. Fixed-width records make a
	// snapshot cursor an O(1) lookup instead of another full-log scan.
	indexRecordSize = 8 + 8 + sha256.Size
)

// ErrCursorInvalid means a snapshot's derived event cursor cannot be
// verified against the current journal/index. Callers must discard the
// snapshot optimization and fold the full source-of-truth log.
var ErrCursorInvalid = errors.New("eventstore: invalid journal cursor")

type eventIndexRecord struct {
	Seq    int64
	Offset int64
	Hash   [sha256.Size]byte
}

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
	index, err := os.OpenFile(filepath.Join(sessionDir, indexFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		_ = f.Close()
		_ = lock.Close()
		return nil, fmt.Errorf("eventstore: index: %w", err)
	}
	seq, end, hash, err := reconcileEventIndex(f, index)
	if err != nil {
		_ = index.Close()
		_ = f.Close()
		_ = lock.Close()
		return nil, err
	}
	if err := f.Truncate(end); err != nil {
		_ = index.Close()
		_ = f.Close()
		_ = lock.Close()
		return nil, fmt.Errorf("eventstore: truncate torn tail: %w", err)
	}
	if _, err := f.Seek(end, 0); err != nil {
		_ = index.Close()
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
	if _, err := index.Seek(0, io.SeekEnd); err != nil {
		_ = index.Close()
		_ = f.Close()
		_ = lock.Close()
		return nil, fmt.Errorf("eventstore: index: %w", err)
	}
	return &EventStore{dir: sessionDir, f: f, index: index, lock: lock,
		seq: seq, offset: end, hash: hash, now: time.Now}, nil
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

// reconcileEventIndex verifies the fixed-width cache's last boundary and
// scans only the unindexed journal tail. A missing/corrupt index is derived
// again from the full log; the event journal remains the sole truth.
func reconcileEventIndex(log, index *os.File) (int64, int64, [sha256.Size]byte, error) {
	if stat, err := index.Stat(); err == nil && stat.Size()%indexRecordSize != 0 {
		// A crash can tear a derived index append. Drop only the incomplete
		// record, then validate the last complete boundary below.
		if err := index.Truncate(stat.Size() - stat.Size()%indexRecordSize); err != nil {
			return 0, 0, [sha256.Size]byte{}, fmt.Errorf("eventstore: index truncate: %w", err)
		}
	}
	count := int64(0)
	if stat, err := index.Stat(); err == nil {
		count = stat.Size() / indexRecordSize
	}
	var last eventIndexRecord
	valid := count == 0
	if count > 0 {
		var err error
		last, err = readIndexRecord(index, count)
		valid = err == nil && last.Seq == count && validateLastIndexedLine(log, index, count, last)
	}
	if !valid {
		if err := index.Truncate(0); err != nil {
			return 0, 0, [sha256.Size]byte{}, fmt.Errorf("eventstore: index rebuild: %w", err)
		}
		last, count = eventIndexRecord{}, 0
	}
	if _, err := log.Seek(last.Offset, io.SeekStart); err != nil {
		return 0, 0, [sha256.Size]byte{}, fmt.Errorf("eventstore: %w", err)
	}
	if _, err := index.Seek(count*indexRecordSize, io.SeekStart); err != nil {
		return 0, 0, [sha256.Size]byte{}, fmt.Errorf("eventstore: index: %w", err)
	}
	seq, end, hash, err := scanLogTail(log, last.Seq, last.Offset, last.Hash, func(rec eventIndexRecord) error {
		return appendIndexRecord(index, rec, false)
	})
	if err != nil {
		return 0, 0, [sha256.Size]byte{}, err
	}
	if err := index.Sync(); err != nil {
		return 0, 0, [sha256.Size]byte{}, fmt.Errorf("eventstore: index fsync: %w", err)
	}
	return seq, end, hash, nil
}

func validateLastIndexedLine(log, index *os.File, count int64, last eventIndexRecord) bool {
	var prev eventIndexRecord
	if count > 1 {
		var err error
		prev, err = readIndexRecord(index, count-1)
		if err != nil || prev.Seq != count-1 {
			return false
		}
	}
	if last.Offset <= prev.Offset {
		return false
	}
	stat, err := log.Stat()
	if err != nil || last.Offset > stat.Size() || last.Offset-prev.Offset > 32*1024*1024 {
		return false
	}
	line := make([]byte, last.Offset-prev.Offset)
	if _, err := log.ReadAt(line, prev.Offset); err != nil || len(line) == 0 || line[len(line)-1] != '\n' {
		return false
	}
	var env event.Envelope
	if json.Unmarshal(line[:len(line)-1], &env) != nil || env.Seq != last.Seq {
		return false
	}
	return rollEventHash(prev.Hash, line) == last.Hash
}

// scanLogTail streams complete newline-terminated events starting at start.
// A partial final segment is a torn tail and is excluded from end.
func scanLogTail(log *os.File, lastSeq, start int64, hash [sha256.Size]byte,
	onRecord func(eventIndexRecord) error) (int64, int64, [sha256.Size]byte, error) {
	reader := bufio.NewReaderSize(log, 64*1024)
	offset := start
	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) && len(line) > 0 {
			return lastSeq, offset, hash, nil // torn tail
		}
		if errors.Is(err, io.EOF) {
			return lastSeq, offset, hash, nil
		}
		if err != nil {
			return 0, 0, [sha256.Size]byte{}, fmt.Errorf("eventstore: read at offset %d: %w", offset, err)
		}
		var env event.Envelope
		if uerr := json.Unmarshal(line[:len(line)-1], &env); uerr != nil {
			return 0, 0, [sha256.Size]byte{}, fmt.Errorf("eventstore: corrupt line at offset %d: %w", offset, uerr)
		}
		if env.Seq != lastSeq+1 {
			return 0, 0, [sha256.Size]byte{}, fmt.Errorf("eventstore: non-contiguous seq %d after %d", env.Seq, lastSeq)
		}
		offset += int64(len(line))
		hash = rollEventHash(hash, line)
		lastSeq = env.Seq
		if onRecord != nil {
			if err := onRecord(eventIndexRecord{Seq: lastSeq, Offset: offset, Hash: hash}); err != nil {
				return 0, 0, [sha256.Size]byte{}, err
			}
		}
	}
}

func rollEventHash(previous [sha256.Size]byte, line []byte) [sha256.Size]byte {
	h := sha256.New()
	_, _ = h.Write(previous[:])
	_, _ = h.Write(line)
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

func appendIndexRecord(f *os.File, rec eventIndexRecord, sync bool) error {
	var raw [indexRecordSize]byte
	binary.BigEndian.PutUint64(raw[0:8], uint64(rec.Seq))
	binary.BigEndian.PutUint64(raw[8:16], uint64(rec.Offset))
	copy(raw[16:], rec.Hash[:])
	if _, err := f.Write(raw[:]); err != nil {
		return fmt.Errorf("eventstore: index append: %w", err)
	}
	if sync {
		if err := f.Sync(); err != nil {
			return fmt.Errorf("eventstore: index fsync: %w", err)
		}
	}
	return nil
}

func readIndexRecord(f *os.File, ordinal int64) (eventIndexRecord, error) {
	if ordinal < 1 {
		return eventIndexRecord{}, ErrCursorInvalid
	}
	var raw [indexRecordSize]byte
	if _, err := f.ReadAt(raw[:], (ordinal-1)*indexRecordSize); err != nil {
		return eventIndexRecord{}, err
	}
	var rec eventIndexRecord
	rec.Seq = int64(binary.BigEndian.Uint64(raw[0:8]))
	rec.Offset = int64(binary.BigEndian.Uint64(raw[8:16]))
	copy(rec.Hash[:], raw[16:])
	return rec, nil
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
	framed := append(line, '\n')
	if _, err := s.f.Write(framed); err != nil {
		s.broken = true
		return event.Envelope{}, fmt.Errorf("eventstore: append: %w", err)
	}
	if err := s.f.Sync(); err != nil {
		s.broken = true
		return event.Envelope{}, fmt.Errorf("eventstore: fsync: %w", err)
	}
	s.offset += int64(len(framed))
	s.hash = rollEventHash(s.hash, framed)
	// The index is a disposable cache. A journal fact that is already fsynced
	// must never become a failed/ambiguous append merely because its derived
	// cursor could not be persisted; disable the cache and rebuild next open.
	if s.index != nil {
		if err := appendIndexRecord(s.index, eventIndexRecord{
			Seq: s.seq, Offset: s.offset, Hash: s.hash,
		}, false); err != nil {
			_ = s.index.Close()
			s.index = nil
		}
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
	if s.index != nil {
		_ = s.index.Close()
		s.index = nil
	}
	if s.lock != nil {
		// A clean release scrubs the pid first: HasLiveWriter probes the
		// FILE, and the holder may be a long-lived daemon whose pid stays
		// alive long after this session's loop stopped — `stop` left
		// sessions reading "running" forever (QA Round4 F-I2). A crash
		// skips this scrub, so the ESRCH probe still works there.
		_ = s.lock.Truncate(0)
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

// EventCursorAt returns the byte boundary and rolling prefix hash for seq.
// The fixed-width index makes this O(1). ok=false means the derived cache is
// unavailable; callers may still write a legacy snapshot and full-fold.
func EventCursorAt(sessionDir string, seq int64) (offset int64, hash string, ok bool) {
	if seq < 1 {
		return 0, "", false
	}
	idx, err := os.Open(filepath.Join(sessionDir, indexFile))
	if err != nil {
		return 0, "", false
	}
	defer func() { _ = idx.Close() }()
	rec, err := readIndexRecord(idx, seq)
	if err != nil || rec.Seq != seq {
		return 0, "", false
	}
	return rec.Offset, hex.EncodeToString(rec.Hash[:]), true
}

// ReadEventsAfter verifies a snapshot cursor against the O(1) event index,
// seeks directly to its journal byte offset, and decodes only the tail.
// ErrCursorInvalid asks the caller to discard the snapshot optimization.
func ReadEventsAfter(sessionDir string, seq, offset int64, hash string) ([]event.Envelope, error) {
	if seq < 1 || offset < 1 || len(hash) != sha256.Size*2 {
		return nil, ErrCursorInvalid
	}
	wantHash, err := hex.DecodeString(hash)
	if err != nil || len(wantHash) != sha256.Size {
		return nil, ErrCursorInvalid
	}
	idx, err := os.Open(filepath.Join(sessionDir, indexFile))
	if err != nil {
		return nil, ErrCursorInvalid
	}
	rec, rerr := readIndexRecord(idx, seq)
	logPath := filepath.Join(sessionDir, eventsFile)
	boundaryLog, berr := os.Open(logPath)
	boundaryValid := berr == nil && rerr == nil && validateLastIndexedLine(boundaryLog, idx, seq, rec)
	if boundaryLog != nil {
		_ = boundaryLog.Close()
	}
	_ = idx.Close()
	if rerr != nil || rec.Seq != seq || rec.Offset != offset || !bytes.Equal(rec.Hash[:], wantHash) {
		return nil, ErrCursorInvalid
	}
	if !boundaryValid {
		return nil, ErrCursorInvalid
	}
	f, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("eventstore: %w", err)
	}
	defer func() { _ = f.Close() }()
	stat, err := f.Stat()
	if err != nil || offset > stat.Size() {
		return nil, ErrCursorInvalid
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("eventstore: tail seek: %w", err)
	}
	reader := bufio.NewReaderSize(f, 64*1024)
	var out []event.Envelope
	wantSeq := seq + 1
	current := offset
	for {
		line, rerr := reader.ReadBytes('\n')
		if errors.Is(rerr, io.EOF) {
			return out, nil // ignore a concurrent/torn partial tail
		}
		if rerr != nil {
			return nil, fmt.Errorf("eventstore: tail read at offset %d: %w", current, rerr)
		}
		var env event.Envelope
		if err := json.Unmarshal(line[:len(line)-1], &env); err != nil {
			return nil, fmt.Errorf("eventstore: corrupt line at offset %d: %w", current, err)
		}
		if env.Seq != wantSeq {
			return nil, fmt.Errorf("%w: tail seq %d after %d", ErrCursorInvalid, env.Seq, wantSeq-1)
		}
		out = append(out, env)
		wantSeq++
		current += int64(len(line))
	}
}

// ReadEventPrefix decodes at most limit events without loading the journal.
// Resume uses it for the SessionStarted schema guard before reading a tail.
func ReadEventPrefix(sessionDir string, limit int) ([]event.Envelope, error) {
	if limit <= 0 {
		return nil, nil
	}
	f, err := os.Open(filepath.Join(sessionDir, eventsFile))
	if err != nil {
		return nil, fmt.Errorf("eventstore: %w", err)
	}
	defer func() { _ = f.Close() }()
	reader := bufio.NewReaderSize(f, 64*1024)
	out := make([]event.Envelope, 0, limit)
	for len(out) < limit {
		line, rerr := reader.ReadBytes('\n')
		if errors.Is(rerr, io.EOF) {
			return out, nil
		}
		if rerr != nil {
			return nil, fmt.Errorf("eventstore: prefix: %w", rerr)
		}
		var env event.Envelope
		if err := json.Unmarshal(line[:len(line)-1], &env); err != nil {
			return nil, fmt.Errorf("eventstore: corrupt prefix: %w", err)
		}
		out = append(out, env)
	}
	return out, nil
}
