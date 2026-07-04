package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// inspectCmd implements `agentrunner inspect <session> [--json]` (S4.8): a
// per-turn timeline with each call's adjudication verdict and token/cache
// columns, read from the event log and its fold. Where `events` dumps the
// raw log, `inspect` renders the run the way a human reasons about it.
func inspectCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "emit the report as JSON")
	var flagArgs, positional []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
		} else {
			positional = append(positional, a)
		}
	}
	if err := fs.Parse(append(flagArgs, positional...)); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: agentrunner inspect <session-id-or-prefix> [--json]")
		return ExitUsage
	}

	dir, err := resolveSessionDir(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	events, err := store.ReadEvents(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	s, err := state.Fold(events)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: fold: %v\n", err)
		return ExitRun
	}

	report := buildInspectReport(events, s)
	if *asJSON {
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "agentrunner: %v\n", err)
			return ExitRun
		}
		fmt.Fprintln(stdout, string(raw))
		return ExitOK
	}
	renderInspect(stdout, report)
	return ExitOK
}

// inspectReport is the structured `inspect` output (also the --json shape).
type inspectReport struct {
	Spec    string        `json:"spec"`
	Model   string        `json:"model"`
	Mode    string        `json:"mode"`
	Status  string        `json:"status"`
	Reason  string        `json:"reason,omitempty"`
	Turns   int           `json:"turns"`
	Entries []entryReport `json:"entries"`
	Usage   usageReport   `json:"usage"`
}

