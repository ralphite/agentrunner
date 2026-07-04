package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"

	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/snapshot"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// shadowDirFor maps a (normalized) workspace root to its shadow-repo home:
// one repo per workspace path under <data>/shadow/<hash>, so every session
// in the same workspace shares tree dedup and rewind/fork refs live outside
// the workspace itself.
func shadowDirFor(root string) (string, error) {
	data, err := runtime.DataDir()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(root))
	return filepath.Join(data, "shadow", hex.EncodeToString(h[:6])), nil
}

// snapshotStoreFor opens the workspace's shadow snapshot store (S7.2). Any
// failure degrades to nil — barriers exist for fork/rewind and must never
// be a reason a run cannot start.
func snapshotStoreFor(ws *workspace.Workspace, stderr io.Writer) snapshot.Store {
	dir, err := shadowDirFor(ws.Root())
	if err == nil {
		var st snapshot.Store
		if st, err = snapshot.Open(dir, ws.Root()); err == nil {
			return st
		}
	}
	fmt.Fprintf(stderr, "warning: snapshots disabled (fork/rewind unavailable): %v\n", err)
	return nil
}
