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
	out      io.Writer
	inDelta  bool // currently mid text-delta line?
	sawDelta bool // this turn's text already streamed as deltas?
	// session anchors the transcript (INC-12.6): tree members share the
	// root's sink with their own tags, so a foreground render folds member
	// events into one status line each instead of interleaving deltas.
	// Empty = render everything (legacy single-stream wiring).
	session string
	seen    map[string]bool // members already announced
}

func newTextRenderer(out io.Writer) *textRenderer { return &textRenderer{out: out} }

// anchor pins the renderer to one session: other tree members' live events
// fold to a short announcement (their reports re-enter the anchored
// conversation as messages anyway); member approvals still surface — they
// need the user.
func (r *textRenderer) anchor(sid string) *textRenderer {
	r.session = sid
	return r
}

func (r *textRenderer) Emit(e protocol.Event) {
	// Auto-anchor on the first SessionStart (the root's own always precedes
	// any member's events): from then on, member streams fold.
	if r.session == "" && e.Kind == protocol.KindSessionStart && e.Session != "" {
		r.session = e.Session
	}
	if r.session != "" && e.Session != "" && e.Session != r.session {
		if e.Kind == protocol.KindApprovalRequest {
			if r.inDelta {
				fmt.Fprintln(r.out)
				r.inDelta = false
			}
			fmt.Fprintf(r.out, "  ⏸ [member %s] approval required: %s %s (answer with: agentrunner approve %s %s approve|deny)\n",
				e.Session, e.Tool, truncate(e.Args, 80), e.Session, e.ApprovalID)
			return
		}
		if r.seen == nil {
			r.seen = map[string]bool{}
		}
		if !r.seen[e.Session] {
			r.seen[e.Session] = true
			if r.inDelta {
				fmt.Fprintln(r.out)
				r.inDelta = false
			}
			fmt.Fprintf(r.out, "  ⇣ [member %s live — attach for detail]\n", e.Session)
		}
		return
	}
	// A non-delta event closes any open delta line.
	if e.Kind != protocol.KindTextDelta && r.inDelta {
		fmt.Fprintln(r.out)
		r.inDelta = false
	}
	switch e.Kind {
	case protocol.KindGenerationStart:
		fmt.Fprintf(r.out, "\n[gen-step %d]\n", e.N)
		r.sawDelta = false
	case protocol.KindTextDelta:
		fmt.Fprint(r.out, e.Text)
		r.inDelta = true
		r.sawDelta = true
	case protocol.KindUserInput:
		// The user's own half of the conversation. Emitted only by the replay
		// projection (attach 补读) so a rejoining watcher sees what was asked,
		// not just the assistant's answers (QA Wave1 cli-life-02). A source tag
		// is shown for non-user (machine/hook) inputs.
		label := "you"
		if e.Tool != "" { // reuse Tool as the source tag for non-user inputs
			label = e.Tool
		}
		for _, line := range strings.Split(strings.TrimRight(e.Text, "\n"), "\n") {
			fmt.Fprintf(r.out, "▸ %s: %s\n", label, line)
			label = strings.Repeat(" ", len(label)) // align continuation lines
		}
	case protocol.KindMessage:
		// Deltas take precedence: any provider that streamed this turn's
		// text already put it on screen — printing the assembled message
		// again would double it (S6 还债②). The message prints only as the
		// fallback for a turn that produced no deltas.
		if r.sawDelta {
			r.sawDelta = false
			break
		}
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
		reason := ""
		if e.Text != "" {
			reason = e.Text + " — "
		}
		fmt.Fprintf(r.out, "  ⏸ approval required: %s %s (%sanswer with: agentrunner approve %s %s approve|deny)\n",
			e.Tool, truncate(e.Args, 80), reason, e.Session, e.ApprovalID)
	case protocol.KindModeChanged:
		fmt.Fprintf(r.out, "  » mode → %s\n", e.Mode)
	case protocol.KindNote:
		fmt.Fprintf(r.out, "  ✎ %s\n", e.Text)
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
