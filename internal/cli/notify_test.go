package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/notify"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/store"
)

// Startup reconciliation notifies every session idle on an approval —
// once: the journaled sent set silences the second daemon start.
func TestReconcileNotificationsIdleApproval(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// Synthesize a session idle in WAITING_APPROVAL.
	dir, err := runtime.SessionDir("sess-idle")
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	journal := func(typ string, p any) {
		t.Helper()
		env, err := event.New(typ, p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := es.Append(env); err != nil {
			t.Fatal(err)
		}
	}
	journal(event.TypeSessionStarted, &event.SessionStarted{SpecName: "x", Model: "m", Task: "t"})
	detail, _ := json.Marshal(event.ApprovalRequested{ApprovalID: "apr-42", EffectID: "eff-x"})
	journal(event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitApproval, Detail: detail})
	if err := es.Close(); err != nil {
		t.Fatal(err)
	}

	data, _ := runtime.DataDir()
	var errOut bytes.Buffer
	n, err := notify.Open(filepath.Join(data, "notifier"), nil, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	reconcileNotifications(n)
	if !strings.Contains(errOut.String(), "apr-42") || !strings.Contains(errOut.String(), "sess-idle") {
		t.Fatalf("reconcile missed the idle approval: %q", errOut.String())
	}
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}

	// Second start: already notified, stays silent.
	var errOut2 bytes.Buffer
	n2, err := notify.Open(filepath.Join(data, "notifier"), nil, &errOut2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = n2.Close() }()
	reconcileNotifications(n2)
	if strings.Contains(errOut2.String(), "apr-42") {
		t.Fatalf("reconcile double-notified: %q", errOut2.String())
	}
}

// An ended session is not reconciled (documented cut: adopting the notifier
// must not replay history), and unreadable session dirs are skipped.
func TestReconcileSkipsEndedAndGarbage(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir, err := runtime.SessionDir("sess-done")
	if err != nil {
		t.Fatal(err)
	}
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []struct {
		typ string
		p   any
	}{
		{event.TypeSessionStarted, &event.SessionStarted{SpecName: "x", Model: "m", Task: "t"}},
		{event.TypeSessionClosed, &event.SessionClosed{Reason: "closed", Source: "user"}},
	} {
		env, _ := event.New(ev.typ, ev.p)
		if _, err := es.Append(env); err != nil {
			t.Fatal(err)
		}
	}
	_ = es.Close()
	// Garbage dir alongside.
	data, _ := runtime.DataDir()
	if err := os.MkdirAll(filepath.Join(data, "sessions", "not-a-session"), 0o700); err != nil {
		t.Fatal(err)
	}

	var errOut bytes.Buffer
	n, err := notify.Open(filepath.Join(data, "notifier"), nil, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = n.Close() }()
	reconcileNotifications(n)
	if strings.Contains(errOut.String(), "sess-done") {
		t.Fatalf("ended session reconciled: %q", errOut.String())
	}
}