type entryReport struct {
	Turn         int    `json:"turn"`
	Kind         string `json:"kind"` // llm | compact | tool
	Name         string `json:"name"`
	CallID       string `json:"call_id,omitempty"`
	Verdict      string `json:"verdict,omitempty"`
	Gate         string `json:"gate,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	CacheRead    int    `json:"cache_read,omitempty"`
}

type usageReport struct {
	InputTokens    int `json:"input_tokens"`
	OutputTokens   int `json:"output_tokens"`
	CacheRead      int `json:"cache_read"`
	CacheWrite     int `json:"cache_write"`
	Billed         int `json:"billed"`
	BudgetLimit    int `json:"budget_limit,omitempty"`
	BudgetReserved int `json:"budget_reserved,omitempty"`
}

// verdictInfo is one effect's adjudication outcome, keyed for lookup.
type verdictInfo struct {
	verdict string
	gate    string
}

func buildInspectReport(events []event.Envelope, s state.State) inspectReport {
	// Pass 1: index each effect's resolution by call id (tools) and effect id
	// (llm effects have no call id).
	byCall := map[string]verdictInfo{}
	byEffect := map[string]verdictInfo{}
	for _, e := range events {
		if e.Type != event.TypeEffectResolved {
			continue
		}
		dec, err := event.DecodePayload(e)
		if err != nil {
			continue
		}
		r := dec.(*event.EffectResolved)
		info := verdictInfo{verdict: r.Verdict, gate: decidingGate(r.GateResults, r.Verdict)}
		if r.CallID != "" {
			byCall[r.CallID] = info
		}
		byEffect[r.EffectID] = info
	}

	// Pass 2: walk activity completions into per-turn timeline entries.
	report := inspectReport{
		Spec:   s.Run.SpecName,
		Model:  s.Run.Model,
		Mode:   s.CurrentMode(),
		Status: s.Run.Status,
		Reason: s.Run.Reason,
		Turns:  s.Run.Turn,
	}
	// ActivityCompleted carries no name/call id — those live on the matching
	// ActivityStarted; index them as we walk.
	started := map[string]*event.ActivityStarted{}
	curTurn := 0
	for _, e := range events {
		switch e.Type {
		case event.TypeTurnStarted:
			if dec, err := event.DecodePayload(e); err == nil {
				curTurn = dec.(*event.TurnStarted).Turn
			}
		case event.TypeActivityStarted:
			if dec, err := event.DecodePayload(e); err == nil {
				a := dec.(*event.ActivityStarted)
				started[a.ActivityID] = a
			}
		case event.TypeActivityCompleted:
			dec, err := event.DecodePayload(e)
			if err != nil {
				continue
			}
			a := dec.(*event.ActivityCompleted)
			report.Entries = append(report.Entries, activityEntry(curTurn, a, started[a.ActivityID], byCall, byEffect))
		}
	}

	report.Usage = usageReport{
		InputTokens:    s.Run.Usage.InputTokens,
		OutputTokens:   s.Run.Usage.OutputTokens,
		CacheRead:      s.Run.Usage.CacheReadTokens,
		CacheWrite:     s.Run.Usage.CacheWriteTokens,
		Billed:         s.Run.Usage.Billed(),
		BudgetReserved: s.Budget.ReservedTotal(),
	}
	return report
}

func activityEntry(turn int, a *event.ActivityCompleted, started *event.ActivityStarted, byCall, byEffect map[string]verdictInfo) entryReport {
	var callID, name string
	if started != nil {
		callID, name = started.CallID, started.Name
	}
	e := entryReport{Turn: turn, CallID: callID}
	switch {
	case strings.HasPrefix(a.ActivityID, "llm-t"):
		e.Kind, e.Name = "llm", "complete"
		if v, ok := byEffect[llmEffectID(a.ActivityID)]; ok {
			e.Verdict, e.Gate = v.verdict, v.gate
		}
	case strings.HasPrefix(a.ActivityID, "compact-t"):
		e.Kind, e.Name = "compact", "summarize"
	default:
		e.Kind, e.Name = "tool", name
		if v, ok := byCall[callID]; ok {
			e.Verdict, e.Gate = v.verdict, v.gate
		}
	}
	if a.Usage != nil {
		e.InputTokens = a.Usage.InputTokens
		e.OutputTokens = a.Usage.OutputTokens
		e.CacheRead = a.Usage.CacheReadTokens
	}
	return e
}

// llmEffectID maps an llm activity id (llm-t<n>) to its effect id
// (eff-llm-t<n>), the namespacing the loop uses.
func llmEffectID(activityID string) string {
	return "eff-" + activityID
}

// decidingGate names the gate that produced the verdict (the denier on a
// deny, otherwise the last gate that ruled), with its reason.
func decidingGate(results []event.GateResult, verdict string) string {
	var chosen *event.GateResult
	for i := range results {
		r := results[i]
		if verdict == event.VerdictDeny && r.Decision == event.VerdictDeny {
			chosen = &results[i]
			break
		}
		chosen = &results[i]
	}
	if chosen == nil {
		return ""
	}
	if chosen.Reason != "" {
		return fmt.Sprintf("%s: %s", chosen.Gate, chosen.Reason)
	}
	return chosen.Gate
}

func renderInspect(w io.Writer, r inspectReport) {
	fmt.Fprintf(w, "spec    %s    model %s    mode %s\n", r.Spec, r.Model, r.Mode)
	status := r.Status
	if r.Reason != "" {
		status += " (" + r.Reason + ")"
	}
	fmt.Fprintf(w, "status  %s    turns %d\n\n", status, r.Turns)

	fmt.Fprintln(w, "TIMELINE")
	lastTurn := -1
	for _, e := range r.Entries {
		if e.Turn != lastTurn {
			fmt.Fprintf(w, "  turn %d\n", e.Turn)
			lastTurn = e.Turn
		}
		verdict := e.Verdict
		if e.Gate != "" {
			verdict = fmt.Sprintf("%s [%s]", e.Verdict, e.Gate)
		}
		toks := ""
		if e.InputTokens > 0 || e.OutputTokens > 0 {
			toks = fmt.Sprintf("in %d out %d cache_r %d", e.InputTokens, e.OutputTokens, e.CacheRead)
		}
		id := e.CallID
		fmt.Fprintf(w, "    %-8s %-12s %-10s %-24s %s\n", e.Kind, e.Name, id, verdict, toks)
	}

	u := r.Usage
	fmt.Fprintf(w, "\nUSAGE   input %d  output %d  cache_read %d  cache_write %d  billed %d\n",
		u.InputTokens, u.OutputTokens, u.CacheRead, u.CacheWrite, u.Billed)
	if u.BudgetReserved > 0 {
		fmt.Fprintf(w, "        reserved %d\n", u.BudgetReserved)
	}
}
