package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"unicode/utf8"

	"github.com/ralphite/agentrunner/internal/tool"
)

// The artifact consumption face (INC-40, HANDA #11): artifacts_list /
// artifacts_read are loop-handled read tools like goal_status — they read
// the fold's Published projection (the journaled truth; orphan blobs from a
// publish crash window never appear) plus the CAS, touch nothing, and so
// bypass the effect pipeline.
const (
	artifactsReadDefaultBytes = 20000
	artifactsReadMaxBytes     = 50000
)

// runArtifactsList answers with every published stream's latest version and
// size. published is the fold snapshot taken on the drive goroutine.
func (l *Loop) runArtifactsList(published map[string]int) tool.Result {
	if l.Artifacts == nil {
		return errorResult("artifacts_list: no artifact store in this run")
	}
	type row struct {
		Stream  string `json:"stream"`
		Version int    `json:"version"`
		Bytes   int    `json:"bytes,omitempty"`
	}
	rows := make([]row, 0, len(published))
	streams, err := l.Artifacts.Streams()
	if err != nil {
		return errorResult(fmt.Sprintf("artifacts_list: reading the store failed: %v", err))
	}
	for stream, version := range published {
		r := row{Stream: stream, Version: version}
		for _, v := range streams[stream] {
			if v.Version == version {
				r.Bytes = v.Bytes
			}
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Stream < rows[j].Stream })
	p, _ := json.Marshal(map[string]any{"artifacts": rows, "count": len(rows)})
	return tool.Result{Payload: p}
}

// runArtifactsRead fetches one version's content: UTF-8 text is sliced by
// offset/max_bytes with a next_offset cursor; binary content answers with
// metadata only (pushing raw bytes at the model helps nobody).
func (l *Loop) runArtifactsRead(published map[string]int, args json.RawMessage) tool.Result {
	if l.Artifacts == nil {
		return errorResult("artifacts_read: no artifact store in this run")
	}
	var in struct {
		Stream   string `json:"stream"`
		Version  int    `json:"version"`
		Offset   int    `json:"offset"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := json.Unmarshal(args, &in); err != nil || in.Stream == "" {
		return errorResult(`artifacts_read: invalid args: need {"stream"}`)
	}
	latest, ok := published[in.Stream]
	if !ok {
		return errorResult(fmt.Sprintf("artifacts_read: no published artifact stream %q (artifacts_list shows what exists)", in.Stream))
	}
	version := in.Version
	if version == 0 {
		version = latest
	}
	// Resolve the version's ref through the store manifest; only journaled
	// versions (≤ the fold's latest) are addressable.
	if version < 1 || version > latest {
		return errorResult(fmt.Sprintf("artifacts_read: stream %q has versions 1..%d", in.Stream, latest))
	}
	streams, err := l.Artifacts.Streams()
	if err != nil {
		return errorResult(fmt.Sprintf("artifacts_read: reading the store failed: %v", err))
	}
	ref := ""
	for _, v := range streams[in.Stream] {
		if v.Version == version {
			ref = v.Ref
		}
	}
	if ref == "" {
		return errorResult(fmt.Sprintf("artifacts_read: version %d of %q is not in the store", version, in.Stream))
	}
	content, err := l.Artifacts.Get(ref)
	if err != nil {
		return errorResult(fmt.Sprintf("artifacts_read: fetching %s failed: %v", ref, err))
	}
	if !utf8.Valid(content) {
		p, _ := json.Marshal(map[string]any{
			"stream": in.Stream, "version": version, "binary": true,
			"total_bytes": len(content),
			"note":        "binary content is not rendered; consume it via its ref outside the conversation",
		})
		return tool.Result{Payload: p}
	}
	maxB := in.MaxBytes
	if maxB <= 0 {
		maxB = artifactsReadDefaultBytes
	}
	if maxB > artifactsReadMaxBytes {
		maxB = artifactsReadMaxBytes
	}
	off := in.Offset
	if off < 0 || off > len(content) {
		return errorResult(fmt.Sprintf("artifacts_read: offset %d out of range (total %d bytes)", off, len(content)))
	}
	end := off + maxB
	if end > len(content) {
		end = len(content)
	}
	// Never split a UTF-8 rune at the slice edge.
	for end < len(content) && end > off && !utf8.RuneStart(content[end]) {
		end--
	}
	out := map[string]any{
		"stream": in.Stream, "version": version,
		"content": string(content[off:end]), "total_bytes": len(content),
	}
	if end < len(content) {
		out["next_offset"] = end
	}
	p, _ := json.Marshal(out)
	return tool.Result{Payload: p}
}
