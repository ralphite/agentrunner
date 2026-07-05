package agent

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// The registry is complete from S2 on: every kind has a row, and the
// producibility column matches the stage plan.
func TestWaitRegistryTable(t *testing.T) {
	cases := []struct {
		kind          string
		producible    int
		interruptible bool
		onInterrupt   string
		resolvedBy    string
	}{
		{event.WaitInput, 4, true, "superseded_by_interrupt", "input"},
		{event.WaitApproval, 3, true, "denied_by_interrupt", "approval_response"},
		{event.WaitTasks, 6, true, "tasks_cancelled_by_interrupt", "tasks_done"},
		{event.WaitTimer, 6, true, "timer_cancelled_by_interrupt", "timer_fired"},
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
		if rule.ProducibleStage != tc.producible || rule.Interruptible != tc.interruptible ||
			rule.OnInterrupt != tc.onInterrupt || rule.ResolvedBy != tc.resolvedBy {
			t.Errorf("row %q = %+v, want %+v", tc.kind, rule, tc)
		}
		// S2 may produce none of them.
		if CanProduce(tc.kind, 2) {
			t.Errorf("kind %q must not be producible in S2", tc.kind)
		}
		if !CanProduce(tc.kind, tc.producible) {
			t.Errorf("kind %q must be producible from stage %d", tc.kind, tc.producible)
		}
	}
}

// Every cell of the interrupt column, driven with synthetic WaitingEntered
// events (nothing can produce them yet — that is the point).
func TestInterruptResolvesEveryKind(t *testing.T) {
	for kind, rule := range WaitRules {
		t.Run(kind, func(t *testing.T) {
			m := &memAppend{}
			s := state.New()
			entered, err := event.New(event.TypeWaitingEntered, &event.WaitingEntered{Kind: kind})
			if err != nil {
				t.Fatal(err)
			}
			entered.Seq = 1
			if s, err = state.Apply(s, entered); err != nil {
				t.Fatal(err)
			}

			appendE := func(typ string, payload any) (event.Envelope, error) {
				env, aerr := m.append(typ, payload)
				if aerr != nil {
					return env, aerr
				}
				s, aerr = state.Apply(s, env)
				return env, aerr
			}
			if err := ResolveWaitingOnInterrupt(s, appendE); err != nil {
				t.Fatal(err)
			}

			// journal-inputs-first: the interrupt lands BEFORE the resolution.
			want := []string{"input_received", "waiting_resolved"}
			if got := m.types(); !equal(got, want) {
				t.Fatalf("events = %v, want %v", got, want)
			}
			var resolved event.WaitingResolved
			_ = json.Unmarshal(m.events[1].Payload, &resolved)
			if resolved.Kind != kind || resolved.Resolution != rule.OnInterrupt {
				t.Errorf("resolved = %+v", resolved)
			}
			if s.Waiting != nil || s.Run.Status != state.StatusRunning {
				t.Errorf("state after interrupt: waiting=%+v status=%s", s.Waiting, s.Run.Status)
			}
			// Control input must not pollute the transcript.
			if len(s.Conversation.Messages) != 0 {
				t.Errorf("interrupt leaked into conversation: %+v", s.Conversation.Messages)
			}
		})
	}
}

func TestInterruptOnNonWaitingIsNoop(t *testing.T) {
	m := &memAppend{}
	if err := ResolveWaitingOnInterrupt(state.New(), m.append); err != nil {
		t.Fatal(err)
	}
	if len(m.events) != 0 {
		t.Errorf("events = %v, want none", m.types())
	}
}

func TestInterruptUnknownKindErrors(t *testing.T) {
	s := state.New()
	s.Waiting = &state.Waiting{Kind: "seance"}
	if err := ResolveWaitingOnInterrupt(s, (&memAppend{}).append); err == nil {
		t.Fatal("unknown waiting kind must error")
	}
}

// The S2 exit criterion, synthetic edition: a waiting state journaled by
// one process is visible to the next (kill → reopen → fold → still parked),
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
		{event.TypeRunStarted, &event.RunStarted{SpecName: "t", SubStateVersions: state.SubStateVersions()}},
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
	if s.Waiting == nil || s.Waiting.Kind != event.WaitApproval || s.Run.Status != state.StatusWaiting {
		t.Fatalf("state across process boundary: %+v", s.Run)
	}
	if got := decide(s, 5, "", false); got.kind != doWait {
		t.Fatalf("decide on parked state = %+v, want doWait", got)
	}
}
