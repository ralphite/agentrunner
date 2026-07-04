package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/event"
)

func TestTTYApprovalsParsing(t *testing.T) {
	req := agent.ApprovalRequest{
		ApprovalID: "apr-eff-call_1_0", CallID: "call_1_0",
		ToolName: "bash", Args: json.RawMessage(`{"command":"make deploy"}`),
		GateResults: []event.GateResult{
			{Gate: "permission", Decision: "ask", Reason: "execute requires approval"},
		},
	}
	cases := []struct {
		input      string
		approve    bool
		wantReason string
	}{
		{"y\n", true, ""},
		{"yes\n", true, ""},
		{"n\n", false, "denied at terminal"},
		{"n too risky for friday\n", false, "too risky for friday"},
		{"whatever\n", false, "denied at terminal"},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		a := &ttyApprovals{in: strings.NewReader(tc.input), out: &out}
		d, err := a.Resolve(context.Background(), req)
		if err != nil {
			t.Fatalf("%q: %v", tc.input, err)
		}
		if d.Approve != tc.approve || d.Reason != tc.wantReason || d.Source != "tty" {
			t.Errorf("%q: decision = %+v", tc.input, d)
		}
		for _, want := range []string{"approval required", "make deploy", "execute requires approval"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%q: prompt missing %q", tc.input, want)
			}
		}
	}
}

func TestTTYApprovalsCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := &ttyApprovals{in: blockingReader{}, out: &bytes.Buffer{}}
	if _, err := a.Resolve(ctx, agent.ApprovalRequest{}); err == nil {
		t.Fatal("canceled ctx must surface")
	}
}

type blockingReader struct{}

func (blockingReader) Read([]byte) (int, error) { select {} }
