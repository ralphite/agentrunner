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
