package tool

import (
	"os"
	"path/filepath"
	"testing"
)

// mkfile writes a file (creating parent dirs) under the workspace root.
func mkfile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func grepMatches(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, _ := m["matches"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(map[string]any))
	}
	return out
}

func TestGrepFindsMatches(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "a.go", "package x\nfunc Foo() {}\n")
	mkfile(t, root, "sub/b.txt", "nothing here\nFoo bar\n")
	mkfile(t, root, "c.go", "package x\nfunc Bar() {}\n")

	m, isErr := run(t, e, "grep", `{"pattern":"Foo"}`)
	if isErr {
		t.Fatalf("grep errored: %v", m)
	}
	ms := grepMatches(t, m)
	if len(ms) != 2 {
		t.Fatalf("want 2 matches, got %d: %v", len(ms), ms)
	}
	// Path a.go line 2, sub/b.txt line 2.
	got := map[string]int{}
	for _, mm := range ms {
		got[mm["path"].(string)] = int(mm["line"].(float64))
	}
	if got["a.go"] != 2 || got[filepath.Join("sub", "b.txt")] != 2 {
		t.Fatalf("unexpected match locations: %v", got)
	}
}

// The credential red line: grep must never surface the content of a
// credential-shaped file, even though it lives in the workspace.
func TestGrepRespectsCredentialExclusion(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, ".env", "API_TOKEN=supersecretvalue\n")
	mkfile(t, root, "readme.md", "set API_TOKEN in your env\n")

	m, isErr := run(t, e, "grep", `{"pattern":"API_TOKEN"}`)
	if isErr {
		t.Fatalf("grep errored: %v", m)
	}
	ms := grepMatches(t, m)
	if len(ms) != 1 || ms[0]["path"].(string) != "readme.md" {
		t.Fatalf("credential file leaked or wrong matches: %v", ms)
	}
	for _, mm := range ms {
		if mm["path"] == ".env" {
			t.Fatal(".env content surfaced in grep — credential red line breached")
		}
	}
}

func TestGrepSkipsVendoredTrees(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "node_modules/pkg/index.js", "needle here\n")
	mkfile(t, root, "src/app.js", "needle here\n")

	m, _ := run(t, e, "grep", `{"pattern":"needle"}`)
	ms := grepMatches(t, m)
	if len(ms) != 1 || ms[0]["path"].(string) != filepath.Join("src", "app.js") {
		t.Fatalf("vendored tree not excluded: %v", ms)
	}
}

func TestGrepBadRegexIsModelError(t *testing.T) {
	e, _ := newExec(t)
	m, isErr := run(t, e, "grep", `{"pattern":"["}`)
	if !isErr {
		t.Fatalf("bad regex should be a model-visible error, got %v", m)
	}
}

func TestGrepTruncates(t *testing.T) {
	e, root := newExec(t)
	var b []byte
	for i := 0; i < 50; i++ {
		b = append(b, []byte("match line\n")...)
	}
	mkfile(t, root, "big.txt", string(b))

	m, _ := run(t, e, "grep", `{"pattern":"match","max_results":10}`)
	ms := grepMatches(t, m)
	if len(ms) != 10 {
		t.Fatalf("want 10 (capped), got %d", len(ms))
	}
	if m["truncated"] != true {
		t.Fatal("truncated flag should be set")
	}
}

// Omitting max_results honors the grep.json contract "default 100, cap 200":
// the default is 100, NOT the 200 hard cap (audit 2026-07-21 caught the code
// returning up to 200 on omit while the model-facing contract promised 100).
func TestGrepDefaultLimitIs100NotCap(t *testing.T) {
	e, root := newExec(t)
	var b []byte
	for i := 0; i < 250; i++ { // more than both the default (100) and the cap (200)
		b = append(b, []byte("match line\n")...)
	}
	mkfile(t, root, "big.txt", string(b))

	// Omitted max_results → default 100.
	m, _ := run(t, e, "grep", `{"pattern":"match"}`)
	if got := len(grepMatches(t, m)); got != grepDefaultMatches {
		t.Fatalf("omitted max_results = %d matches, want default %d", got, grepDefaultMatches)
	}
	// An explicit request above the default but under the cap is honored.
	m, _ = run(t, e, "grep", `{"pattern":"match","max_results":150}`)
	if got := len(grepMatches(t, m)); got != 150 {
		t.Fatalf("max_results=150 = %d matches, want 150", got)
	}
	// An explicit request above the cap is clamped to the cap.
	m, _ = run(t, e, "grep", `{"pattern":"match","max_results":9999}`)
	if got := len(grepMatches(t, m)); got != grepMaxMatches {
		t.Fatalf("max_results=9999 = %d matches, want cap %d", got, grepMaxMatches)
	}
}

