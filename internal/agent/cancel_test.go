package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
)

// Cancellation from above terminates the activity as ActivityCancelled
// (not Completed/Failed), journaled only after the run drained, carrying
// the partial output.
func TestActivityCancelledCarriesPartialOutput(t *testing.T) {
	m := &memAppend{}
	x := testExecutor(m)

	ctx, cancel := context.WithCancel(context.Background())
	drained := false
	done := make(chan error, 1)
	go func() {
		done <- x.Do(ctx, Activity{
			ID: "tool-call_1_0", Kind: event.KindTool, Name: "bash", CallID: "call_1_0",
			Run: func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				<-ctx.Done() // simulates bash: kill group, then return partial output
				drained = true
				return json.RawMessage(`{"stdout":"partial...","canceled":true}`), nil, true, nil
			},
		})
	}()
	cancel()
	err := <-done
	if err == nil || errs.ClassOf(err) != errs.Canceled {
		t.Fatalf("err = %v (class %s), want canceled", err, errs.ClassOf(err))
	}
	if !drained {
		t.Fatal("terminal event must wait for the run to drain")
	}
	want := []string{"activity_started", "activity_cancelled"}
	if got := m.types(); !equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	var cancelled event.ActivityCancelled
	_ = json.Unmarshal(m.events[1].Payload, &cancelled)
	if !strings.Contains(cancelled.PartialOutput, "partial...") {
		t.Errorf("partial output lost: %+v", cancelled)
	}
}

// Cancel with an armed timer: the pending timer is cleaned up and the
// terminal is still ActivityCancelled, not a fabricated timeout.
func TestActivityCancelledNotConfusedWithTimeout(t *testing.T) {
	m := &memAppend{}
	x := testExecutor(m)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- x.Do(ctx, Activity{
			ID: "tool-call_1_0", Kind: event.KindTool, Name: "bash", CallID: "call_1_0",
			Timeout: time.Hour,
			Run: func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
				<-ctx.Done()
				return json.RawMessage(`{"canceled":true}`), nil, true, nil
			},
		})
	}()
	cancel()
	if err := <-done; errs.ClassOf(err) != errs.Canceled {
		t.Fatalf("err = %v, want canceled class", err)
	}
	want := []string{"activity_started", "timer_set", "timer_cancelled", "activity_cancelled"}
	if got := m.types(); !equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}
