// INC-50 (G14/UJ-12): the hook registry backing the daemon's HTTP ingress.
// A hook is a per-session delivery capability: an unguessable id plus a
// bearer token. The token exists in plaintext exactly once — on the create
// response — and is stored only as a sha256 hash; it never enters a journal.
// The registry is a small owner-only JSON file under the data dir, written
// atomically by the CLI (`ar hook create/revoke`) and re-read by the daemon
// on every request, so hook changes need no daemon restart.
package daemon

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ralphite/agentrunner/internal/fileutil"
)

// Hook is one registered ingress capability.
type Hook struct {
	ID          string `json:"id"`
	Session     string `json:"session"`
	Name        string `json:"name,omitempty"`
	TokenSHA256 string `json:"token_sha256"`
	CreatedAt   string `json:"created_at"`
}

type hookFile struct {
	Hooks []Hook `json:"hooks"`
}

// Principal is the identity a delivery through this hook carries into the
// journal ("hook:<name>", falling back to the id when unnamed).
func (h Hook) Principal() string {
	if h.Name != "" {
		return "hook:" + h.Name
	}
	return "hook:" + h.ID
}

// VerifyToken reports whether the presented plaintext token matches this
// hook, in constant time over the hashes.
func (h Hook) VerifyToken(token string) bool {
	sum := sha256.Sum256([]byte(token))
	return hmac.Equal([]byte(hex.EncodeToString(sum[:])), []byte(h.TokenSHA256))
}

// LoadHooks reads the registry; a missing file is an empty registry.
func LoadHooks(path string) ([]Hook, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f hookFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("hooks registry %s: %w", path, err)
	}
	return f.Hooks, nil
}

// FindHook resolves one hook by id.
func FindHook(path, id string) (Hook, bool, error) {
	hooks, err := LoadHooks(path)
	if err != nil {
		return Hook{}, false, err
	}
	for _, h := range hooks {
		if h.ID == id {
			return h, true, nil
		}
	}
	return Hook{}, false, nil
}

// CreateHook mints a new hook for a session and persists it. The returned
// token is the ONLY plaintext copy that will ever exist.
func CreateHook(path, session, name string) (Hook, string, error) {
	id, err := randomHex(8)
	if err != nil {
		return Hook{}, "", err
	}
	token, err := randomHex(32)
	if err != nil {
		return Hook{}, "", err
	}
	sum := sha256.Sum256([]byte(token))
	h := Hook{
		ID: id, Session: session, Name: name,
		TokenSHA256: hex.EncodeToString(sum[:]),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := fileutil.WithLock(path, func() error {
		hooks, err := LoadHooks(path)
		if err != nil {
			return err
		}
		return writeHooks(path, append(hooks, h))
	}); err != nil {
		return Hook{}, "", err
	}
	return h, token, nil
}

// RevokeHook removes a hook by id; reports whether it existed.
func RevokeHook(path, id string) (bool, error) {
	found := false
	err := fileutil.WithLock(path, func() error {
		hooks, err := LoadHooks(path)
		if err != nil {
			return err
		}
		kept := hooks[:0]
		for _, h := range hooks {
			if h.ID == id {
				found = true
				continue
			}
			kept = append(kept, h)
		}
		if !found {
			return nil
		}
		return writeHooks(path, kept)
	})
	return found, err
}

// writeHooks persists the registry atomically (temp + rename), owner-only:
// anyone who can read it can enumerate hook ids (though not tokens).
func writeHooks(path string, hooks []Hook) error {
	raw, err := json.MarshalIndent(hookFile{Hooks: hooks}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fileutil.AtomicWrite(path, raw, 0o600)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("hooks: entropy unavailable: %w", err)
	}
	return hex.EncodeToString(b), nil
}