func TestGrepSkipsBinary(t *testing.T) {
	e, root := newExec(t)
	// A NUL byte marks the file binary; its "needle" must not be scanned.
	if err := os.WriteFile(filepath.Join(root, "blob.bin"), []byte("needle\x00needle"), 0o644); err != nil {
		t.Fatal(err)
	}
	mkfile(t, root, "text.txt", "needle\n")
	m, _ := run(t, e, "grep", `{"pattern":"needle"}`)
	ms := grepMatches(t, m)
	if len(ms) != 1 || ms[0]["path"].(string) != "text.txt" {
		t.Fatalf("binary file scanned: %v", ms)
	}
}

func TestGrepPathScope(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "keep/a.txt", "target\n")
	mkfile(t, root, "other/b.txt", "target\n")
	m, _ := run(t, e, "grep", `{"pattern":"target","path":"keep"}`)
	ms := grepMatches(t, m)
	if len(ms) != 1 || ms[0]["path"].(string) != filepath.Join("keep", "a.txt") {
		t.Fatalf("path scope not honored: %v", ms)
	}
}

func globPaths(t *testing.T, m map[string]any) []string {
	t.Helper()
	raw, _ := m["paths"].([]any)
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(string))
	}
	return out
}

func TestGlobMatches(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "a.go", "")
	mkfile(t, root, "sub/deep/b.go", "")
	mkfile(t, root, "readme.md", "")

	m, isErr := run(t, e, "glob", `{"pattern":"**/*.go"}`)
	if isErr {
		t.Fatalf("glob errored: %v", m)
	}
	ps := globPaths(t, m)
	if len(ps) != 2 {
		t.Fatalf("want 2 .go files, got %v", ps)
	}

	m, _ = run(t, e, "glob", `{"pattern":"*.md"}`)
	ps = globPaths(t, m)
	if len(ps) != 1 || ps[0] != "readme.md" {
		t.Fatalf("root-level glob wrong: %v", ps)
	}
}

func TestGlobRespectsExclusion(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "node_modules/x.go", "")
	mkfile(t, root, ".env", "")
	mkfile(t, root, "real.go", "")

	m, _ := run(t, e, "glob", `{"pattern":"**/*"}`)
	ps := globPaths(t, m)
	for _, p := range ps {
		if p == ".env" || p == filepath.Join("node_modules", "x.go") {
			t.Fatalf("excluded path surfaced: %v", ps)
		}
	}
	found := false
	for _, p := range ps {
		if p == "real.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("real file missing: %v", ps)
	}
}

func TestGlobPathScope(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "cmd/main.go", "")
	mkfile(t, root, "internal/x.go", "")
	// Pattern is relative to the search root; output stays workspace-relative.
	m, _ := run(t, e, "glob", `{"pattern":"*.go","path":"cmd"}`)
	ps := globPaths(t, m)
	if len(ps) != 1 || ps[0] != filepath.Join("cmd", "main.go") {
		t.Fatalf("path-scoped glob wrong: %v", ps)
	}
}

// Glob metacharacters that are regex-special are escaped, so a `[` in a
// pattern matches a literal bracket rather than erroring.
func TestGlobLiteralBracket(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "a[1].txt", "")
	mkfile(t, root, "b.txt", "")
	m, isErr := run(t, e, "glob", `{"pattern":"a[1].txt"}`)
	if isErr {
		t.Fatalf("glob errored: %v", m)
	}
	ps := globPaths(t, m)
	if len(ps) != 1 || ps[0] != "a[1].txt" {
		t.Fatalf("literal bracket not matched: %v", ps)
	}
}

// globToRegexp unit checks: the `**` semantics are the fiddly part.
func TestGlobToRegexp(t *testing.T) {
	cases := []struct {
		pat, path string
		want      bool
	}{
		{"**/*.go", "a.go", true},
		{"**/*.go", "x/y/a.go", true},
		{"**/*.go", "a.txt", false},
		{"*.md", "readme.md", true},
		{"*.md", "sub/readme.md", false},
		{"cmd/**", "cmd/a/b.go", true},
		{"cmd/**", "internal/a.go", false},
		{"internal/**/*.go", "internal/x.go", true},
		{"internal/**/*.go", "internal/a/b/x.go", true},
	}
	for _, c := range cases {
		re, err := globToRegexp(c.pat)
		if err != nil {
			t.Fatalf("%q: compile: %v", c.pat, err)
		}
		if got := re.MatchString(c.path); got != c.want {
			t.Errorf("glob %q vs %q = %v, want %v (regex %s)", c.pat, c.path, got, c.want, re.String())
		}
	}
}

// Registry wiring: both tools advertise to the provider and dispatch.
func TestGrepGlobRegistered(t *testing.T) {
	for _, name := range []string{"grep", "glob"} {
		if _, ok := Get(name); !ok {
			t.Fatalf("%s not registered", name)
		}
		if _, err := ProviderDefs([]string{name}); err != nil {
			t.Fatalf("%s provider def: %v", name, err)
		}
	}
}
