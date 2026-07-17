package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/index"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// S1 defaults pack limits.
const (
	readMaxLines       = 2000
	readMaxBytes       = 50 * 1024
	mediaReadMaxBytes  = 5 << 20   // media (image/PDF) read cap (INC-33)
	bashOutputBytes    = 30 * 1024 // combined budget: split across stdout+stderr
	bashKillGrace      = 5 * time.Second
	bashInterruptGrace = 500 * time.Millisecond
	bashPipeDeadline   = 2 * time.Second
)

// Result is a tool execution outcome. IsError results render as error
// tool_results for the model (决策 #9); the loop continues either way.
type Result struct {
	Payload json.RawMessage
	IsError bool
}

func errResult(format string, args ...any) Result {
	msg, _ := json.Marshal(map[string]any{"error": fmt.Sprintf(format, args...)})
	return Result{Payload: msg, IsError: true}
}

// DisabledToolResult is the model-visible error for a tool call whose name the
// agent's spec never enabled. The spec's tools: face was advisory-only —
// dispatch ran any registered tool regardless (QA Wave2 bob-01/heidi-03) — so
// the loop uses this to refuse an un-advertised call as defense-in-depth.
func DisabledToolResult(name string) Result {
	return errResult("tool %q is not enabled for this agent (add it to the spec's tools: to allow it)", name)
}

func okResult(v any) Result {
	payload, _ := json.Marshal(v)
	return Result{Payload: payload}
}

// SessionEnvVar marks every process a session spawns, so cleanup
// assertions can find strays by marker instead of grepping global ps.
const SessionEnvVar = "AGENTRUNNER_SESSION"

// BlobStore is the CAS seam read_file uses for media reads (INC-33): bytes go
// into the store BEFORE the tool result lands (blob-before-event), so the
// journal only ever carries the ref. The agent loop injects its tree-shared
// artifact store; a bare executor (no store) refuses media reads explicitly.
type BlobStore interface {
	Put(data []byte) (ref string, err error)
}

// Executor runs built-in tools against a workspace. Wall-clock limits are
// NOT owned here (2.11): the activity executor arms a durable timer and
// cancels ctx with cause errs.ErrActivityTimeout; bash only reacts.
type Executor struct {
	WS *workspace.Workspace
	// Session tags spawned processes via SessionEnvVar (2.12).
	Session string
	// blobs is the media-read CAS (INC-33), injected via SetBlobs. Guarded:
	// the executor is shared down the agent tree and every member injects the
	// same tree-root store — first set wins, later sets are no-ops.
	blobsMu sync.Mutex
	blobs   BlobStore
	// index is the lazily-built IndexStore for semantic_search (S7 模块 4):
	// in-memory, derived, rebuilt per process — the executor is shared down
	// the agent tree, so the whole tree shares one index per workspace.
	indexOnce sync.Once
	index     *index.Indexer
	// Network containment (S7 模块 5). The executor is shared down the agent
	// tree, so containment is a RATCHET: any spec in the tree demanding
	// network=none flips it for everyone, and nothing widens it back.
	netNone       atomic.Bool
	sandboxMu     sync.Mutex
	sandboxProbes map[bool]sandboxProbe
	// ProbeSandbox injects a backend capability failure after the real
	// platform probe (tests only).
	ProbeSandbox func(networkNone bool) error
}

// ContainNetwork ratchets bash executions into a fresh network namespace
// (loopback only). Irreversible for the executor's lifetime.
func (e *Executor) ContainNetwork() { e.netNone.Store(true) }

// NetworkContained reports whether bash egress is removed.
func (e *Executor) NetworkContained() bool { return e.netNone.Load() }

// SetBlobs injects the media-read CAS (INC-33). First set wins: the executor
// is shared down the agent tree and every member injects the same tree-root
// store, so later calls are no-ops rather than races.
func (e *Executor) SetBlobs(b BlobStore) {
	e.blobsMu.Lock()
	defer e.blobsMu.Unlock()
	if e.blobs == nil {
		e.blobs = b
	}
}

