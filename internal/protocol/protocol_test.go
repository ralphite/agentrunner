package protocol

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONSinkLines(t *testing.T) {
	var buf bytes.Buffer
	s := NewJSONSink(&buf)
	s.Emit(Event{Kind: KindGenerationStart, N: 1})
	s.Emit(Event{Kind: KindTextDelta, N: 1, Text: "hi"})
	s.Emit(Event{Kind: KindRunEnd, Reason: "completed", N: 1})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	var e Event
	if err := json.Unmarshal([]byte(lines[1]), &e); err != nil {
		t.Fatal(err)
	}
	if e.Kind != KindTextDelta || e.Text != "hi" {
		t.Errorf("event = %+v", e)
	}
	// Sparse encoding: absent fields omitted.
	if strings.Contains(lines[1], "is_error") || strings.Contains(lines[1], "\"tool\"") {
		t.Errorf("non-sparse encoding: %s", lines[1])
	}
}

func TestDiscardSinkNoPanic(t *testing.T) {
	Discard.Emit(Event{Kind: KindError, Text: "boom"})
}
