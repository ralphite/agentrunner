package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/store"
)

func diffEnv(t *testing.T, seq int64, typ string, payload any) event.Envelope {
	t.Helper()
	env, err := event.New(typ, payload)
	if err != nil {
		t.Fatal(err)
	}
	env.Seq = seq
	return env
}

func TestPlanLastTurnDiffBaseline(t *testing.T) {
	ref := strings.Repeat("a", 40)
	events := []event.Envelope{
		diffEnv(t, 1, event.TypeInputReceived, &event.InputReceived{Text: "first", Source: "cli"}),
		diffEnv(t, 2, event.TypeCheckpointBarrier, &event.CheckpointBarrier{BarrierID: "bar-t1", SnapshotRef: strings.Repeat("b", 40)}),
		diffEnv(t, 3, event.TypeInputReceived, &event.InputReceived{Text: "goal continuation", Source: "program"}),
		diffEnv(t, 4, event.TypeInputReceived, &event.InputReceived{Text: "latest", Source: "user"}),
		diffEnv(t, 5, event.TypeInputReceived, &event.InputReceived{Text: "worker mail", Source: "agent"}),
		diffEnv(t, 6, event.TypeInputReceived, &event.InputReceived{Text: "webhook", Source: "machine"}),
		// Explicit/manual and final barriers are after arbitrary work; neither
		// may masquerade as the generation-start baseline.
		diffEnv(t, 7, event.TypeCheckpointBarrier, &event.CheckpointBarrier{BarrierID: "bar-m7", SnapshotRef: strings.Repeat("d", 40)}),
		diffEnv(t, 8, event.TypeCheckpointBarrier, &event.CheckpointBarrier{BarrierID: "bar-final", SnapshotRef: strings.Repeat("c", 40)}),
		diffEnv(t, 9, event.TypeCheckpointBarrier, &event.CheckpointBarrier{BarrierID: "bar-t2", SnapshotRef: ref}),
	}
	got, reason, err := planLastTurnDiffBaseline(events)
	if err != nil || reason != "" || got == nil {
		t.Fatalf("baseline = %+v reason=%q err=%v", got, reason, err)
	}
	if got.InputSeq != 4 || got.BarrierSeq != 9 || got.BarrierID != "bar-t2" || got.SnapshotRef != ref {
		t.Fatalf("wrong baseline: %+v", got)
	}

	for _, source := range []string{"", "user", "cli", "unix-socket"} {
		t.Run("human_"+source, func(t *testing.T) {
			evs := []event.Envelope{
				diffEnv(t, 1, event.TypeInputReceived, &event.InputReceived{Source: source}),
				diffEnv(t, 2, event.TypeCheckpointBarrier, &event.CheckpointBarrier{BarrierID: "bar-t1", SnapshotRef: ref}),
			}
			base, _, err := planLastTurnDiffBaseline(evs)
			if err != nil || base == nil || base.InputSeq != 1 {
				t.Fatalf("source %q: base=%+v err=%v", source, base, err)
			}
		})
	}

	if got, reason, err := planLastTurnDiffBaseline(events[:8]); err != nil || got != nil || !strings.Contains(reason, "no durable") {
		t.Fatalf("no barrier: got=%+v reason=%q err=%v", got, reason, err)
	}
	if got, reason, err := planLastTurnDiffBaseline(events[2:4]); err != nil || got != nil || !strings.Contains(reason, "no durable") {
		t.Fatalf("human without barrier: got=%+v reason=%q err=%v", got, reason, err)
	}
	if got, reason, err := planLastTurnDiffBaseline(events[2:3]); err != nil || got != nil || !strings.Contains(reason, "no human") {
		t.Fatalf("machine-only: got=%+v reason=%q err=%v", got, reason, err)
	}
}