func (e *Executor) blobStore() BlobStore {
	e.blobsMu.Lock()
	defer e.blobsMu.Unlock()
	return e.blobs
}

// Execute dispatches one tool call. Unknown tools and malformed args are
// model-visible errors, not harness failures.
func (e *Executor) Execute(ctx context.Context, name string, args json.RawMessage) Result {
	switch name {
	case "read_file":
		return e.readFile(args)
	case "edit_file":
		return e.editFile(args)
	case "write_file":
		return e.writeFile(args)
	case "bash":
		return e.bash(ctx, args)
	case "schedule_next":
		return e.scheduleNext(args)
	case "finish_series":
		return e.finishSeries(args)
	case "semantic_search":
		return e.semanticSearch(args)
	case "grep":
		return e.grep(args)
	case "glob":
		return e.glob(args)
	case "web_fetch":
		return e.webFetch(ctx, args)
	case "skill":
		return e.skill(args)
	default:
		return errResult("unknown tool %q", name)
	}
}

// skill (INC-20, #45/§3.5) is the model-side invoke of a skill: given a name
// from the <skills> directory block, return that skill's SKILL.md body (the
// instructions after the frontmatter) as the tool result. Read-class and
// side-effect-free — equivalent to reading the skill's file, but by name.
// SECURITY: the name is a bare identifier — any path separator or traversal
// is refused, and WS.Resolve bounds the final path to the workspace, so this
// can never read outside .claude/skills.
func (e *Executor) skill(rawArgs json.RawMessage) Result {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Name == "" {
		return errResult("skill: invalid args: need {\"name\": string}")
	}
	// A skill name is a directory identifier, never a path. Reject separators
	// and traversal outright rather than relying on Resolve alone.
	if strings.ContainsAny(args.Name, "/\\") || args.Name == "." || args.Name == ".." ||
		strings.Contains(args.Name, "..") {
		return errResult("skill: invalid name %q (skill names are bare identifiers)", args.Name)
	}
	path, err := e.WS.Resolve(filepath.Join(".claude", "skills", args.Name, "SKILL.md"))
	if err != nil {
		return errResult("skill: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return errResult("skill %q not found (check the <skills> directory for available names)", args.Name)
	}
	body := stripFrontmatter(string(raw))
	if strings.TrimSpace(body) == "" {
		// A frontmatter-only skill still has a usable name/description in the
		// directory; return the whole file rather than nothing.
		body = string(raw)
	}
	return okResult(body)
}

// stripFrontmatter drops a leading YAML frontmatter block ("---\n…\n---") and
// returns the remaining body. A file without frontmatter is returned as-is.
func stripFrontmatter(s string) string {
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return s
	}
	rest := s[strings.Index(s, "\n")+1:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return s // unterminated — treat the whole thing as body
	}
	body := rest[end+len("\n---"):]
	// Drop the rest of the closing fence line and one following newline.
	if nl := strings.IndexByte(body, '\n'); nl >= 0 {
		body = body[nl+1:]
	} else {
		body = ""
	}
	return strings.TrimLeft(body, "\n")
}

// grep/glob limits (INC-3). Both are read-class content-surfacing tools:
// they walk the workspace with the SAME credential/vendored-tree exclusion
// as semantic_search (index.SkipDir/SkipFile) so no credential line ever
// lands in the journal, and they cap output like every other tool.
const (
	grepMaxMatches   = 200
	grepMaxLineBytes = 2000    // clamp a single matched line
	grepScanFileCap  = 1 << 20 // bytes scanned per file (skip the tail of huge files)
	globMaxResults   = 1000
)

// resolveSearchRoot bounds an optional workspace-relative sub-path to the
// workspace, falling back to the root. WS.Resolve enforces the boundary.
func (e *Executor) resolveSearchRoot(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return e.WS.Root(), nil
	}
	return e.WS.Resolve(rel)
}

