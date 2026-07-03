//go:build live

package gemini

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/provider"
)

// loadDotEnv populates missing env vars from the repo-root .env (local
// convenience per PLAN §0; never overrides existing values).
func loadDotEnv(t *testing.T) {
	f, err := os.Open("../../../.env")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && os.Getenv(k) == "" {
			t.Setenv(k, v)
		}
	}
}

func TestLiveSmoke(t *testing.T) {
	loadDotEnv(t)
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set; live smoke deferred to stage-exit checkpoint")
	}

	ctx := context.Background()
	p, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}

	turn, err := provider.CollectTurn(p.Complete(ctx, provider.CompleteRequest{
		Model:     "gemini-flash-latest",
		MaxTokens: 200,
		System:    "You are terse.",
		Messages: []provider.Message{{
			Role:  provider.RoleUser,
			Parts: []provider.Part{{Kind: provider.PartText, Text: "Say the single word: pong"}},
		}},
		Turn: 1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.Message.Parts) == 0 || !strings.Contains(strings.ToLower(turn.Message.Parts[0].Text), "pong") {
		t.Errorf("unexpected reply: %+v", turn.Message)
	}
	if turn.Usage.OutputTokens == 0 {
		t.Errorf("usage not populated: %+v", turn.Usage)
	}
}
