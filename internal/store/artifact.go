package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ArtifactStore is the content-addressed deliverable store (S5.5) — the
// SnapshotStore pattern (atomic tmp+fsync+rename, degrade-friendly reads)
// reused for a CAS. Refs are opaque "sha256-<hex>" strings derived from
// content; identical content publishes to the same blob idempotently.
//
// The ordering invariant mirrors the journal's fsync-before-ack: the BLOB
// (and manifest) are durable BEFORE the caller journals ArtifactPublished,
// so a ref in the event log always resolves. A crash between blob write and
// event append leaves an orphan blob — harmless, GC-able — never a dangling
// ref.
type ArtifactStore struct {
	root string
}

// ArtifactVersion is one entry of a stream's version chain.
type ArtifactVersion struct {
	Stream  string `json:"stream"`
	Version int    `json:"version"`
	Ref     string `json:"ref"`
	Bytes   int    `json:"bytes"`
}

// manifest is the whole store's stream → version-chain index.
type manifest struct {
	Streams map[string][]ArtifactVersion `json:"streams"`
}

// OpenArtifactStore opens (creating if needed) an artifact store rooted at
// dir — conventionally <sessionDir>/artifacts.
func OpenArtifactStore(dir string) (*ArtifactStore, error) {
	if err := os.MkdirAll(filepath.Join(dir, "blobs"), 0o700); err != nil {
		return nil, fmt.Errorf("artifacts: %w", err)
	}
	return &ArtifactStore{root: dir}, nil
}

// Put writes content as a blob and returns its ref. Durable (fsynced)
// before return; writing existing content is a no-op returning the same ref.
func (a *ArtifactStore) Put(content []byte) (string, error) {
	sum := sha256.Sum256(content)
	ref := "sha256-" + hex.EncodeToString(sum[:])
	final := filepath.Join(a.root, "blobs", ref)
	if _, err := os.Stat(final); err == nil {
		return ref, nil // CAS: same content, same blob
	}
	if err := atomicWrite(final, content); err != nil {
		return "", fmt.Errorf("artifacts: %w", err)
	}
	return ref, nil
}

// Get resolves a ref to its content.
func (a *ArtifactStore) Get(ref string) ([]byte, error) {
	if filepath.Base(ref) != ref {
		return nil, fmt.Errorf("artifacts: malformed ref %q", ref)
	}
	raw, err := os.ReadFile(filepath.Join(a.root, "blobs", ref))
	if err != nil {
		return nil, fmt.Errorf("artifacts: ref %s: %w", ref, err)
	}
	return raw, nil
}

// Publish writes content and appends it to the stream's version chain
// (versions are 1-based, dense). Blob and manifest are both durable when
// this returns — the caller may then journal the fact.
func (a *ArtifactStore) Publish(stream string, content []byte) (ArtifactVersion, error) {
	if stream == "" {
		return ArtifactVersion{}, fmt.Errorf("artifacts: empty stream name")
	}
	ref, err := a.Put(content)
	if err != nil {
		return ArtifactVersion{}, err
	}
	m, err := a.readManifest()
	if err != nil {
		return ArtifactVersion{}, err
	}
	v := ArtifactVersion{Stream: stream, Version: len(m.Streams[stream]) + 1,
		Ref: ref, Bytes: len(content)}
	m.Streams[stream] = append(m.Streams[stream], v)
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return ArtifactVersion{}, fmt.Errorf("artifacts: %w", err)
	}
	if err := atomicWrite(filepath.Join(a.root, "manifest.json"), raw); err != nil {
		return ArtifactVersion{}, fmt.Errorf("artifacts: %w", err)
	}
	return v, nil
}

// Latest returns a stream's newest version.
func (a *ArtifactStore) Latest(stream string) (ArtifactVersion, bool, error) {
	m, err := a.readManifest()
	if err != nil {
		return ArtifactVersion{}, false, err
	}
	chain := m.Streams[stream]
	if len(chain) == 0 {
		return ArtifactVersion{}, false, nil
	}
	return chain[len(chain)-1], true, nil
}

// Streams returns the full manifest (stream → version chain).
func (a *ArtifactStore) Streams() (map[string][]ArtifactVersion, error) {
	m, err := a.readManifest()
	if err != nil {
		return nil, err
	}
	return m.Streams, nil
}

func (a *ArtifactStore) readManifest() (manifest, error) {
	m := manifest{Streams: map[string][]ArtifactVersion{}}
	raw, err := os.ReadFile(filepath.Join(a.root, "manifest.json"))
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return m, fmt.Errorf("artifacts: %w", err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("artifacts: manifest corrupt: %w", err)
	}
	if m.Streams == nil {
		m.Streams = map[string][]ArtifactVersion{}
	}
	return m, nil
}

// atomicWrite is the snapshot-store discipline: tmp + fsync + rename, 0600.
func atomicWrite(final string, data []byte) error {
	tmp := final + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}