// readForScan reads up to grepScanFileCap bytes, refusing binary files (a
// NUL byte is the cheap, standard heuristic) so grep never dumps a blob.
func readForScan(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, grepScanFileCap))
	if err != nil || bytes.IndexByte(raw, 0) >= 0 {
		return "", false
	}
	return string(raw), true
}

type grepMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
	// Before/After are context lines (INC-24, grep -B/-A/-C), redacted and
	// clamped like the match line. Omitted when no context was requested.
	Before []string `json:"before,omitempty"`
	After  []string `json:"after,omitempty"`
}

// clampGrepLine redacts and length-clamps one grep line (match or context).
func clampGrepLine(r *redact.Redactor, line string) string {
	if len(line) > grepMaxLineBytes {
		line = trimToValidUTF8(line[:grepMaxLineBytes]) + " …[line truncated]"
	}
	return r.String(line)
}

// grep searches file contents by RE2 regex across the workspace, returning
// matching lines (path + 1-based line + redacted text). Credential files and
// vendored trees are excluded at the walk; a bad regex is a model-visible
// error, not a harness failure.
func (e *Executor) grep(rawArgs json.RawMessage) Result {
	var in struct {
		Pattern         string `json:"pattern"`
		Path            string `json:"path"`
		Glob            string `json:"glob"`
		CaseInsensitive bool   `json:"case_insensitive"`
		Multiline       bool   `json:"multiline"`
		OutputMode      string `json:"output_mode"`
		After           int    `json:"-A"`
		Before          int    `json:"-B"`
		Context         int    `json:"-C"`
		MaxResults      int    `json:"max_results"`
	}
	if err := json.Unmarshal(rawArgs, &in); err != nil || strings.TrimSpace(in.Pattern) == "" {
		return errResult("grep: invalid args: need {\"pattern\": string}")
	}
	if e.WS == nil {
		return errResult("grep: no workspace")
	}
	switch in.OutputMode {
	case "", "content", "files_with_matches", "count":
	default:
		return errResult("grep: bad output_mode %q (want content|files_with_matches|count)", in.OutputMode)
	}
	// Inline RE2 flags (INC-22 case_insensitive; INC-27 multiline). multiline
	// adds `s` (dotall: `.` matches `\n`) and `m` (`^`/`$` anchor at line
	// boundaries) so a pattern can span lines while keeping per-line anchor
	// semantics — making multiline a strict superset of the line-by-line mode.
	flags := ""
	if in.CaseInsensitive {
		flags += "i"
	}
	if in.Multiline {
		flags += "sm"
	}
	pat := in.Pattern
	if flags != "" {
		pat = "(?" + flags + ")" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return errResult("grep: bad pattern: %v", err)
	}
	// Optional filename glob (INC-22): validate it once so a bad pattern is a
	// clear error, not silently non-matching.
	if in.Glob != "" {
		if _, err := filepath.Match(in.Glob, "probe"); err != nil {
			return errResult("grep: bad glob %q: %v", in.Glob, err)
		}
	}
	root, err := e.resolveSearchRoot(in.Path)
	if err != nil {
		return errResult("grep: %v", err)
	}
	limit := in.MaxResults
	if limit <= 0 || limit > grepMaxMatches {
		limit = grepMaxMatches
	}
	contentMode := in.OutputMode == "" || in.OutputMode == "content"
	// Context lines (INC-24, grep -A/-B/-C): -C is shorthand for both. Clamp
	// negatives to 0 and cap to a sane window so a huge -C can't blow up output.
	before, after := in.Before, in.Context
	if in.After > after {
		after = in.After
	}
	if in.Context > before {
		before = in.Context
	}
	if before < 0 {
		before = 0
	}
	if after < 0 {
		after = 0
	}
	const grepMaxContext = 20
	if before > grepMaxContext {
		before = grepMaxContext
	}
	if after > grepMaxContext {
		after = grepMaxContext
	}
	r := redact.FromEnv()
	matches := []grepMatch{}
	fileCounts := map[string]int{} // rel path -> match count (files_with_matches / count modes)
	filesScanned := 0
	truncated := false
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: search what we can
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && index.SkipDir(name) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || index.SkipFile(name) {
			return nil
		}
		// Filename glob filter (INC-22): skip files whose basename doesn't match.
		if in.Glob != "" {
			if ok, _ := filepath.Match(in.Glob, name); !ok {
				return nil
			}
		}
		// content mode stops at the line cap; files/count modes scan everything
		// (they are already cheap) so their totals are complete.
		if contentMode && len(matches) >= limit {
			truncated = true
			return fs.SkipAll
		}
		content, ok := readForScan(path)
		if !ok {
			return nil
		}
		filesScanned++
		rel, _ := filepath.Rel(e.WS.Root(), path)
		lines := strings.Split(content, "\n")
		// Multiline (INC-27): match the WHOLE file so a pattern can span lines.
		// Each match reports its starting line; context is taken around the
		// match's start/end lines.
		if in.Multiline {
			for _, loc := range re.FindAllStringIndex(content, -1) {
				fileCounts[rel]++
				if !contentMode {
					continue
				}
				if len(matches) >= limit {
					truncated = true
					return fs.SkipAll
				}
				startLine := 1 + strings.Count(content[:loc[0]], "\n")
				matchText := content[loc[0]:loc[1]]
				m := grepMatch{Path: rel, Line: startLine, Text: clampGrepLine(r, matchText)}
				if before > 0 {
					lo := startLine - 1 - before
					if lo < 0 {
						lo = 0
					}
					for _, bl := range lines[lo : startLine-1] {
						m.Before = append(m.Before, clampGrepLine(r, bl))
					}
				}
				if after > 0 {
					endLine := startLine + strings.Count(matchText, "\n")
					if endLine < len(lines) {
						hi := endLine + after
						if hi > len(lines) {
							hi = len(lines)
						}
						for _, al := range lines[endLine:hi] {
							m.After = append(m.After, clampGrepLine(r, al))
						}
					}
				}
				matches = append(matches, m)
			}
			return nil
		}
		for i, line := range lines {
			if !re.MatchString(line) {
				continue
			}
			fileCounts[rel]++
			if !contentMode {
				continue
			}
			if len(matches) >= limit {
				truncated = true
				return fs.SkipAll
			}
			m := grepMatch{Path: rel, Line: i + 1, Text: clampGrepLine(r, line)}
			if before > 0 {
				lo := i - before
				if lo < 0 {
					lo = 0
				}
				for _, bl := range lines[lo:i] {
					m.Before = append(m.Before, clampGrepLine(r, bl))
				}
			}
			if after > 0 {
				hi := i + 1 + after
				if hi > len(lines) {
					hi = len(lines)
				}
				for _, al := range lines[i+1 : hi] {
					m.After = append(m.After, clampGrepLine(r, al))
				}
			}
			matches = append(matches, m)
		}
		return nil
	})
	if walkErr != nil {
		return errResult("grep: %v", walkErr)
	}
	switch in.OutputMode {
	case "files_with_matches":
		files := make([]string, 0, len(fileCounts))
		for p := range fileCounts {
			files = append(files, p)
		}
		sort.Strings(files)
		return okResult(map[string]any{"files": files, "files_scanned": filesScanned})
	case "count":
		type fc struct {
			Path  string `json:"path"`
			Count int    `json:"count"`
		}
		counts := make([]fc, 0, len(fileCounts))
		for p, c := range fileCounts {
			counts = append(counts, fc{Path: p, Count: c})
		}
		sort.Slice(counts, func(i, j int) bool { return counts[i].Path < counts[j].Path })
		return okResult(map[string]any{"counts": counts, "files_scanned": filesScanned})
	default:
		return okResult(map[string]any{"matches": matches, "files_scanned": filesScanned, "truncated": truncated})
	}
}

