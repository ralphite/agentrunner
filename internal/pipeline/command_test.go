package pipeline

import (
	"context"
	"reflect"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
)

// --- unit: splitCompound ---

func TestSplitCompound(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"git status", []string{"git status"}},
		{"git add && git commit", []string{"git add", "git commit"}},
		{"a ; b ; c", []string{"a", "b", "c"}},
		{"a || b", []string{"a", "b"}},
		{"cat f | grep x", []string{"cat f", "grep x"}},
		{"build &", []string{"build"}},
		{"a |& b", []string{"a", "b"}},
		{"a\nb", []string{"a", "b"}},
		// quotes hide separators
		{`echo "a && b"`, []string{`echo "a && b"`}},
		{`echo 'x ; y'`, []string{`echo 'x ; y'`}},
		// unbalanced quote → fail-safe whole-command single segment
		{`echo "oops && rm`, []string{`echo "oops && rm`}},
	}
	for _, tc := range cases {
		if got := splitCompound(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCompound(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- unit: stripWrappers ---

func TestStripWrappers(t *testing.T) {
	cases := []struct{ in, want string }{
		{"npm test", "npm test"},
		{"timeout 60 npm test", "npm test"},
		{"timeout -k 5 60 npm test", "npm test"},
		{"time go build", "go build"},
		{"nice -n 10 make", "make"},
		{"nohup server", "server"},
		{"xargs rm", "rm"},
		{"xargs -0 rm", "xargs -0 rm"}, // flagged xargs is NOT stripped (fail-safe)
		{"nice time npm test", "npm test"},
		{"timeout", "timeout"}, // malformed (nothing after) — left as-is
	}
	for _, tc := range cases {
		if got := stripWrappers(tc.in); got != tc.want {
			t.Errorf("stripWrappers(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- unit: isReadOnlyCommand ---

func TestIsReadOnlyCommand(t *testing.T) {
	ro := []string{"ls -la", "cat file", "pwd", "grep -r x .", "find . -name '*.go'", "wc -l f", "true"}
	for _, s := range ro {
		if !isReadOnlyCommand(s) {
			t.Errorf("isReadOnlyCommand(%q) = false, want true", s)
		}
	}
	notRO := []string{
		"rm -rf x", "npm install", "git push",
		`find . -exec rm {} \;`, // find that EXECUTES
		"find . -delete",        // find that DELETES
		"cat f > g",             // redirection = can write
		"echo hi > out",         // redirection
		"cat $(evil)",           // command substitution
		"ls `whoami`",           // backtick substitution
		"",
	}
	for _, s := range notRO {
		if isReadOnlyCommand(s) {
			t.Errorf("isReadOnlyCommand(%q) = true, want false", s)
		}
	}
}

// --- integration: per-segment adjudication (the security fix) ---

// SECURITY: a compound where one segment matches an allow rule but another
// segment does not must NOT be allowed — the unmatched segment falls to the
// mode default (ask), which dominates.
func TestCompoundSplitTakesStrictest(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Rules: []PermissionRule{
		{Tool: "bash", Command: "git *", Action: "allow"},
	}}
	// git segment allowed, rm segment unmatched → default ask dominates.
	d := g.Check(context.Background(), toolEffect("bash", "execute", `{"command":"git status && rm -rf x"}`))
	if d.Action != event.VerdictAsk {
		t.Fatalf("compound with unmatched rm segment = %+v, want ask (strictest)", d)
	}
}

// A hard deny on any segment denies the whole compound.
func TestCompoundSegmentDenyDominates(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Rules: []PermissionRule{
		{Tool: "bash", Command: "git *", Action: "allow"},
		{Tool: "bash", Command: "rm *", Action: "deny"},
	}}
	d := g.Check(context.Background(), toolEffect("bash", "execute", `{"command":"git status && rm -rf x"}`))
	if d.Action != event.VerdictDeny {
		t.Fatalf("compound with rm-deny segment = %+v, want deny", d)
	}
}

// When EVERY segment is covered by an allow, the compound is allowed.
func TestCompoundAllSegmentsCovered(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Rules: []PermissionRule{
		{Tool: "bash", Command: "git *", Action: "allow"},
	}}
	d := g.Check(context.Background(), toolEffect("bash", "execute", `{"command":"git add . && git commit -m x"}`))
	if d.Action != event.VerdictAllow {
		t.Fatalf("all-git compound = %+v, want allow", d)
	}
}

// A read-only builtin segment needs no rule; it does not hold back a compound
// whose other segments are allowed.
func TestCompoundReadonlySegmentIsAllowed(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Rules: []PermissionRule{
		{Tool: "bash", Command: "git *", Action: "allow"},
	}}
	// `ls` matches no rule but is read-only → allow; `git status` allowed.
	d := g.Check(context.Background(), toolEffect("bash", "execute", `{"command":"ls && git status"}`))
	if d.Action != event.VerdictAllow {
		t.Fatalf("readonly + git compound = %+v, want allow", d)
	}
}

// A wrapper must not defeat a command rule.
func TestWrapperStrippedForMatch(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Rules: []PermissionRule{
		{Tool: "bash", Command: "npm test", Action: "allow"},
	}}
	d := g.Check(context.Background(), toolEffect("bash", "execute", `{"command":"timeout 60 npm test"}`))
	if d.Action != event.VerdictAllow {
		t.Fatalf("timeout-wrapped npm test = %+v, want allow via strip", d)
	}
}

// A single read-only command needs no rule at all (empty rule set → default
// ask, but ls is allowed anyway).
func TestSingleReadonlyNeedsNoRule(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Rules: nil}
	if d := g.Check(context.Background(), toolEffect("bash", "execute", `{"command":"ls -la"}`)); d.Action != event.VerdictAllow {
		t.Fatalf("bare ls = %+v, want allow (read-only set)", d)
	}
	// but a non-read-only command still falls to the default ask.
	if d := g.Check(context.Background(), toolEffect("bash", "execute", `{"command":"npm install"}`)); d.Action != event.VerdictAsk {
		t.Fatalf("npm install with no rule = %+v, want default ask", d)
	}
}

// SECURITY: a quoted separator must not be treated as a split point that
// leaves a dangerous tail unadjudicated.
func TestQuotedSeparatorStaysOneSegment(t *testing.T) {
	g := &PermissionGate{WS: newPermWS(t), Rules: []PermissionRule{
		{Tool: "bash", Command: "echo *", Action: "allow"},
	}}
	// The whole thing is one echo segment; it matches echo* → allow. If we
	// wrongly split on the quoted &&, the "rm" tail would be a separate
	// unmatched segment (ask) — assert we do NOT do that.
	d := g.Check(context.Background(), toolEffect("bash", "execute", `{"command":"echo \"a && rm b\""}`))
	if d.Action != event.VerdictAllow {
		t.Fatalf("quoted-separator echo = %+v, want allow (one segment)", d)
	}
}
