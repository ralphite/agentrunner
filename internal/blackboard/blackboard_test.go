package blackboard

import (
	"fmt"
	"sync"
	"testing"
)

func TestPublishReadOrder(t *testing.T) {
	b := New()
	b.Publish("plan", "lead", "first")
	b.Publish("findings", "researcher", "other topic")
	b.Publish("plan", "reviewer", "second")

	notes := b.Read("plan")
	if len(notes) != 2 || notes[0].Text != "first" || notes[1].Text != "second" {
		t.Fatalf("notes = %+v", notes)
	}
	if notes[0].Seq >= notes[1].Seq {
		t.Errorf("seq not monotonic: %+v", notes)
	}
	if notes[1].From != "reviewer" {
		t.Errorf("from = %q", notes[1].From)
	}
	if got := b.Read("empty"); len(got) != 0 {
		t.Errorf("empty topic = %+v", got)
	}
}

// The mirror sees every publish, in order, outside the lock — re-reading
// the board from the mirror callback must not deadlock.
func TestPublishMirror(t *testing.T) {
	b := New()
	var seen []Note
	b.Mirror = func(n Note) {
		_ = b.Read(n.Topic) // lock re-entry check: must not deadlock
		seen = append(seen, n)
	}
	b.Publish("plan", "lead", "step one")
	b.Publish("plan", "worker", "done")
	if len(seen) != 2 || seen[0].Text != "step one" || seen[1].Seq != 2 {
		t.Fatalf("mirror saw %+v", seen)
	}
}

func TestReadReturnsCopy(t *testing.T) {
	b := New()
	b.Publish("t", "a", "original")
	got := b.Read("t")
	got[0].Text = "mutated"
	if b.Read("t")[0].Text != "original" {
		t.Error("Read must return a copy")
	}
}

func TestConcurrentPublish(t *testing.T) {
	b := New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b.Publish("t", "w", fmt.Sprintf("note-%d", i))
		}(i)
	}
	wg.Wait()
	notes := b.Read("t")
	if len(notes) != 50 {
		t.Fatalf("len = %d", len(notes))
	}
	seen := map[int]bool{}
	for _, n := range notes {
		if seen[n.Seq] {
			t.Fatalf("duplicate seq %d", n.Seq)
		}
		seen[n.Seq] = true
	}
}
