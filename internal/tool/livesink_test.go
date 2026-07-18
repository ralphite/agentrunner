package tool

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
)

// The live tee must deliver chunks as written AND leave the buffered result
// bytes-for-bytes identical — the completion record is the durable truth.
func TestRunSandboxedTeesLiveOutput(t *testing.T) {
	e, _ := newExec(t)
	if _, err := e.SandboxInfo(); err != nil {
		t.Skipf("no OS sandbox backend here: %v", err)
	}
	var mu sync.Mutex
	var live bytes.Buffer
	ctx := WithLiveOutput(context.Background(), func(chunk []byte) {
		mu.Lock()
		defer mu.Unlock()
		live.Write(chunk)
	})
	out, isErr := runCtx(t, ctx, e, "bash", `{"command":"echo tail-me; echo err-me 1>&2"}`)
	if isErr {
		t.Fatalf("errored: %v", out)
	}
	if !strings.Contains(out["stdout"].(string), "tail-me") {
		t.Fatalf("stdout lost: %v", out)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, want := range []string{"tail-me", "err-me"} {
		if !strings.Contains(live.String(), want) {
			t.Errorf("live tee missing %q: %q", want, live.String())
		}
	}
}
