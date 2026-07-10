package tool

import (
	"path/filepath"
	"testing"
)

// case_insensitive matches regardless of case (INC-22, #35).
func TestGrepCaseInsensitive(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "a.go", "FooBar\nfoobar\nFOOBAR\n")

	// Sensitive (default): only the exact-case line.
	m, _ := run(t, e, "grep", `{"pattern":"foobar"}`)
	if got := len(grepMatches(t, m)); got != 1 {
		t.Fatalf("case-sensitive: want 1 match, got %d", got)
	}
	// Insensitive: all three.
	m, _ = run(t, e, "grep", `{"pattern":"foobar","case_insensitive":true}`)
	if got := len(grepMatches(t, m)); got != 3 {
		t.Fatalf("case-insensitive: want 3 matches, got %d", got)
	}
}

// glob restricts which files are searched by basename.
func TestGrepGlobFilter(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "a.go", "target\n")
	mkfile(t, root, "b.txt", "target\n")
	mkfile(t, root, "sub/c.go", "target\n")

	m, _ := run(t, e, "grep", `{"pattern":"target","glob":"*.go"}`)
	ms := grepMatches(t, m)
	if len(ms) != 2 {
		t.Fatalf("glob *.go: want 2 (a.go, sub/c.go), got %d: %v", len(ms), ms)
	}
	for _, mm := range ms {
		if p := mm["path"].(string); filepath.Ext(p) != ".go" {
			t.Errorf("glob *.go matched a non-.go file: %s", p)
		}
	}
	// A bad glob is a model-visible error.
	if _, isErr := run(t, e, "grep", `{"pattern":"x","glob":"[bad"}`); !isErr {
		t.Error("bad glob should be an error result")
	}
}

// output_mode=files_with_matches returns paths, not lines; count returns
// per-file totals. Both scan the whole tree (no content line cap).
func TestGrepOutputModes(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "a.go", "hit\nhit\nmiss\n")
	mkfile(t, root, "b.go", "hit\n")
	mkfile(t, root, "c.go", "nothing\n")

	// files_with_matches
	m, isErr := run(t, e, "grep", `{"pattern":"hit","output_mode":"files_with_matches"}`)
	if isErr {
		t.Fatalf("files mode errored: %v", m)
	}
	files, _ := m["files"].([]any)
	if len(files) != 2 {
		t.Fatalf("files_with_matches: want 2 files, got %d: %v", len(files), files)
	}
	if m["matches"] != nil {
		t.Error("files mode should not include a matches array")
	}

	// count
	m, _ = run(t, e, "grep", `{"pattern":"hit","output_mode":"count"}`)
	counts, _ := m["counts"].([]any)
	if len(counts) != 2 {
		t.Fatalf("count: want 2 files, got %v", counts)
	}
	byPath := map[string]int{}
	for _, c := range counts {
		cm := c.(map[string]any)
		byPath[cm["path"].(string)] = int(cm["count"].(float64))
	}
	if byPath["a.go"] != 2 || byPath["b.go"] != 1 {
		t.Fatalf("count wrong: %v", byPath)
	}

	// bad output_mode is a model-visible error
	if _, isErr := run(t, e, "grep", `{"pattern":"hit","output_mode":"bogus"}`); !isErr {
		t.Error("bad output_mode should error")
	}
}

// -A/-B/-C context lines (INC-24).
func TestGrepContextLines(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "a.go", "L1\nL2\nMATCH\nL4\nL5\n")

	// -B 2: two lines before.
	m, _ := run(t, e, "grep", `{"pattern":"MATCH","-B":2}`)
	ms := grepMatches(t, m)
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d", len(ms))
	}
	before := toStrs(ms[0]["before"])
	if len(before) != 2 || before[0] != "L1" || before[1] != "L2" {
		t.Errorf("-B 2 before = %v, want [L1 L2]", before)
	}
	if ms[0]["after"] != nil {
		t.Errorf("-B only should have no after: %v", ms[0]["after"])
	}

	// -A 1: one line after.
	m, _ = run(t, e, "grep", `{"pattern":"MATCH","-A":1}`)
	ms = grepMatches(t, m)
	after := toStrs(ms[0]["after"])
	if len(after) != 1 || after[0] != "L4" {
		t.Errorf("-A 1 after = %v, want [L4]", after)
	}

	// -C 1: both sides.
	m, _ = run(t, e, "grep", `{"pattern":"MATCH","-C":1}`)
	ms = grepMatches(t, m)
	if b := toStrs(ms[0]["before"]); len(b) != 1 || b[0] != "L2" {
		t.Errorf("-C 1 before = %v, want [L2]", b)
	}
	if a := toStrs(ms[0]["after"]); len(a) != 1 || a[0] != "L4" {
		t.Errorf("-C 1 after = %v, want [L4]", a)
	}

	// File-boundary: -B 5 on a match near the top clamps, doesn't panic.
	mkfile(t, root, "top.go", "HIT\nx\n")
	m, _ = run(t, e, "grep", `{"pattern":"HIT","-B":5}`)
	ms = grepMatches(t, m)
	if b := toStrs(ms[0]["before"]); len(b) != 0 {
		t.Errorf("-B 5 at file top = %v, want empty (clamped)", b)
	}

	// Default (no context) = old behavior: no before/after keys.
	m, _ = run(t, e, "grep", `{"pattern":"MATCH"}`)
	ms = grepMatches(t, m)
	if ms[0]["before"] != nil || ms[0]["after"] != nil {
		t.Errorf("default grep should carry no context: %v", ms[0])
	}
}

func toStrs(v any) []string {
	raw, _ := v.([]any)
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(string))
	}
	return out
}
