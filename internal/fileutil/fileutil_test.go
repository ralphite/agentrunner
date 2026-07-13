package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func TestAtomicWriteAndLockSerializeTransactions(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "counter")
	if err := AtomicWrite(path, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}
	const writers = 32
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := WithLock(path, func() error {
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				n, err := strconv.Atoi(string(raw))
				if err != nil {
					return err
				}
				return AtomicWrite(path, []byte(strconv.Itoa(n+1)), 0o600)
			}); err != nil {
				t.Errorf("transaction: %v", err)
			}
		}()
	}
	wg.Wait()
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != fmt.Sprint(writers) {
		t.Fatalf("counter = %q, %v; want %d", raw, err, writers)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, err = %v", info.Mode(), err)
	}
}
