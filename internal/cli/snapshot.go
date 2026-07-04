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

// snapshotStoreFor opens the workspace's shadow snapshot store (S7.2): one
// repo per workspace path under <data>/shadow/<hash>, so every session in
// the same workspace shares tree dedup and rewind/fork refs live outside
// the workspace itself. Any failure degrades to nil — barriers exist for
// fork/rewind and must never be a reason a run cannot start.
func snapshotStoreFor(ws *workspace.Workspace, stderr io.Writer) snapshot.Store {
	data, err := runtime.DataDir()
	if err == nil {
		h := sha256.Sum256([]byte(ws.Root()))
		dir := filepath.Join(data, "shadow", hex.EncodeToString(h[:6]))
		var st snapshot.Store
		if st, err = snapshot.Open(dir, ws.Root()); err == nil {
			return st
		}
	}
	fmt.Fprintf(stderr, "warning: snapshots disabled (fork/rewind unavailable): %v\n", err)
	return nil
}
