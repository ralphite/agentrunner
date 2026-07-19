package agent

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// The registry is the single source of interrupt-resolution literals
// (INC-69): production sites in conversation.go/approval.go read
// WaitRules[kind].OnInterrupt instead of hardcoding the strings, so these
// pinned values ARE the wire contract — change them and the journaled
// resolutions change with them.
func TestWaitRulesAreResolutionSource(t *testing.T) {
	cases := []struct {
		kind          string
		interruptible bool
		onInterrupt   string
		onSteer       string
		resolvedBy    string
	}{
		{event.WaitInput, true, "superseded_by_interrupt", "", "input"},
		// INC-70 Option B: a user-class message at the approval park
		// supersedes the pending ask (denied_by_steer).
		{event.WaitApproval, true, "denied_by_interrupt", "denied_by_steer", "approval_response"},
	}
	if len(cases) != len(WaitRules) {
		t.Fatalf("registry has %d rows, table pins %d", len(WaitRules), len(cases))
	}
	for _, tc := range cases {
		rule, ok := WaitRules[tc.kind]
		if !ok {
			t.Errorf("kind %q missing from registry", tc.kind)
			continue
		}
		if rule.Interruptible != tc.interruptible ||
			rule.OnInterrupt != tc.onInterrupt || rule.OnSteer != tc.onSteer ||
			rule.ResolvedBy != tc.resolvedBy {
			t.Errorf("row %q = %+v, want %+v", tc.kind, rule, tc)
		}
	}
}

// The S2 exit criterion, synthetic edition: a waiting state journaled by
// one process is visible to the next (kill → reopen → fold → still idle),
// and the drive loop refuses to run past it.
func TestWaitingSurvivesProcessBoundary(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sess")
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		typ     string
		payload any
	}{
		{event.TypeSessionStarted, &event.SessionStarted{SpecName: "t", SubStateVersions: state.SubStateVersions()}},
		{event.TypeWaitingEntered, &event.WaitingEntered{Kind: event.WaitApproval,
			Detail: json.RawMessage(`{"call_id":"call_1_0"}`)}},
	} {
		env, err := event.New(pair.typ, pair.payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := es.Append(env); err != nil {
			t.Fatal(err)
		}
	}
	_ = es.Close() // process boundary

	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := state.Fold(events)
	if err != nil {
		t.Fatal(err)
	}
	if s.Waiting == nil || s.Waiting.Kind != event.WaitApproval || s.Session.Status != state.StatusWaiting {
		t.Fatalf("state across process boundary: %+v", s.Session)
	}
	if got := decide(s, 5); got.kind != doWait {
		t.Fatalf("decide on idle state = %+v, want doWait", got)
	}
}
