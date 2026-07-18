package cli

import "testing"

// TestSendScope drives the INC-73 per-command output-scoping state machine
// through the scenarios the concurrent-send findings exposed.
func TestSendScope(t *testing.T) {
	// Unscoped (mySeq 0): render everything, first idle detaches — the legacy
	// behavior that keeps an older daemon / `new` working.
	t.Run("unscoped fallback", func(t *testing.T) {
		s := newSendScope()
		s.onAck(0)
		if s.suppress() {
			t.Fatal("unscoped must render everything")
		}
		if s.onGenStart([]int64{9}) {
			t.Fatal("unscoped never detaches on gen-start")
		}
		if s.suppress() || !s.idleDetaches() {
			t.Fatal("unscoped renders all + detaches at first idle")
		}
	})

	// Our turn is the FIRST to run: render it, detach at its idle.
	t.Run("our turn first", func(t *testing.T) {
		s := newSendScope()
		s.onAck(5)
		if !s.suppress() {
			t.Fatal("before our turn, suppress")
		}
		if s.idleDetaches() {
			t.Fatal("an idle before our turn must NOT detach us")
		}
		s.onGenStart([]int64{5}) // our turn
		if s.suppress() {
			t.Fatal("our turn renders")
		}
		s.onGenStart(nil) // tool-loop continuation stays ours
		if s.suppress() {
			t.Fatal("continuation step of our turn still renders")
		}
		if !s.idleDetaches() {
			t.Fatal("our turn's idle detaches")
		}
	})

	// Our turn is the SECOND to run (another client's turn goes first): suppress
	// the other, ignore its idle, then render ours — the cli-life-01 trap.
	t.Run("our turn second", func(t *testing.T) {
		s := newSendScope()
		s.onAck(6)
		if s.onGenStart([]int64{5}) { // someone else's turn
			t.Fatal("a foreign turn before ours must not detach")
		}
		if !s.suppress() {
			t.Fatal("foreign turn is suppressed")
		}
		if s.idleDetaches() {
			t.Fatal("foreign turn's idle must not detach us")
		}
		s.onGenStart([]int64{6}) // our turn
		if s.suppress() {
			t.Fatal("our turn renders")
		}
		if !s.idleDetaches() {
			t.Fatal("our idle detaches")
		}
	})

	// After our turn, a later foreign turn starts (no idle between): detach.
	t.Run("detach when a later foreign turn begins", func(t *testing.T) {
		s := newSendScope()
		s.onAck(5)
		s.onGenStart([]int64{5}) // ours
		if s.onGenStart([]int64{6}) != true {
			t.Fatal("a foreign turn after ours must detach")
		}
	})

	// Coalesced: one generation consumes both inputs — both followers own it.
	t.Run("coalesced turn owns both", func(t *testing.T) {
		for _, mine := range []int64{5, 6} {
			s := newSendScope()
			s.onAck(mine)
			s.onGenStart([]int64{5, 6})
			if s.suppress() {
				t.Fatalf("coalesced turn must be ours for seq %d", mine)
			}
		}
	})
}
