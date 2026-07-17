package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ralphite/agentrunner/internal/driver"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider"
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
	report, err := buildInspectTree(dir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	// A "running" top-level session whose host process is gone is STRANDED
	// (T1/T2b — 状态撒谎): the daemon crashed or was restarted and nothing is
	// advancing it. Say so, and point at the recovery — inspect is the audit
	// command a stuck user reaches for. The probe reads the lock's pid; it
	// never takes the lock, so it cannot disturb a live writer.
	if report.Status == "running" && !store.HasLiveWriter(dir) {
		report.Status = "stranded"
		report.Reason = "no live host — recover: agentrunner resume " + filepath.Base(dir)
	}
	if report.Waiting != nil && report.Waiting.ApprovalID != "" {
		report.Waiting.AnswerWith = fmt.Sprintf("agentrunner approve %s %s approve|deny",
			filepath.Base(dir), report.Waiting.ApprovalID)
	}
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
// Children are the sub-agent runs (S5.9), recursively — the tree render.
type inspectReport struct {
	Kind                 string                       `json:"kind,omitempty"` // run | driver
	Spec                 string                       `json:"spec"`
	Model                string                       `json:"model"`
	Mode                 string                       `json:"mode"`
	Status               string                       `json:"status"`
	Reason               string                       `json:"reason,omitempty"`
	GenSteps             int                          `json:"gen_steps"`
	Entries              []entryReport                `json:"entries"`
	Usage                usageReport                  `json:"usage"`
	Artifacts            []artifactReport             `json:"artifacts,omitempty"`
	Children             []childReportRef             `json:"children,omitempty"`
	Goal                 *goalReport                  `json:"goal,omitempty"`
	Progress             []event.ProgressItem         `json:"progress,omitempty"`
	Stats                *statsReport                 `json:"stats,omitempty"`
	Turns                int                          `json:"turns,omitempty"`
	Items                int                          `json:"items,omitempty"`
	ProviderCapabilities *provider.CapabilityEnvelope `json:"provider_capabilities,omitempty"`
	Delegations          []state.Delegation           `json:"delegations,omitempty"`
	// Waiting names what an idle session is waiting FOR — for an approval
	// that includes the id and the answer command (QA Round2 F-E4: `approve
	// -h` says "inspect shows the id", and it used to show only "waiting").
	Waiting *waitingReport `json:"waiting,omitempty"`
}

// waitingReport surfaces the idle wait: kind (approval | input | background work…)
// plus, for approvals, the pending ask itself.
type waitingReport struct {
	Kind       string `json:"kind"`
	ApprovalID string `json:"approval_id,omitempty"`
	Tool       string `json:"tool,omitempty"`
	Args       string `json:"args,omitempty"`
	AnswerWith string `json:"answer_with,omitempty"`
	// Question and AskQuestions surface an ask_user park (INC-47.2) so a UI
	// can render the prompt or a structured form. Question is the plain
	// single-question text; AskQuestions is the structured form (empty for
	// a legacy single-question ask).
	Question     string              `json:"question,omitempty"`
	AskQuestions []event.AskQuestion `json:"ask_questions,omitempty"`
}

// goalReport surfaces an active in-session goal (INC-D1) so a driver/UI can
// show it and its progress.
type goalReport struct {
	GoalID    string `json:"goal_id"`
	Goal      string `json:"goal"`
	Checks    int    `json:"checks"`
	MaxChecks int    `json:"max_checks,omitempty"`
	Paused    bool   `json:"paused,omitempty"`
	// Verifiers counts command verifiers; 0 = self-certified goal (INC-10,
	// the model's audited goal_complete claim decides). Claimed = a claim is
	// pending adjudication at the next turn boundary.
	Verifiers int  `json:"verifiers"`
	Claimed   bool `json:"claimed,omitempty"`
}

// artifactReport is one published deliverable version (S5.9 column).
type artifactReport struct {
	Stream  string `json:"stream"`
	Version int    `json:"version"`
	Ref     string `json:"ref"`
	Source  string `json:"source,omitempty"`
}

// childReportRef ties a spawn call to the child run's own report.
type childReportRef struct {
	CallID  string        `json:"call_id"`
	Agent   string        `json:"agent"`
	Session string        `json:"session"`
	Reason  string        `json:"reason"`
	Report  inspectReport `json:"report"`
}

type entryReport struct {
	GenStep int    `json:"gen_step"`
	Kind    string `json:"kind"` // llm | compact | tool
	Name    string `json:"name"`
	CallID  string `json:"call_id,omitempty"`
	// Detail is the salient argument of a tool call (the file a file-tool
	// touched, the command bash ran) so the audit shows WHAT happened, not
	// just which tool (T7/R2-D-2).
	Detail       string `json:"detail,omitempty"`
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

// buildInspectTree builds a session's report and recurses into its child
// runs (S5.9): each SubagentCompleted names a child whose journal lives
// under <dir>/sub/; the tree render opens each child once, recursively.
func buildInspectTree(dir string) (inspectReport, error) {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return inspectReport{}, err
	}
	if isDriverJournal(events) {
		return buildDriverInspectTree(dir, events)
	}
	s, err := state.Fold(events)
	if err != nil {
		return inspectReport{}, fmt.Errorf("fold: %w", err)
	}
	report := buildInspectReport(events, s)
	for _, e := range events {
		switch e.Type {
		case event.TypeArtifactPublished:
			if dec, derr := event.DecodePayload(e); derr == nil {
				p := dec.(*event.ArtifactPublished)
				report.Artifacts = append(report.Artifacts, artifactReport{
					Stream: p.Stream, Version: p.Version, Ref: p.Ref, Source: p.Source,
				})
			}
		case event.TypeSubagentCompleted:
			dec, derr := event.DecodePayload(e)
			if derr != nil {
				continue
			}
			sub := dec.(*event.SubagentCompleted)
			childDir := childDirFor(dir, sub)
			if childDir == "" {
				continue
			}
			childReport, cerr := buildInspectTree(childDir)
			if cerr != nil {
				continue // a broken child journal must not sink the parent's view
			}
			settleChildReport(&childReport, sub.Reason)
			report.Children = append(report.Children, childReportRef{
				CallID: sub.CallID, Agent: sub.Agent, Session: sub.ChildSession,
				Reason: sub.Reason, Report: childReport,
			})
		}
	}
	report.Children = dedupeChildren(report.Children)
	return report, nil
}

// dedupeChildren keeps one entry per child, keyed by session (else call_id):
// a revived child journals one SubagentCompleted per settlement, so the same
// child shows up once per revival. First appearance keeps its position, the
// LATEST settlement wins the content — the freshest status is the true one
// (G26; the same contract as webui dedupeInspectNodes).
func dedupeChildren(refs []childReportRef) []childReportRef {
	order := make([]string, 0, len(refs))
	latest := make(map[string]childReportRef, len(refs))
	for i, ref := range refs {
		key := ref.Session
		if key == "" {
			key = ref.CallID
		}
		if key == "" {
			key = fmt.Sprintf("anonymous-%d", i)
		}
		if _, seen := latest[key]; !seen {
			order = append(order, key)
		}
		latest[key] = ref
	}
	out := make([]childReportRef, 0, len(order))
	for _, key := range order {
		out = append(out, latest[key])
	}
	return out
}

func isDriverJournal(events []event.Envelope) bool {
	return len(events) > 0 && event.DriverStream[events[0].Type]
}

// buildDriverInspectTree projects a driver's OWN journal through the driver
// fold. Driver and run streams deliberately have different state machines;
// sending driver_started through state.Fold made every real goal/loop series
// appear corrupt even though its journal was healthy.
func buildDriverInspectTree(dir string, events []event.Envelope) (inspectReport, error) {
	s, err := driver.Fold(events)
	if err != nil {
		return inspectReport{}, fmt.Errorf("driver fold: %w", err)
	}
	report := inspectReport{
		Kind: "driver", Status: string(s.Status), Reason: s.Reason,
		GenSteps: len(s.Iterations), Usage: driverUsageReport(s),
	}
	for _, env := range events {
		if env.Type == event.TypeDriverStarted {
			decoded, derr := event.DecodePayload(env)
			if derr != nil {
				return inspectReport{}, derr
			}
			report.Spec = decoded.(*event.DriverStarted).SpecName
			break
		}
	}
	for _, it := range s.Iterations {
		status := "scheduled"
		switch {
		case it.Skipped:
			status = "skipped"
		case it.Completed:
			status = it.ChildReason
		case it.Launched:
			status = "running"
		}
		detail := fmt.Sprintf("pass=%v score=%g", it.Verdict.Pass, it.Verdict.Score)
		report.Entries = append(report.Entries, entryReport{
			GenStep: it.N, Kind: "iteration", Name: status, Detail: detail,
		})
		childDir := filepath.Join(dir, "sub", fmt.Sprintf("iter-%d", it.N))
		if _, statErr := store.ReadEvents(childDir); statErr != nil {
			continue
		}
		childReport, childErr := buildInspectTree(childDir)
		if childErr != nil {
			continue
		}
		settleChildReport(&childReport, it.ChildReason)
		report.Children = append(report.Children, childReportRef{
			CallID: fmt.Sprintf("iteration-%d", it.N), Agent: "iteration",
			Session: it.ChildSession, Reason: it.ChildReason, Report: childReport,
		})
	}
	return report, nil
}

func driverUsageReport(s driver.State) usageReport {
	var total provider.Usage
	for _, it := range s.Iterations {
		if it.Completed {
			total = addProviderUsage(total, it.Usage)
			continue
		}
		// A retry attempt can have settled before its logical iteration does.
		// Show that real spend immediately without double-counting completed
		// iterations, whose Usage already contains every attempt.
		for _, attempt := range it.Attempts {
			if attempt.Completed {
				total = addProviderUsage(total, attempt.Usage)
			}
		}
	}
	return usageReport{
		InputTokens: total.InputTokens, OutputTokens: total.OutputTokens,
		CacheRead: total.CacheReadTokens, CacheWrite: total.CacheWriteTokens,
		Billed: total.Billed(),
	}
}

func addProviderUsage(a, b provider.Usage) provider.Usage {
	a.InputTokens += b.InputTokens
	a.OutputTokens += b.OutputTokens
	a.CacheReadTokens += b.CacheReadTokens
	a.CacheWriteTokens += b.CacheWriteTokens
	return a
}

// settleChildReport applies the parent's durable settlement to the nested
// projection. A canceled/error child can end with a mid-activity run fold, and
// a completed child can leave model-maintained progress rows unfinished; none
// of those may still be reported as running after settlement.
func settleChildReport(report *inspectReport, reason string) {
	switch reason {
	case "canceled", "killed", "stopped":
		report.Status = "canceled"
		report.Reason = reason
	case "error", "child_failed":
		report.Status = "failed"
		if report.Reason == "" {
			report.Reason = reason
		}
	}
	for i := range report.Progress {
		if report.Progress[i].Status == "running" || report.Progress[i].Status == "pending" {
			report.Progress[i].Status = "failed"
		}
	}
}

// childDirFor maps a SubagentCompleted to its journal dir: the child
// session suffix "-a<n>" names <dir>/sub/<call_id>-a<n>.
func childDirFor(dir string, sub *event.SubagentCompleted) string {
	i := strings.LastIndex(sub.ChildSession, "-a")
	if i < 0 {
		return ""
	}
	return filepath.Join(dir, "sub", sub.CallID+sub.ChildSession[i:])
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
	// Status/Reason are read off the SHAPE (决策 #31): explicit marks and
	// final provider failures are visible terminal shapes; otherwise
	// quiescence names the finish.
	status, reason := s.Session.Status, ""
	if s.Session.Failure != nil {
		status = "failed"
		reason = s.Session.Failure.Class
		if s.Session.Failure.Message != "" {
			reason = strings.TrimSpace(reason + ": " + s.Session.Failure.Message)
		}
	} else if s.Session.Closed != nil {
		status, reason = "marked", s.Session.Closed.Reason
		if s.Session.Closed.Reason == "stopped" {
			status, reason = "stopped", ""
		}
	} else if q, r := state.Quiescence(s); q {
		if strings.HasPrefix(r, "failed:") {
			status, reason = "failed", strings.TrimPrefix(r, "failed:")
		} else {
			status, reason = "quiescent", r
		}
	}
	report := inspectReport{
		Kind:     "run",
		Spec:     s.Session.SpecName,
		Model:    s.Session.Model,
		Mode:     s.CurrentMode(),
		Status:   status,
		Reason:   reason,
		GenSteps: s.Session.GenStep,
		Turns:    len(s.Interactions.Turns), Items: len(s.Interactions.Items),
		ProviderCapabilities: s.Session.ProviderCapabilities,
	}
	for _, delegation := range s.Team {
		report.Delegations = append(report.Delegations, delegation)
	}
	sort.Slice(report.Delegations, func(i, j int) bool {
		return report.Delegations[i].DelegationID < report.Delegations[j].DelegationID
	})
	if s.Goal != nil {
		report.Goal = &goalReport{
			GoalID: s.Goal.GoalID, Goal: s.Goal.Goal, Checks: s.Goal.Checks,
			MaxChecks: s.Goal.Budget.MaxChecks, Paused: s.Goal.Paused,
			Verifiers: len(s.Goal.Verifiers), Claimed: s.Goal.Claimed,
		}
	}
	report.Progress = append([]event.ProgressItem(nil), s.Session.Progress...)
	if s.Waiting != nil {
		wr := &waitingReport{Kind: s.Waiting.Kind}
		if s.Waiting.Kind == event.WaitApproval {
			var req event.ApprovalRequested
			if json.Unmarshal(s.Waiting.Detail, &req) == nil {
				wr.ApprovalID = req.ApprovalID
				wr.Tool = req.ToolName
				wr.Args = compactPayload(req.Args, 100)
			}
		}
		if s.Waiting.Kind == event.WaitInput && len(s.Waiting.Detail) > 0 {
			// An ask_user park (INC-47.2): surface the question(s) so a UI
			// renders the prompt or a structured form. A plain standby idle
			// has empty detail and falls through.
			var d struct {
				Question  string              `json:"question"`
				Questions []event.AskQuestion `json:"questions"`
			}
			if json.Unmarshal(s.Waiting.Detail, &d) == nil {
				wr.Question = d.Question
				wr.AskQuestions = d.Questions
			}
		}
		report.Waiting = wr
	}
	// ActivityCompleted carries no name/call id — those live on the matching
	// ActivityStarted; index them as we walk. A DENIED tool call never becomes
	// an activity, so its name comes from the assistant message that issued it.
	started := map[string]*event.ActivityStarted{}
	callName := map[string]string{}
	callArgs := map[string]json.RawMessage{}
	curTurn := 0
	for _, e := range events {
		switch e.Type {
		case event.TypeGenerationStarted:
			if dec, err := event.DecodePayload(e); err == nil {
				curTurn = dec.(*event.GenerationStarted).GenStep
			}
		case event.TypeAssistantMessage:
			if dec, err := event.DecodePayload(e); err == nil {
				for _, p := range dec.(*event.AssistantMessage).Message.Parts {
					if p.Kind == provider.PartToolCall {
						callName[p.CallID] = p.ToolName
						callArgs[p.CallID] = p.Args
					}
				}
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
			report.Entries = append(report.Entries, activityEntry(curTurn, a, started[a.ActivityID], byCall, byEffect, callArgs))
		case event.TypeEffectResolved:
			// A denied tool call produces no activity, so it would silently
			// vanish from the timeline — the audit reads "all allow" when every
			// action was blocked (T2/R2-D-3). Surface the denial explicitly.
			dec, err := event.DecodePayload(e)
			if err != nil {
				continue
			}
			r := dec.(*event.EffectResolved)
			if r.CallID != "" && r.Verdict == event.VerdictDeny {
				report.Entries = append(report.Entries, entryReport{
					GenStep: curTurn, Kind: "tool", Name: callName[r.CallID], CallID: r.CallID,
					Detail:  toolDetail(callArgs[r.CallID]),
					Verdict: event.VerdictDeny, Gate: decidingGate(r.GateResults, r.Verdict),
				})
			}
		}
	}

	report.Usage = usageReport{
		InputTokens:    s.Session.Usage.InputTokens,
		OutputTokens:   s.Session.Usage.OutputTokens,
		CacheRead:      s.Session.Usage.CacheReadTokens,
		CacheWrite:     s.Session.Usage.CacheWriteTokens,
		Billed:         s.Session.Usage.Billed(),
		BudgetReserved: s.Budget.ReservedTotal(),
	}
	report.Stats = buildStats(events)
	return report
}

// statsReport quantifies what a session DID (INC-43, HANDA #31): per-tool
// call counts and outcomes, line deltas from write/edit results, and
// active_seconds — wall-clock with at least one activity in flight
// (merged intervals), so idle standby and approval waits don't count.
// These are REPORTING projections over envelope timestamps, not core fold
// state (the fold stays wall-clock-free).
type statsReport struct {
	Tools         map[string]*toolStat `json:"tools,omitempty"`
	ToolCalls     int                  `json:"tool_calls"`
	ToolFailures  int                  `json:"tool_failures"`
	LinesAdded    int                  `json:"lines_added"`
	LinesRemoved  int                  `json:"lines_removed"`
	ActiveSeconds float64              `json:"active_seconds"`
}

type toolStat struct {
	Calls      int   `json:"calls"`
	Success    int   `json:"success"`
	Fail       int   `json:"fail"`
	DurationMS int64 `json:"duration_ms"`
}

func buildStats(events []event.Envelope) *statsReport {
	st := &statsReport{Tools: map[string]*toolStat{}}
	callNames := map[string]string{}
	type open struct {
		started *event.ActivityStarted
		ts      time.Time
	}
	inflight := map[string]open{}
	type span struct{ start, end time.Time }
	var spans []span
	closeActivity := func(id string, end time.Time, fail, cancelled bool, result json.RawMessage) {
		o, ok := inflight[id]
		if !ok {
			return
		}
		delete(inflight, id)
		if !o.ts.IsZero() && !end.IsZero() && end.After(o.ts) {
			spans = append(spans, span{o.ts, end})
		}
		if o.started == nil || o.started.Kind != event.KindTool {
			return
		}
		name := o.started.Name
		ts := st.Tools[name]
		if ts == nil {
			ts = &toolStat{}
			st.Tools[name] = ts
		}
		ts.Calls++
		st.ToolCalls++
		if fail || cancelled {
			ts.Fail++
			st.ToolFailures++
		} else {
			ts.Success++
		}
		if !o.ts.IsZero() && !end.IsZero() && end.After(o.ts) {
			ts.DurationMS += end.Sub(o.ts).Milliseconds()
		}
		if !fail && !cancelled && (name == "write_file" || name == "edit_file") && len(result) > 0 {
			var d struct {
				Added   int `json:"lines_added"`
				Removed int `json:"lines_removed"`
			}
			if json.Unmarshal(result, &d) == nil {
				st.LinesAdded += d.Added
				st.LinesRemoved += d.Removed
			}
		}
	}
	for _, env := range events {
		dec, err := event.DecodePayload(env)
		if err != nil {
			continue
		}
		switch p := dec.(type) {
		case *event.AssistantMessage:
			for _, part := range p.Message.Parts {
				if part.Kind == provider.PartToolCall && part.CallID != "" {
					callNames[part.CallID] = part.ToolName
				}
			}
		case *event.EffectResolved:
			if p.CallID == "" || p.Verdict != event.VerdictDeny {
				break
			}
			name := callNames[p.CallID]
			if name == "" {
				name = "unknown"
			}
			ts := st.Tools[name]
			if ts == nil {
				ts = &toolStat{}
				st.Tools[name] = ts
			}
			ts.Calls++
			ts.Fail++
			st.ToolCalls++
			st.ToolFailures++
		case *event.ActivityStarted:
			inflight[p.ActivityID] = open{started: p, ts: env.TS}
		case *event.ActivityCompleted:
			closeActivity(p.ActivityID, env.TS, p.IsError, false, p.Result)
		case *event.ActivityFailed:
			if p.Final {
				closeActivity(p.ActivityID, env.TS, true, false, nil)
			}
		case *event.ActivityCancelled:
			closeActivity(p.ActivityID, env.TS, false, true, nil)
		}
	}
	// Merge overlapping activity spans: parallel tool batches count once.
	sort.Slice(spans, func(i, j int) bool { return spans[i].start.Before(spans[j].start) })
	var active time.Duration
	var curStart, curEnd time.Time
	for _, sp := range spans {
		if curEnd.IsZero() || sp.start.After(curEnd) {
			active += curEnd.Sub(curStart)
			curStart, curEnd = sp.start, sp.end
			continue
		}
		if sp.end.After(curEnd) {
			curEnd = sp.end
		}
	}
	active += curEnd.Sub(curStart)
	st.ActiveSeconds = float64(active.Milliseconds()) / 1000
	if st.ToolCalls == 0 && st.ActiveSeconds == 0 {
		return nil
	}
	return st
}

func activityEntry(turn int, a *event.ActivityCompleted, started *event.ActivityStarted, byCall, byEffect map[string]verdictInfo, callArgs map[string]json.RawMessage) entryReport {
	var callID, name string
	if started != nil {
		callID, name = started.CallID, started.Name
	}
	e := entryReport{GenStep: turn, CallID: callID}
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
		e.Detail = toolDetail(callArgs[callID])
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

// toolDetail pulls the salient argument out of a tool call for the audit
// timeline — the file a file-tool touched, the command bash ran — so the
// reader sees WHAT happened, not just which tool (T7/R2-D-2).
func toolDetail(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(args, &m) != nil {
		return ""
	}
	for _, k := range []string{"path", "command", "file", "query"} {
		if v, ok := m[k].(string); ok && v != "" {
			return oneLine(v, 60)
		}
	}
	return ""
}

// oneLine collapses a value to a single, length-capped line for the timeline.
func oneLine(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimRight(s[:i], " ") + " …"
	}
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
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
	renderInspectIndent(w, r, "")
}

func renderInspectIndent(w io.Writer, r inspectReport, pad string) {
	if r.Kind == "driver" {
		fmt.Fprintf(w, "%sdriver  %s\n", pad, r.Spec)
	} else {
		fmt.Fprintf(w, "%sspec    %s    model %s    mode %s\n", pad, r.Spec, r.Model, r.Mode)
	}
	status := r.Status
	if r.Reason != "" {
		status += " (" + r.Reason + ")"
	}
	countLabel := "gen-steps"
	if r.Kind == "driver" {
		countLabel = "iterations"
	}
	fmt.Fprintf(w, "%sstatus  %s    %s %d\n", pad, status, countLabel, r.GenSteps)
	if r.Goal != nil {
		// The goal is first-class session state — surface it here, not only
		// in --json (QA Round4 F-I3/F-J2: users had to grep `events`).
		g := r.Goal
		line := fmt.Sprintf("%sgoal    %q · %d", pad, g.Goal, g.Checks)
		if g.MaxChecks > 0 {
			line += fmt.Sprintf("/%d", g.MaxChecks)
		}
		line += " checks"
		if g.Verifiers == 0 {
			line += " · self-certified"
		}
		if g.Claimed {
			line += " · claim pending"
		}
		if g.Paused {
			line += " · paused"
		}
		fmt.Fprintln(w, line)
	}
	if len(r.Progress) > 0 {
		// The model-maintained checklist (INC-37): one line per step, with a
		// done-count summary — the human-readable twin of --json's progress.
		done := 0
		for _, it := range r.Progress {
			if it.Status == "done" {
				done++
			}
		}
		fmt.Fprintf(w, "%sprogress %d/%d done\n", pad, done, len(r.Progress))
		mark := map[string]string{"pending": "·", "running": "▸", "done": "✓", "failed": "✗"}
		for _, it := range r.Progress {
			fmt.Fprintf(w, "%s        %s %s — %s\n", pad, mark[it.Status], it.Title, it.Status)
		}
	}
	if st := r.Stats; st != nil {
		// One activity line (INC-43): what the session DID. Full per-tool
		// numbers live in --json.
		line := fmt.Sprintf("%sstats   %d tool calls", pad, st.ToolCalls)
		if st.ToolFailures > 0 {
			line += fmt.Sprintf(" (%d failed)", st.ToolFailures)
		}
		if st.LinesAdded > 0 || st.LinesRemoved > 0 {
			line += fmt.Sprintf(" · +%d/−%d lines", st.LinesAdded, st.LinesRemoved)
		}
		line += fmt.Sprintf(" · active %.1fs", st.ActiveSeconds)
		fmt.Fprintln(w, line)
	}
	if r.Waiting != nil {
		if r.Waiting.ApprovalID != "" {
			fmt.Fprintf(w, "%swaiting approval %s: %s %s\n", pad, r.Waiting.ApprovalID, r.Waiting.Tool, r.Waiting.Args)
			if r.Waiting.AnswerWith != "" {
				fmt.Fprintf(w, "%s        answer with: %s\n", pad, r.Waiting.AnswerWith)
			}
		} else {
			fmt.Fprintf(w, "%swaiting %s\n", pad, r.Waiting.Kind)
		}
	}
	fmt.Fprintln(w)
	if r.Kind != "driver" && (r.Turns > 0 || r.Items > 0) {
		fmt.Fprintf(w, "%sitems   turns %d    items %d\n\n", pad, r.Turns, r.Items)
	}

	fmt.Fprintln(w, pad+"TIMELINE")
	lastTurn := -1
	for _, e := range r.Entries {
		if e.GenStep != lastTurn {
			label := "gen-step"
			if r.Kind == "driver" {
				label = "iteration"
			}
			fmt.Fprintf(w, "%s  %s %d\n", pad, label, e.GenStep)
			lastTurn = e.GenStep
		}
		verdict := e.Verdict
		if e.Gate != "" {
			verdict = fmt.Sprintf("%s [%s]", e.Verdict, e.Gate)
		}
		toks := ""
		if e.InputTokens > 0 || e.OutputTokens > 0 {
			toks = fmt.Sprintf("in %d out %d cache_r %d", e.InputTokens, e.OutputTokens, e.CacheRead)
		}
		tail := toks
		if e.Detail != "" {
			if tail != "" {
				tail += "  "
			}
			tail += e.Detail
		}
		fmt.Fprintf(w, "%s    %-8s %-12s %-10s %-24s %s\n", pad, e.Kind, e.Name, e.CallID, verdict, strings.TrimRight(tail, " "))
	}
	if len(r.Delegations) > 0 {
		fmt.Fprintln(w, "\n"+pad+"DELEGATIONS")
		for _, delegation := range r.Delegations {
			workspaceMode, workspacePath := "", ""
			if delegation.Workspace != nil {
				workspaceMode, workspacePath = delegation.Workspace.Mode, delegation.Workspace.Path
			}
			fmt.Fprintf(w, "%s  %s  %-10s member %s  workspace %s %s\n",
				pad, delegation.DelegationID, delegation.Status, delegation.AssignedTo, workspaceMode, workspacePath)
		}
	}

	if len(r.Artifacts) > 0 {
		fmt.Fprintln(w, "\n"+pad+"ARTIFACTS")
		for _, a := range r.Artifacts {
			fmt.Fprintf(w, "%s  %s@v%d  %s  (%s)\n", pad, a.Stream, a.Version, a.Ref, a.Source)
		}
	}

	u := r.Usage
	// "(tokens)" so the numbers aren't misread as a dollar amount — "billed"
	// is billed TOKENS, not money; the harness has no provider price table
	// (blackbox R2-D-1). Cost = billed × your provider's per-token price.
	fmt.Fprintf(w, "\n%sUSAGE (tokens)   input %d  output %d  cache_read %d  cache_write %d  billed %d\n",
		pad, u.InputTokens, u.OutputTokens, u.CacheRead, u.CacheWrite, u.Billed)
	if u.BudgetReserved > 0 {
		fmt.Fprintf(w, "%s        reserved %d\n", pad, u.BudgetReserved)
	}

	// The agent tree (S5.9): each child run renders recursively, indented.
	for _, c := range r.Children {
		fmt.Fprintf(w, "\n%sCHILD   %s → %s  [%s]  (%s)\n", pad, c.CallID, c.Agent, c.Reason, c.Session)
		renderInspectIndent(w, c.Report, pad+"    ")
	}
}
