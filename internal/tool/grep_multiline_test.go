package tool

import (
	"strings"
	"testing"
)

// A file with a multi-line function body to match across lines.
const multilineSrc = `package p

func Foo(x int) int {
	y := x * 2
	return y
}

func Bar() {}
`

// grep multiline (INC-27, #35): a pattern spanning lines matches only when
// multiline is set; the default stays line-by-line.
func TestGrepMultiline(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "p.go", multilineSrc)

	// Cross-line pattern: "func Foo" ... "return y" straddles lines 3–5.
	// Backslashes are doubled for JSON ("\\s" is the escape for a literal \s).
	crossline := `func Foo[\\s\\S]*?return y`

	// Default (line-by-line): no single line contains both → no match.
	m, isErr := run(t, e, "grep", `{"pattern":"`+crossline+`"}`)
	if isErr {
		t.Fatalf("grep errored: %v", m)
	}
	if got := len(grepMatches(t, m)); got != 0 {
		t.Fatalf("without multiline: want 0 matches for a cross-line pattern, got %d", got)
	}

	// multiline: the whole file is one string → the pattern spans lines.
	m, isErr = run(t, e, "grep", `{"pattern":"`+crossline+`","multiline":true}`)
	if isErr {
		t.Fatalf("multiline grep errored: %v", m)
	}
	ms := grepMatches(t, m)
	if len(ms) != 1 {
		t.Fatalf("multiline: want 1 cross-line match, got %d: %v", len(ms), ms)
	}
	if line := int(ms[0]["line"].(float64)); line != 3 {
		t.Errorf("match line = %d, want 3 (the func Foo line)", line)
	}
	text := ms[0]["text"].(string)
	if !strings.Contains(text, "func Foo") || !strings.Contains(text, "return y") {
		t.Errorf("match text should span from func Foo to return y, got %q", text)
	}
}

// The `m` flag rides along with multiline so `^`/`$` anchor at line
// boundaries, not just string start/end — proving the mode is a strict
// superset of line-by-line anchoring.
func TestGrepMultilineLineAnchors(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "p.go", multilineSrc)

	// "int {" sits at the END of line 3; `$` must anchor there (needs `m`),
	// then the match crosses to "return" on line 5 (needs `s`/whole-file).
	// Backslashes doubled for JSON.
	pat := `int \\{$[\\s\\S]*?return`
	m, isErr := run(t, e, "grep", `{"pattern":"`+pat+`","multiline":true}`)
	if isErr {
		t.Fatalf("multiline grep errored: %v", m)
	}
	if got := len(grepMatches(t, m)); got != 1 {
		t.Fatalf("want 1 match anchoring `$` at a line boundary and crossing lines, got %d", got)
	}
}

// Context lines around a cross-line match are taken from the match's start/end
// lines; case_insensitive composes with multiline.
func TestGrepMultilineContextAndCaseFold(t *testing.T) {
	e, root := newExec(t)
	mkfile(t, root, "p.go", multilineSrc)

	// -B 2 from the match start (line 3) → lines 1–2 ("package p", "").
	m, _ := run(t, e, "grep", `{"pattern":"func Foo[\\s\\S]*?return y","multiline":true,"-B":2}`)
	ms := grepMatches(t, m)
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d", len(ms))
	}
	before, _ := ms[0]["before"].([]any)
	joined := ""
	for _, b := range before {
		joined += b.(string) + "\n"
	}
	if !strings.Contains(joined, "package p") {
		t.Errorf("before-context should include line 1 (package p), got %q", joined)
	}

	// case_insensitive + multiline together.
	m, _ = run(t, e, "grep", `{"pattern":"FUNC FOO[\\s\\S]*?RETURN Y","multiline":true,"case_insensitive":true}`)
	if got := len(grepMatches(t, m)); got != 1 {
		t.Fatalf("case-insensitive multiline: want 1 match, got %d", got)
	}
}