func TestPlanLastTurnDiffPairsMessageSnapshotsByTurnID(t *testing.T) {
	start := strings.Repeat("a", 40)
	end := strings.Repeat("b", 40)
	other := strings.Repeat("c", 40)
	events := []event.Envelope{
		diffEnv(t, 1, event.TypeCheckpointBarrier, &event.CheckpointBarrier{
			BarrierID: "bar-msg-before-user", SnapshotRef: start,
			MessageAnchor: &event.MessageAnchor{Side: "before_user", ItemID: "u1", TurnID: "turn-1"},
		}),
		diffEnv(t, 2, event.TypeInputReceived, &event.InputReceived{
			Text: "work", Source: "user", ItemID: "u1", TurnID: "turn-1",
		}),
		diffEnv(t, 3, event.TypeCheckpointBarrier, &event.CheckpointBarrier{
			BarrierID: "bar-wrong-turn", SnapshotRef: other,
			MessageAnchor: &event.MessageAnchor{Side: "after_assistant", ItemID: "a0", TurnID: "turn-0"},
		}),
		diffEnv(t, 4, event.TypeAssistantMessage, &event.AssistantMessage{
			TurnID: "turn-1", ItemID: "a1",
			Message: provider.Message{Parts: []provider.Part{{Kind: provider.PartText, Text: "done"}}},
		}),
		diffEnv(t, 5, event.TypeCheckpointBarrier, &event.CheckpointBarrier{
			BarrierID: "bar-msg-after-assistant", SnapshotRef: end,
			MessageAnchor: &event.MessageAnchor{Side: "after_assistant", ItemID: "a1", TurnID: "turn-1"},
		}),
	}
	got, reason, err := planLastTurnDiffBaseline(events)
	if err != nil || reason != "" || got == nil {
		t.Fatalf("baseline = %+v reason=%q err=%v", got, reason, err)
	}
	if got.TurnID != "turn-1" || got.SnapshotRef != start || got.EndSnapshotRef != end ||
		!got.Completed || got.BarrierSeq != 1 || got.EndBarrierSeq != 5 {
		t.Fatalf("wrong same-turn comparison: %+v", got)
	}

	active, reason, err := planLastTurnDiffBaseline(events[:2])
	if err != nil || reason != "" || active == nil || active.Completed || active.EndSnapshotRef != "" {
		t.Fatalf("active baseline = %+v reason=%q err=%v", active, reason, err)
	}

	if got, reason, err := planLastTurnDiffBaseline(events[:4]); err != nil || got != nil ||
		!strings.Contains(reason, "no durable end") {
		t.Fatalf("completed without end: got=%+v reason=%q err=%v", got, reason, err)
	}

	cleanEmpty := append([]event.Envelope{}, events[:3]...)
	cleanEmpty = append(cleanEmpty, diffEnv(t, 4, event.TypeAssistantMessage, &event.AssistantMessage{
		TurnID: "turn-1", ItemID: "a-empty",
		Message: provider.Message{},
	}))
	if got, reason, err := planLastTurnDiffBaseline(cleanEmpty); err != nil || got != nil ||
		!strings.Contains(reason, "no durable end") {
		t.Fatalf("clean empty completion without end: got=%+v reason=%q err=%v", got, reason, err)
	}
}

func TestCLIDiffLastTurnJSON(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg"))
	ws := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "same.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	shadow, err := openShadow(ws)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := shadow.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "same.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "new.txt"), []byte("created\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "large.txt"), []byte(strings.Repeat("x", 256*1024+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "binary.bin"), []byte{'a', 0, 'b'}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "node_modules", "pkg", "index.js"), []byte("generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	id := "20260711-120000-last-turn-diff-aaaa"
	dir := filepath.Join(mustDataDir(t), "sessions", id)
	es, err := store.OpenEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		typ string
		p   any
	}{
		{event.TypeSessionStarted, &event.SessionStarted{SpecName: "qa", Model: "scripted", Prompt: "last turn", WorkspaceRoot: ws}},
		{event.TypeInputReceived, &event.InputReceived{Text: "change files", Source: "cli"}},
		{event.TypeGenerationStarted, &event.GenerationStarted{GenStep: 1}},
		{event.TypeCheckpointBarrier, &event.CheckpointBarrier{BarrierID: "bar-t1", GenStep: 1, SnapshotRef: ref}},
	} {
		env, err := event.New(item.typ, item.p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := es.Append(env); err != nil {
			t.Fatal(err)
		}
	}
	if err := es.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"diff", id, "--scope", "last-turn", "--json"}, "dev", &stdout, &stderr); code != ExitOK {
		t.Fatalf("diff exit=%d stderr=%s", code, stderr.String())
	}
	var got lastTurnDiffResponse
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	// HiddenUntracked is 0, not 1, since G62: node_modules is excluded from the
	// snapshot itself rather than staged-then-hidden, so no addition remains to
	// hide. That already WAS the behavior for any workspace whose .gitignore
	// listed node_modules; the harness-level floor just makes it uniform. See
	// snapshot.TestReviewHidesVendorButStillSnapshotsIt for the hidden-count path.
	if !got.Available || got.Workspace != ws || got.BarrierID != "bar-t1" ||
		!strings.Contains(got.Diff, "same.txt") || !strings.Contains(got.Diff, "new.txt") ||
		!strings.Contains(got.Numstat, "new.txt") || strings.Contains(got.Diff, "node_modules") ||
		strings.Contains(got.Diff, "large.txt") || strings.Contains(got.Diff, "binary.bin") ||
		strings.Join(got.Untracked, ",") != "binary.bin,large.txt" ||
		got.UntrackedReasons["binary.bin"] != "binary" || got.UntrackedReasons["large.txt"] != "large" ||
		got.HiddenUntracked != 0 {
		t.Fatalf("response = %+v", got)
	}
}

