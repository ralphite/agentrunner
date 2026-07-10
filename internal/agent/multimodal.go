package agent

import (
	"encoding/json"
	"fmt"

	"github.com/ralphite/agentrunner/internal/provider"
)

// needsInflate reports whether a part is a blob-carrying kind whose bytes
// are not loaded yet (fold parts carry only the CAS ref).
func needsInflate(p provider.Part) bool {
	return (p.Kind == provider.PartImage || p.Kind == provider.PartFile) &&
		len(p.Data) == 0 && p.Ref != ""
}

// inflateBlobs loads image/file part bytes from the CAS into the assembled
// request messages (v2 M4.1), copying each affected Parts slice so the fold
// stays byte-free — Data is json:"-" and must never reach a journal or
// snapshot. blob-before-event guarantees the ref resolves; a miss is a
// journal-integrity error, not a soft skip.
func (l *Loop) inflateBlobs(msgs []provider.Message) error {
	for i, m := range msgs {
		dirty := false
		for _, p := range m.Parts {
			if needsInflate(p) {
				dirty = true
				break
			}
		}
		if !dirty {
			continue
		}
		if err := l.ensureArtifacts(); err != nil {
			return fmt.Errorf("inflate blobs: %w", err)
		}
		if l.Artifacts == nil {
			return fmt.Errorf("inflate blobs: message has blob refs but no artifact store")
		}
		parts := make([]provider.Part, len(m.Parts))
		copy(parts, m.Parts)
		for j := range parts {
			if !needsInflate(parts[j]) {
				continue
			}
			data, err := l.Artifacts.Get(parts[j].Ref)
			if err != nil {
				return fmt.Errorf("inflate blob %s: %w", parts[j].Ref, err)
			}
			parts[j].Data = data
		}
		msgs[i].Parts = parts
	}
	return nil
}

// mediaResultPart maps a read_file media-envelope result (INC-33) to the blob
// part it carries: {"kind":"image"|"file","media_type":…,"ref":…}. Gated on
// BOTH the tool name and the shape — another tool's coincidental payload must
// not grow a media part (a bogus ref would fail inflation and poison the
// turn). A microcompact-elided result is a placeholder and never matches.
func mediaResultPart(toolName string, result []byte) (provider.Part, bool) {
	if toolName != "read_file" || len(result) == 0 {
		return provider.Part{}, false
	}
	var env struct {
		Kind      string `json:"kind"`
		MediaType string `json:"media_type"`
		Ref       string `json:"ref"`
	}
	if json.Unmarshal(result, &env) != nil || env.Ref == "" || env.MediaType == "" {
		return provider.Part{}, false
	}
	switch env.Kind {
	case "image":
		return provider.Part{Kind: provider.PartImage, Ref: env.Ref, MediaType: env.MediaType}, true
	case "file":
		return provider.Part{Kind: provider.PartFile, Ref: env.Ref, MediaType: env.MediaType}, true
	}
	return provider.Part{}, false
}
