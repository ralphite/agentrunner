// Package fileutil owns the small, shared filesystem primitives used by
// cross-session state writers. Locks live in the user's cache rather than
// beside the target so locking CLAUDE.md never pollutes a workspace.
package fileutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// WithLock serializes a read-modify-write transaction for target across
// goroutines and processes. The persistent lock file is harmless: flock is
// released by the kernel when a process exits, so stale files never block.
func WithLock(target string, fn func() error) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("file lock: %w", err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("file lock: %w", err)
	}
	sum := sha256.Sum256([]byte(filepath.Clean(abs)))
	dir := filepath.Join(cache, "agentrunner", "locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("file lock: %w", err)
	}
	path := filepath.Join(dir, hex.EncodeToString(sum[:])+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("file lock: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("file lock: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	return fn()
}

// AtomicWrite replaces path only after the complete new content is fsynced.
// A unique temp name lets unrelated writers fail independently; callers that
// need read-modify-write semantics combine it with WithLock.
func AtomicWrite(path string, data []byte, perm os.FileMode) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()
	if err := f.Chmod(perm); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	closed = true
	return os.Rename(tmp, path)
}
