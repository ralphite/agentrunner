package notify

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Dedup holds within one Notifier AND across a reopen: the sent set folds
// from the notifier's own stream.
func TestNotifyDedupAcrossReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notifier")
	outFile := filepath.Join(t.TempDir(), "delivered.log")
	cmd := []string{"sh", "-c", "cat >> " + outFile}

	var errOut bytes.Buffer
	n, err := Open(dir, cmd, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	n.Notify(Notification{Key: "run_end/s1", Kind: "run_end", Session: "s1", Text: "run s1 completed"})
	n.Notify(Notification{Key: "run_end/s1", Kind: "run_end", Session: "s1", Text: "dup"})
	n.Notify(Notification{Key: "approval/s1/apr-1", Kind: "approval", Session: "s1", Text: "needs a decision"})
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), "\n"); got != 2 {
		t.Fatalf("delivered %d lines, want 2 (dup dropped):\n%s", got, raw)
	}
	if !strings.Contains(string(raw), "run s1 completed") || strings.Contains(string(raw), `"dup"`) {
		t.Fatalf("delivery content wrong:\n%s", raw)
	}

	// Reopen: the journal remembers; the same keys stay silent.
	n2, err := Open(dir, cmd, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = n2.Close() }()
	if !n2.Seen("run_end/s1") || !n2.Seen("approval/s1/apr-1") {
		t.Fatal("sent set did not survive reopen")
	}
	n2.Notify(Notification{Key: "run_end/s1", Kind: "run_end", Text: "post-restart dup"})
	raw2, _ := os.ReadFile(outFile)
	if strings.Contains(string(raw2), "post-restart dup") {
		t.Fatal("dedup failed across restart")
	}
}

// No command configured → the stderr fallback carries the notification.
func TestNotifyStderrFallback(t *testing.T) {
	var errOut bytes.Buffer
	n, err := Open(filepath.Join(t.TempDir(), "n"), nil, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = n.Close() }()
	n.Notify(Notification{Key: "k1", Kind: "run_end", Text: "the run ended"})
	if !strings.Contains(errOut.String(), "the run ended") {
		t.Fatalf("stderr fallback missing: %q", errOut.String())
	}
}

// A failing command falls back to stderr — the notification is never
// silently lost.
func TestNotifyCommandFailureFallsBack(t *testing.T) {
	var errOut bytes.Buffer
	n, err := Open(filepath.Join(t.TempDir(), "n"), []string{"false"}, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = n.Close() }()
	n.Notify(Notification{Key: "k1", Kind: "approval", Text: "please decide"})
	out := errOut.String()
	if !strings.Contains(out, "command failed") || !strings.Contains(out, "please decide") {
		t.Fatalf("fallback missing: %q", out)
	}
}
