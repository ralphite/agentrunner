package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"golang.org/x/term"

	"github.com/ralphite/agentrunner/internal/agent"
)

// ttyApprovals is the interactive resolver (3.10): shows the effect and
// every gate's judgment, then reads y/n (an optional reason follows after
// a space). Non-TTY runs use EnvApprovals instead — never a hang.
type ttyApprovals struct {
	in  io.Reader
	out io.Writer
	// mu serializes concurrent asks: sibling sub-agents share this resolver
	// (S5 review) — without it two approval prompts interleave and two stdin readers
	// race for one typed answer, approving the wrong request.
	mu sync.Mutex
}

func (a *ttyApprovals) Resolve(ctx context.Context, req agent.ApprovalRequest) (agent.ApprovalDecision, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	fmt.Fprintf(a.out, "\n─── approval required ───\n")
	if req.Agent != "" {
		fmt.Fprintf(a.out, "  agent: %s\n", req.Agent)
	}
	if req.ToolName != "" {
		fmt.Fprintf(a.out, "  tool: %s\n  args: %s\n", req.ToolName, truncate(string(req.Args), 200))
	}
	for _, g := range req.GateResults {
		fmt.Fprintf(a.out, "  %s: %s", g.Gate, g.Decision)
		if g.Reason != "" {
			fmt.Fprintf(a.out, " (%s)", g.Reason)
		}
		fmt.Fprintln(a.out)
	}
	fmt.Fprintf(a.out, "approve? [y/n] (optionally: n <reason>): ")

	// Terminal reads cannot be canceled directly; read in a goroutine and
	// race ctx (an abandoned read dies with the process).
	type line struct {
		s   string
		err error
	}
	ch := make(chan line, 1)
	go func() {
		s, err := bufio.NewReader(a.in).ReadString('\n')
		ch <- line{s, err}
	}()
	select {
	case <-ctx.Done():
		return agent.ApprovalDecision{}, ctx.Err()
	case l := <-ch:
		if l.err != nil {
			return agent.ApprovalDecision{}, fmt.Errorf("reading approval: %w", l.err)
		}
		answer := strings.TrimSpace(l.s)
		verb, reason, _ := strings.Cut(answer, " ")
		switch strings.ToLower(verb) {
		case "y", "yes":
			return agent.ApprovalDecision{Approve: true, Reason: strings.TrimSpace(reason), Source: "tty"}, nil
		default:
			r := strings.TrimSpace(reason)
			if r == "" {
				r = "denied at terminal"
			}
			return agent.ApprovalDecision{Approve: false, Reason: r, Source: "tty"}, nil
		}
	}
}

// approvalResolver picks the interactive resolver when stdin is a real
// terminal (NOT merely a char device — /dev/null is one too); otherwise
// nil, and the loop falls back to fail-closed EnvApprovals.
func approvalResolver(stdout io.Writer) agent.ApprovalResolver {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}
	return &ttyApprovals{in: os.Stdin, out: stdout}
}