// glob lists workspace files whose path matches a glob pattern (with `**`
// depth support). Patterns match relative to the search root; results are
// workspace-relative (usable directly by read_file) and sorted. Excludes
// credential files and vendored trees.
func (e *Executor) glob(rawArgs json.RawMessage) Result {
	var in struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(rawArgs, &in); err != nil || strings.TrimSpace(in.Pattern) == "" {
		return errResult("glob: invalid args: need {\"pattern\": string}")
	}
	if e.WS == nil {
		return errResult("glob: no workspace")
	}
	re, err := globToRegexp(in.Pattern)
	if err != nil {
		return errResult("glob: bad pattern: %v", err)
	}
	root, err := e.resolveSearchRoot(in.Path)
	if err != nil {
		return errResult("glob: %v", err)
	}
	paths := []string{}
	truncated := false
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && index.SkipDir(name) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || index.SkipFile(name) {
			return nil
		}
		relToRoot, err := filepath.Rel(root, path)
		if err != nil || !re.MatchString(relToRoot) {
			return nil
		}
		if len(paths) >= globMaxResults {
			truncated = true
			return fs.SkipAll
		}
		relToWS, _ := filepath.Rel(e.WS.Root(), path)
		paths = append(paths, relToWS)
		return nil
	})
	if walkErr != nil {
		return errResult("glob: %v", walkErr)
	}
	sort.Strings(paths)
	return okResult(map[string]any{"paths": paths, "truncated": truncated})
}

