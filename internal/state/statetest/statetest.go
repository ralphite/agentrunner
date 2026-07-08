// Package statetest holds test helpers for fold assertions. All later
// fold-equality checks (2.13 snapshot-resume equivalence, crash-matrix
// assertions) go through AssertFoldEqual so failures show WHICH sub-state
// diverged, not a wall of struct dump.
package statetest

import (
	"encoding/json"
	"testing"

	"github.com/ralphite/agentrunner/internal/state"
)

// AssertFoldEqual compares two states namespace by namespace in their JSON
// form. JSON comparison is deliberate: snapshots round-trip through JSON,
// so nil-vs-empty-map differences must not count as divergence.
func AssertFoldEqual(t testing.TB, got, want state.State) {
	t.Helper()
	pairs := []struct {
		name      string
		got, want any
	}{
		{"conversation", got.Conversation, want.Conversation},
		{"activities", got.Activities, want.Activities},
		{"waiting", got.Waiting, want.Waiting},
		{"timers", got.Timers, want.Timers},
		{"session", got.Session, want.Session},
		{"effects", got.Effects, want.Effects},
		{"mode", got.Mode, want.Mode},
		{"budget", got.Budget, want.Budget},
		{"compaction", got.Compaction, want.Compaction},
		{"handles", got.Handles, want.Handles},
		{"barriers", got.Barriers, want.Barriers},
	}
	for _, p := range pairs {
		g, w := mustJSON(t, p.got), mustJSON(t, p.want)
		if g != w {
			t.Errorf("fold sub-state %q diverges:\n got: %s\nwant: %s", p.name, g, w)
		}
	}
}

// mustJSON renders a normalized JSON form: empty maps/slices and explicit
// nulls collapse away, so `{}`, `null`, and an absent key all compare equal.
func mustJSON(t testing.TB, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("statetest: marshal: %v", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("statetest: %v", err)
	}
	out, err := json.MarshalIndent(normalize(decoded), "      ", "  ")
	if err != nil {
		t.Fatalf("statetest: %v", err)
	}
	return string(out)
}

func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, vv := range x {
			if n := normalize(vv); n != nil {
				out[k] = n
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		if len(x) == 0 {
			return nil
		}
		out := make([]any, len(x))
		for i, vv := range x {
			out[i] = normalize(vv)
		}
		return out
	}
	return v
}
