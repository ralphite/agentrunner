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
	if !got.Available || got.Workspace != ws || got.BarrierID != "bar-t1" ||
		!strings.Contains(got.Diff, "same.txt") || !strings.Contains(got.Diff, "new.txt") ||
		!strings.Contains(got.Numstat, "new.txt") || strings.Contains(got.Diff, "node_modules") ||
		strings.Contains(got.Diff, "large.txt") || strings.Contains(got.Diff, "binary.bin") ||
		strings.Join(got.Untracked, ",") != "binary.bin,large.txt" ||
		got.UntrackedReasons["binary.bin"] != "binary" || got.UntrackedReasons["large.txt"] != "large" ||
		got.HiddenUntracked != 1 {
		t.Fatalf("response = %+v", got)
	}
}
