package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/ralphite/agentrunner/internal/protocol"
)

// textRenderer turns the output protocol into human-readable terminal
// output. Text deltas stream inline; everything else is a labeled line.
type textRenderer struct {
	out     io.Writer
	inDelta bool // currently mid text-delta line?
}

func newTextRenderer(out io.Writer) *textRenderer { return &textRenderer{out: out} }

func (r *textRenderer) Emit(e protocol.Event) {
	// A non-delta event closes any open delta line.
	if e.Kind != protocol.KindTextDelta && r.inDelta {
		fmt.Fprintln(r.out)
		r.inDelta = false
	}
	switch e.Kind {
	case protocol.KindTurnStart:
		fmt.Fprintf(r.out, "\n[turn %d]\n", e.Turn)
	case protocol.KindTextDelta:
		fmt.Fprint(r.out, e.Text)
		r.inDelta = true
	case protocol.KindMessage:
		// The assembled message already streamed as deltas for live
		// providers; scripted/non-streaming providers emit only this.
		if !strings.HasSuffix(e.Text, "\n") {
			fmt.Fprintln(r.out, e.Text)
		} else {
			fmt.Fprint(r.out, e.Text)
		}
	case protocol.KindToolCall:
		fmt.Fprintf(r.out, "  → %s %s\n", e.Tool, truncate(e.Args, 120))
	case protocol.KindToolResult:
		status := "ok"
		if e.IsError {
			status = "error"
		}
		fmt.Fprintf(r.out, "  ← %s %s\n", status, truncate(e.Result, 200))
	case protocol.KindApprovalRequest:
		fmt.Fprintf(r.out, "  ⏸ approval required: %s\n", e.Tool)
	case protocol.KindModeChanged:
		fmt.Fprintf(r.out, "  » mode → %s\n", e.Mode)
	case protocol.KindDiscard:
		fmt.Fprintf(r.out, "  ↺ %s\n", e.Text)
	case protocol.KindError:
		fmt.Fprintf(r.out, "  ✗ %s\n", e.Text)
	case protocol.KindRunEnd:
		// Summary line is printed by runAgent from the RunResult.
	}
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
