package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunOutputHelperProcess(t *testing.T) {
	if os.Getenv("AGENTRUNNER_RUN_OUTPUT_HELPER") != "1" {
		return
	}
	fmt.Println(strings.Repeat("x", (4<<20)+1))
	time.Sleep(30 * time.Second) // parent must cancel us after Scanner rejects the line
}

func TestRunRegistryFailsAndCancelsOnOversizedOutputLine(t *testing.T) {
	t.Setenv("AGENTRUNNER_RUN_OUTPUT_HELPER", "1")
	rr := newRunRegistry()
	r := rr.start(os.Args[0], "drive", "oversized", t.TempDir(),
		[]string{"-test.run=TestRunOutputHelperProcess"}, t.TempDir(), nil, nil)
	deadline := time.Now().Add(5 * time.Second)
	for !r.finished() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !r.finished() {
		t.Fatal("run remained blocked after Scanner rejected oversized output")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Status != "failed" {
		t.Fatalf("status = %q, want failed", r.Status)
	}
	if got := strings.Join(r.lines, "\n"); !strings.Contains(got, "read run output") {
		t.Fatalf("run lines do not surface scan error: %s", got)
	}
}
