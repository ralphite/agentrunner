package store

import (
	"bytes"
	"testing"

	"github.com/ralphite/agentrunner/internal/protocol"
)

// v2 收口: mailbox seqs are monotonic, reads filter by high-water mark, and
// attachments round-trip byte-exact.
func TestInboxAppendRead(t *testing.T) {
	dir := t.TempDir()
	a, err := AppendInbox(dir, protocol.UserInput{Text: "one"})
	if err != nil || a.DeliverySeq != 1 {
		t.Fatalf("first append: %+v, %v", a, err)
	}
	b, err := AppendInbox(dir, protocol.UserInput{Text: "two",
		Images: []protocol.ImageAttachment{{MediaType: "image/png", Data: []byte{1, 2, 3}}}})
	if err != nil || b.DeliverySeq != 2 {
		t.Fatalf("second append: %+v, %v", b, err)
	}
	all, err := ReadInbox(dir, 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("read all: %d, %v", len(all), err)
	}
	if !bytes.Equal(all[1].Images[0].Data, []byte{1, 2, 3}) {
		t.Fatal("attachment bytes did not round-trip")
	}
	tail, err := ReadInbox(dir, 1)
	if err != nil || len(tail) != 1 || tail[0].Text != "two" {
		t.Fatalf("tail after 1: %+v, %v", tail, err)
	}
	if none, _ := ReadInbox(dir, 2); len(none) != 0 {
		t.Fatal("high-water filter leaked consumed entries")
	}
}

// A fork inherits a consumed-input high-water mark but an empty inbox file;
// the seed advances lastInboxSeq so the next real delivery numbers ABOVE the
// mark, or the dedup would drop it as already-consumed and swallow every
// message the fork receives (C4).
func TestSeedInboxWatermark(t *testing.T) {
	dir := t.TempDir()
	if err := SeedInboxWatermark(dir, 3); err != nil {
		t.Fatal(err)
	}
	// Inert on replay: nothing at or below the mark is returned.
	if pending, _ := ReadInbox(dir, 3); len(pending) != 0 {
		t.Fatalf("seed must not replay, got %+v", pending)
	}
	// The next real delivery lands ABOVE the mark (4, not 1).
	in, err := AppendInbox(dir, protocol.UserInput{Text: "post-fork"})
	if err != nil || in.DeliverySeq != 4 {
		t.Fatalf("append after seed = %+v, %v; want seq 4", in, err)
	}
	// A zero/negative mark is a no-op — an un-forked session numbers from 1.
	dir2 := t.TempDir()
	if err := SeedInboxWatermark(dir2, 0); err != nil {
		t.Fatal(err)
	}
	if first, _ := AppendInbox(dir2, protocol.UserInput{Text: "x"}); first.DeliverySeq != 1 {
		t.Fatalf("zero seed must be a no-op, got seq %d", first.DeliverySeq)
	}
}