func TestCLIDiffCompletedTurnStopsAtAfterAssistantSnapshot(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg"))
	ws := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "state.txt"), []byte("A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	shadow, err := openShadow(ws)
	if err != nil {
		t.Fatal(err)
	}
	refA, err := shadow.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "state.txt"), []byte("B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	refB, err := shadow.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "state.txt"), []byte("C\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "later.txt"), []byte("not this turn\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		id      string
		endRef  string
		want    string
		forbids []string
	}{
		{
			name: "A_to_A_ignores_live_C", id: "20260711-120001-completed-same-aaaa",
			endRef: refA, forbids: []string{"state.txt", "later.txt", "+C"},
		},
		{
			name: "A_to_B_ignores_live_C", id: "20260711-120002-completed-change-bbbb",
			endRef: refB, want: "+B", forbids: []string{"+C", "later.txt"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(mustDataDir(t), "sessions", tc.id)
			es, err := store.OpenEventStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			items := []struct {
				typ string
				p   any
			}{
				{event.TypeSessionStarted, &event.SessionStarted{SpecName: "qa", Model: "scripted", Prompt: "turn", WorkspaceRoot: ws}},
				{event.TypeCheckpointBarrier, &event.CheckpointBarrier{
					BarrierID: "bar-msg-before-u1", SnapshotRef: refA,
					MessageAnchor: &event.MessageAnchor{Side: "before_user", ItemID: "u1", TurnID: "turn-1"},
				}},
				{event.TypeInputReceived, &event.InputReceived{Text: "work", Source: "user", ItemID: "u1", TurnID: "turn-1"}},
				{event.TypeAssistantMessage, &event.AssistantMessage{
					TurnID: "turn-1", ItemID: "a1",
					Message: provider.Message{Parts: []provider.Part{{Kind: provider.PartText, Text: "done"}}},
				}},
				{event.TypeCheckpointBarrier, &event.CheckpointBarrier{
					BarrierID: "bar-msg-after-a1", SnapshotRef: tc.endRef,
					MessageAnchor: &event.MessageAnchor{Side: "after_assistant", ItemID: "a1", TurnID: "turn-1"},
				}},
			}
			for _, item := range items {
				env, err := event.New(item.typ, item.p)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := es.Append(env); err != nil {
					t.Fatal(err)
				}
			}
			if err := es.Close(); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			if code := Run([]string{"diff", tc.id, "--scope", "last-turn", "--json"}, "dev", &stdout, &stderr); code != ExitOK {
				t.Fatalf("diff exit=%d stderr=%s", code, stderr.String())
			}
			var got lastTurnDiffResponse
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("json: %v\n%s", err, stdout.String())
			}
			if !got.Available || !got.Completed || got.EndBarrierID != "bar-msg-after-a1" {
				t.Fatalf("response = %+v", got)
			}
			if tc.want != "" && !strings.Contains(got.Diff, tc.want) {
				t.Errorf("diff missing %q:\n%s", tc.want, got.Diff)
			}
			for _, forbidden := range tc.forbids {
				if strings.Contains(got.Diff, forbidden) || strings.Contains(got.Numstat, forbidden) {
					t.Errorf("completed diff leaked %q:\ndiff=%s\nnumstat=%s", forbidden, got.Diff, got.Numstat)
				}
			}
		})
	}
}
