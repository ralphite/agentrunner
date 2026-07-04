// Package blackboard is the pub/sub collaboration surface for an agent tree
// (S5.4): a topic → ordered-notes store shared by a parent run and its
// sub-agents. The store itself is EPHEMERAL runtime state (lifetime = the
// root run's process) — durability follows the event-log doctrine instead:
// anything that influences a run's result crosses into that run's journal,
// and here that is the READ — a read_notes result is journaled by the
// reading run like any tool result. A note nobody reads never influenced
// anything, so losing the store on restart loses nothing that mattered.
//
// The kernel bus is deliberately NOT in this path yet: no bus instance is
// alive in the CLI run path today, and reads need synchronous access. When
// the S6 daemon hosts actor surfaces (notifier, frontends), Publish grows a
// bus mirror onto an ephemeral topic; the store stays the read-back truth.
package blackboard

import (
	"sort"
	"sync"
)

// Note is one published entry. Seq is a per-board global order — readers
// see cross-topic causality consistently.
type Note struct {
	Seq   int    `json:"seq"`
	Topic string `json:"topic"`
	From  string `json:"from"`
	Text  string `json:"text"`
}

// Board is a concurrency-safe topic store. The zero value is NOT usable;
// call New.
type Board struct {
	mu     sync.Mutex
	nextSq int
	topics map[string][]Note
}

func New() *Board {
	return &Board{topics: map[string][]Note{}}
}

// Publish appends a note to a topic, returning its sequence number.
func (b *Board) Publish(topic, from, text string) Note {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextSq++
	n := Note{Seq: b.nextSq, Topic: topic, From: from, Text: text}
	b.topics[topic] = append(b.topics[topic], n)
	return n
}

// Read returns a topic's notes in publish order (a copy — callers may not
// mutate the store).
func (b *Board) Read(topic string) []Note {
	b.mu.Lock()
	defer b.mu.Unlock()
	notes := b.topics[topic]
	out := make([]Note, len(notes))
	copy(out, notes)
	return out
}

// Topics lists topics with at least one note, sorted.
func (b *Board) Topics() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, len(b.topics))
	for t := range b.topics {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