// globToRegexp translates a shell-style glob into an anchored RE2 pattern.
// `**` matches across separators (with `**/` also matching zero segments),
// `*` and `?` stay within one segment. Regex metacharacters are escaped.
func globToRegexp(pat string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pat); i++ {
		c := pat[i]
		switch c {
		case '*':
			if i+1 < len(pat) && pat[i+1] == '*' {
				i++ // consume second '*'
				if i+1 < len(pat) && pat[i+1] == '/' {
					i++ // consume the slash: `**/` may match zero segments
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '\\', '[', ']':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// semanticSearch queries the workspace's IndexStore (S7 模块 4). The
// indexer builds lazily on first use — the index is the fourth state class
// (derived, rebuildable, disposable), so there is nothing to wire up or
// persist; snippets pass the same redaction as every journaled output.
func (e *Executor) semanticSearch(args json.RawMessage) Result {
	var in struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &in); err != nil || strings.TrimSpace(in.Query) == "" {
		return errResult("semantic_search: invalid args: need {\"query\": string}")
	}
	if e.WS == nil {
		return errResult("semantic_search: no workspace")
	}
	e.indexOnce.Do(func() { e.index = index.New(e.WS.Root()) })
	hits, files, err := e.index.Search(in.Query, in.MaxResults)
	if err != nil {
		return errResult("semantic_search: %v", err)
	}
	r := redact.FromEnv()
	for i := range hits {
		hits[i].Snippet = r.String(hits[i].Snippet)
	}
	if hits == nil {
		hits = []index.Hit{}
	}
	return okResult(map[string]any{"hits": hits, "indexed_files": files})
}

// scheduleNext and finishSeries are pure data-definition tools (S6 loop
// mode): the ack is the whole execution — the MEANING is read by the
// IterationDriver from this run's journal when the iteration ends.
func (e *Executor) scheduleNext(rawArgs json.RawMessage) Result {
	var args struct {
		After string `json:"after"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.After == "" {
		return errResult("schedule_next: invalid args: need {\"after\": duration}")
	}
	if _, err := time.ParseDuration(args.After); err != nil {
		return errResult("schedule_next: bad duration %q (want Go form like \"30m\", \"2h\")", args.After)
	}
	return okResult(map[string]any{
		"output": fmt.Sprintf("next iteration requested after %s (the driver clamps and applies it when this iteration ends)", args.After),
	})
}

func (e *Executor) finishSeries(rawArgs json.RawMessage) Result {
	var args struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Reason == "" {
		return errResult("finish_series: invalid args: need {\"reason\": string}")
	}
	return okResult(map[string]any{
		"output": "series completion claimed; a human verifier reviews it when this iteration ends",
	})
}

func (e *Executor) readFile(rawArgs json.RawMessage) Result {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Path == "" {
		return errResult("read_file: invalid args: need {\"path\": string}")
	}
	path, err := e.WS.Resolve(args.Path)
	if err != nil {
		return errResult("read_file: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return errResult("read_file: %v", err)
	}

	// Media reads (INC-33, #32): an image or PDF becomes a CAS ref envelope —
	// the assembly layer attaches the actual bytes as an image/file part on
	// the tool-result message, so the model SEES the media while the journal
	// stays byte-free. Detection is content-based (sniff), so the default
	// text path below is untouched for everything else.
	if mt := http.DetectContentType(raw); strings.HasPrefix(mt, "image/") || mt == "application/pdf" {
		kind := "image"
		if mt == "application/pdf" {
			kind = "file"
		}
		if len(raw) > mediaReadMaxBytes {
			return errResult("read_file: %s is a %d-byte %s; media reads are capped at %d bytes",
				args.Path, len(raw), mt, mediaReadMaxBytes)
		}
		bs := e.blobStore()
		if bs == nil {
			return errResult("read_file: %s is %s content, but this runtime has no blob store for media reads", args.Path, mt)
		}
		ref, err := bs.Put(raw)
		if err != nil {
			return errResult("read_file: store media: %v", err)
		}
		return okResult(map[string]any{
			"kind": kind, "media_type": mt, "ref": ref, "bytes": len(raw),
			"note": "media content is attached to the conversation as a part; describe or analyze it directly",
		})
	}

	content := string(raw)
	truncated := false
	if len(content) > readMaxBytes {
		content = trimToValidUTF8(content[:readMaxBytes])
		truncated = true
	}
	if lines := strings.Split(content, "\n"); len(lines) > readMaxLines {
		content = strings.Join(lines[:readMaxLines], "\n")
		truncated = true
	}
	if truncated {
		content += fmt.Sprintf("\n[truncated: file is %d bytes, showing at most %d lines / %d bytes]",
			len(raw), readMaxLines, readMaxBytes)
	}
	return okResult(map[string]any{"content": content, "truncated": truncated})
}

// writeFile creates or fully overwrites one file inside the workspace
// (v2 M4.3, core tool: 建新文件不再借道 edit_file 的空 old 特例或 bash
// heredoc). Parent directories are created; the boundary is WS.Resolve.
func (e *Executor) writeFile(rawArgs json.RawMessage) Result {
	var args struct {
		Path    string  `json:"path"`
		Content *string `json:"content"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Path == "" || args.Content == nil {
		return errResult("write_file: invalid args: need {\"path\", \"content\"}")
	}
	path, err := e.WS.Resolve(args.Path)
	if err != nil {
		return errResult("write_file: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errResult("write_file: %v", err)
	}
	// Line-delta accounting (INC-43): an overwrite counts as rewrite —
	// every previous line removed, every new line added. The numbers ride
	// the result payload, so the model sees them and inspect's stats sum
	// them from the journal without re-diffing redacted content.
	removed := 0
	if old, rerr := os.ReadFile(path); rerr == nil {
		removed = countLines(string(old))
	}
	if err := os.WriteFile(path, []byte(*args.Content), 0o644); err != nil {
		return errResult("write_file: %v", err)
	}
	return okResult(map[string]any{"output": fmt.Sprintf("wrote %s (%d bytes)", args.Path, len(*args.Content)),
		"lines_added": countLines(*args.Content), "lines_removed": removed})
}

func (e *Executor) editFile(rawArgs json.RawMessage) Result {
	var args struct {
		Path string `json:"path"`
		Old  string `json:"old"`
		New  string `json:"new"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Path == "" {
		return errResult("edit_file: invalid args: need {\"path\", \"old\", \"new\"}")
	}
	path, err := e.WS.Resolve(args.Path)
	if err != nil {
		return errResult("edit_file: %v", err)
	}

	if args.Old == "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return errResult("edit_file: %v", err)
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				return errResult("edit_file: %s exists; empty \"old\" creates new files only", args.Path)
			}
			return errResult("edit_file: %v", err)
		}
		_, werr := f.WriteString(args.New)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return errResult("edit_file: %v", werr)
		}
		return okResult(map[string]any{"output": fmt.Sprintf("created %s", args.Path),
			"lines_added": countLines(args.New), "lines_removed": 0})
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return errResult("edit_file: %v", err)
	}
	content := string(raw)
	switch n := strings.Count(content, args.Old); n {
	case 1:
		// fallthrough to replace
	case 0:
		return errResult("edit_file: old string not found in %s (0 matches, need exactly 1)", args.Path)
	default:
		return errResult("edit_file: old string matches %d times in %s, need exactly 1", n, args.Path)
	}
	content = strings.Replace(content, args.Old, args.New, 1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return errResult("edit_file: %v", err)
	}
	// Replacement delta counts the edited span's lines (INC-43): the old
	// snippet's lines out, the new snippet's lines in — a one-line tweak
	// reads as 1/1, not a whole-file diff.
	return okResult(map[string]any{"output": fmt.Sprintf("edited %s", args.Path),
		"lines_added": countLines(args.New), "lines_removed": countLines(args.Old)})
}

// countLines counts content lines for the INC-43 delta accounting: empty
// content is zero lines; otherwise a trailing newline does not add one.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

func (e *Executor) bash(ctx context.Context, rawArgs json.RawMessage) Result {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Command == "" {
		return errResult("bash: invalid args: need {\"command\": string}")
	}
	cmd, cleanup, err := e.sandboxedBash(args.Command)
	if err != nil {
		return errResult("bash: required OS sandbox unavailable (%v) — refusing to run", err)
	}
	defer cleanup()
	return e.runSandboxed(ctx, cmd, nil)
}

// RunCommandTool executes a user-defined command tool (INC-55, HANDA-PARITY
// #4): the manifest's FIXED command runs in the mandatory OS sandbox (决策
// #34) — the exact bash containment (isolated HOME/TMP, credential-path
// denial, network ratchet) and process lifecycle — with the model's arguments
// delivered as JSON on the command's stdin. A command tool is a bash effect
// whose command line is fixed by the manifest and whose args are DATA, never
// shell, so it never widens the shell-injection surface bash already governs.
// The loop adjudicates the fixed command through the full pipeline (execute-
// class command effect) before this ever runs.
func (e *Executor) RunCommandTool(ctx context.Context, command string, argsJSON json.RawMessage) Result {
	cmd, cleanup, err := e.sandboxedBash(command)
	if err != nil {
		return errResult("command tool: required OS sandbox unavailable (%v) — refusing to run", err)
	}
	defer cleanup()
	stdin := []byte(argsJSON)
	if len(bytes.TrimSpace(stdin)) == 0 {
		stdin = []byte("{}") // a parameterless call still gets a valid JSON object
	}
	return e.runSandboxed(ctx, cmd, stdin)
}

// runSandboxed runs a prepared sandboxed command, optionally feeding stdin,
// and renders the standard {stdout,stderr,exit_code[,timed_out|canceled]}
// result shared by bash and command tools. Wall-clock is owned by the durable
// timer (2.11): a ctx cancel caused by ErrActivityTimeout renders timed_out,
// any other cancellation renders canceled.
func (e *Executor) runSandboxed(ctx context.Context, cmd *exec.Cmd, stdin []byte) Result {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if fn := liveOutput(ctx); fn != nil {
		// Progress tail (B9): mirror chunks out as they arrive. The buffers
		// stay the completion truth; the tee is bytes-for-bytes the same.
		cmd.Stdout = teeWriter{dst: &stdout, fn: fn}
		cmd.Stderr = teeWriter{dst: &stderr, fn: fn}
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Background children inherit the output pipes; don't let them hold
	// Wait hostage after the direct child exits.
	cmd.WaitDelay = bashPipeDeadline

	if err := cmd.Start(); err != nil {
		return errResult("exec: %v", err)
	}
	pgid := cmd.Process.Pid

	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		done <- err
	}()

	timedOut, canceled := false, false
	select {
	case <-done:
	case <-ctx.Done():
		// The command may have finished in the same instant; prefer done —
		// a completed command must not be journaled as canceled.
		select {
		case <-done:
		default:
			// The durable timer cancels with cause ErrActivityTimeout;
			// render that as a timeout, anything else as user cancellation.
			// A steering interrupt gets a much shorter kill grace than a
			// timeout — interactive cancellation must feel instant.
			grace := bashKillGrace
			if errors.Is(context.Cause(ctx), errs.ErrActivityTimeout) {
				timedOut = true
			} else {
				canceled = true
				if errors.Is(context.Cause(ctx), errs.ErrUserInterrupt) {
					grace = bashInterruptGrace
				}
			}
			killGroup(pgid, grace)
			<-done
		}
	}

	out := map[string]any{
		"stdout":    truncateHeadTail(stdout.String(), bashOutputBytes/2),
		"stderr":    truncateHeadTail(stderr.String(), bashOutputBytes/2),
		"exit_code": cmd.ProcessState.ExitCode(),
	}
	switch {
	case timedOut:
		out["timed_out"] = true
		out["error"] = "command killed after timeout"
	case canceled:
		out["canceled"] = true
		out["error"] = "command canceled"
	}
	payload, _ := json.Marshal(out)
	return Result{Payload: payload, IsError: timedOut || canceled || cmd.ProcessState.ExitCode() != 0}
}

// killGroup terminates the whole process group: SIGTERM, grace, then SIGKILL.
// The direct sandbox wrapper can exit before a TERM-resistant grandchild;
// therefore reaping the wrapper is not proof that the group is gone. Probe
// the group itself and stop immediately on ESRCH. Once ESRCH is observed we
// never signal this pgid again, avoiding a later PID-reuse hazard.
func killGroup(pgid int, grace time.Duration) {
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	deadline := time.After(grace)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			// A final existence check narrows the signal to a group that still
			// contains one of our descendants. ESRCH means cleanup completed.
			if err := syscall.Kill(-pgid, syscall.Signal(0)); !errors.Is(err, syscall.ESRCH) {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			}
			return
		case <-tick.C:
			// Signal 0 probes for existence; only ESRCH means the group is
			// gone (EPERM = alive but unsignalable — keep escalating).
			if err := syscall.Kill(-pgid, syscall.Signal(0)); errors.Is(err, syscall.ESRCH) {
				return
			}
		}
	}
}

func truncateHeadTail(s string, budget int) string {
	if len(s) <= budget {
		return s
	}
	half := budget / 2
	head := trimToValidUTF8(s[:half])
	tail := s[len(s)-half:]
	for len(tail) > 0 && !utf8.ValidString(tail) {
		tail = tail[1:]
	}
	return head +
		fmt.Sprintf("\n[... truncated %d bytes ...]\n", len(s)-budget) +
		tail
}

// trimToValidUTF8 drops at most 3 trailing bytes to avoid a torn rune.
func trimToValidUTF8(s string) string {
	for i := 0; i < 4 && len(s) > 0 && !utf8.ValidString(s); i++ {
		s = s[:len(s)-1]
	}
	return s
}
