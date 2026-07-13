package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ralphite/agentrunner/internal/errs"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/snapshot"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// SubSpecResolver resolves a sub-agent name from the spec's agents whitelist
// to its AgentSpec (S5.3). The CLI resolves <name>.yaml next to the parent
// spec; tests inject directly.
type SubSpecResolver func(name string) (*AgentSpec, error)

// InlineRole is the untrusted, model-authored role accepted by
// spawn_agent when agents_dynamic is enabled. It deliberately has no
// hooks/MCP/skills/model/budget/sandbox fields: those capabilities are
// inherited under harness control, never drafted by the model.
type InlineRole struct {
	Name         string                    `json:"name"`
	Description  string                    `json:"description"`
	Instructions string                    `json:"instructions"`
	Tools        []string                  `json:"tools,omitempty"`
	Permissions  []pipeline.PermissionRule `json:"permissions,omitempty"`
	Escalate     bool                      `json:"escalate,omitempty"`
}

type spawnPlan struct {
	DelegationID string
	DependsOn    []string
	// Replaces retires a predecessor handle before the successor starts
	// (INC-30, G25): the same cancel the kill tool fires, so an abandoned
	// member stops spending the shared budget. Unknown/settled handles are
	// a no-op — replacement is idempotent.
	Replaces string
	Problem  string
}

func planSpawn(team map[string]state.Delegation, call provider.ToolCall) spawnPlan {
	plan := spawnPlan{DelegationID: "delegation-" + call.CallID}
	var args struct {
		DependsOn []string `json:"depends_on"`
		Replaces  string   `json:"replaces"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return plan
	}
	plan.Replaces = strings.TrimSpace(args.Replaces)
	seen := map[string]bool{}
	for _, raw := range args.DependsOn {
		id := raw
		if _, ok := team[id]; !ok {
			if _, ok := team["delegation-"+raw]; ok {
				id = "delegation-" + raw
			} else {
				plan.Problem = fmt.Sprintf("dependency %q does not name a durable delegation", raw)
				return plan
			}
		}
		if team[id].Status != "quiescent" {
			plan.Problem = fmt.Sprintf("dependency %q is %s, not quiescent", id, team[id].Status)
			return plan
		}
		if !seen[id] {
			seen[id] = true
			plan.DependsOn = append(plan.DependsOn, id)
		}
	}
	return plan
}

// safeCallIDRe: the provider-issued CallID lands in the child journal's
// directory name (<parent>/sub/<call_id>-aN), so it must never carry path
// syntax (M3 security review, defense-in-depth — both wire formats today
// are already safe).
var safeCallIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// roleNameRe bounds a dynamic role's name (INC-12.4): it is untrusted model
// output that lands in the trusted message-attribution prefix, so it must
// carry no newlines or framing characters (安全 review P1).
var roleNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func escalationApprovalReason(spec *AgentSpec) string {
	rules, _ := json.Marshal(spec.Permissions)
	tools, _ := json.Marshal(spec.Tools)
	return fmt.Sprintf("child %q (%s) requests a permission exception; tools remain the parent subset %s; approve its declared rules %s (budget, depth, fan-out, and OS containment remain bounded)",
		spec.Name, spec.Description, tools, rules)
}

func approvalReason(results []event.GateResult) string {
	for i := len(results) - 1; i >= 0; i-- {
		if results[i].Gate == "approval" && results[i].Decision == event.VerdictDeny {
			return results[i].Reason
		}
	}
	return ""
}

func escalationOutcome(spec *AgentSpec) string {
	if !spec.Escalate {
		return ""
	}
	if spec.EscalationApproved {
		return "approved"
	}
	return "denied"
}

func escalationNotice(spec *AgentSpec, reason string) string {
	if !spec.Escalate || spec.EscalationApproved {
		return ""
	}
	if reason == "" {
		reason = "denied by user"
	}
	return "permission escalation denied (" + reason + "); child is running under parent∩child permissions"
}

// renderAgentsDirectory freezes the sub-agent directory block (S5.3): the
// model spawns only what it can see here. Unresolvable names are listed
// without a description rather than hidden — the whitelist is the truth.
func renderAgentsDirectory(names []string, dynamic bool, resolve SubSpecResolver) string {
	if len(names) == 0 && !dynamic {
		return ""
	}
	var b strings.Builder
	b.WriteString("<agents>\nSub-agents you can spawn with the spawn_agent tool:\n")
	if dynamic {
		b.WriteString("- Inline roles are allowed: pass role{name,description,instructions,tools?,permissions?,escalate?} instead of agent.\n")
	}
	for _, name := range names {
		desc := ""
		if resolve != nil {
			if spec, err := resolve(name); err == nil && spec.Description != "" {
				desc = ": " + spec.Description
			}
		}
		fmt.Fprintf(&b, "- %s%s\n", name, desc)
	}
	b.WriteString("</agents>")
	return b.String()
}

// spawnAllowance is the min-aggregated child budget (S5.3): the child may
// spend at most min(parent remaining, child spec cap); zero means unlimited
// on that side. The result is both the effect's reservation (the whole
// allowance reserves up front, settling to actual on completion) and the
// child's own budget cap.
func (l *Loop) spawnAllowance(s state.State, childSpec *AgentSpec) int {
	parentRemaining := 0
	if l.Spec.Budget.MaxTotalTokens > 0 {
		parentRemaining = l.Spec.Budget.MaxTotalTokens - s.Session.Usage.Billed() - s.Budget.ReservedTotal()
		if parentRemaining < 1 {
			parentRemaining = 1 // exhausted: reserve something so the gate denies
		}
	}
	child := childSpec.Budget.MaxTotalTokens
	switch {
	case parentRemaining == 0:
		return child
	case child == 0:
		return parentRemaining
	default:
		return min(parentRemaining, child)
	}
}

// resolveSpawnTarget parses spawn/handoff args and resolves the child spec
// through the whitelist. A failure is a MODEL-visible problem (bad args,
// unknown agent), returned as a message, never a harness error.
func (l *Loop) resolveSpawnTarget(toolName string, rawArgs json.RawMessage) (agent, prompt string, spec *AgentSpec, problem string) {
	agent, prompt, _, spec, problem = l.resolveSpawnTargetFull(toolName, rawArgs)
	return agent, prompt, spec, problem
}

// resolveSpawnTargetFull additionally returns validated artifact inputs
// (S5.8): every ref must resolve in the tree store BEFORE the child starts —
// a dangling input is the parent model's mistake, reported to it.
func (l *Loop) resolveSpawnTargetFull(toolName string, rawArgs json.RawMessage) (agent, prompt string, inputs []event.ArtifactInput, spec *AgentSpec, problem string) {
	var args struct {
		Agent  string                `json:"agent"`
		Role   *InlineRole           `json:"role"`
		Prompt string                `json:"prompt"`
		Inputs []event.ArtifactInput `json:"inputs"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Prompt == "" || (args.Agent == "") == (args.Role == nil) {
		return "", "", nil, nil, toolName + ": invalid args: need prompt and exactly one of agent or role"
	}
	if args.Role != nil {
		if toolName != "spawn_agent" {
			return "", "", nil, nil, toolName + ": inline roles are only supported by spawn_agent"
		}
		if !l.Spec.AgentsDynamic {
			return "", "", nil, nil, toolName + ": inline roles are disabled (agents_dynamic is false)"
		}
		var roleProblem string
		spec, roleProblem = l.dynamicRoleSpec(args.Role)
		if roleProblem != "" {
			return "", "", nil, nil, toolName + ": " + roleProblem
		}
		args.Agent = spec.Name
	} else {
		if !slices.Contains(l.Spec.Agents, args.Agent) {
			return "", "", nil, nil, fmt.Sprintf("%s: %q is not in this agent's directory", toolName, args.Agent)
		}
		if l.SubSpecs == nil {
			return "", "", nil, nil, toolName + ": no sub-agent specs available"
		}
		var err error
		spec, err = l.SubSpecs(args.Agent)
		if err != nil {
			return "", "", nil, nil, fmt.Sprintf("%s: %v", toolName, err)
		}
		if spec.Escalate {
			if problem := l.validateEscalatedChild(spec); problem != "" {
				return "", "", nil, nil, toolName + ": " + problem
			}
		}
	}
	for _, in := range args.Inputs {
		if in.Ref == "" || in.Path == "" {
			return "", "", nil, nil, toolName + ": each input needs {\"ref\", \"path\"}"
		}
		if l.Artifacts == nil {
			return "", "", nil, nil, toolName + ": inputs given but no artifact store"
		}
		if _, err := l.Artifacts.Get(in.Ref); err != nil {
			return "", "", nil, nil, fmt.Sprintf("%s: input ref %s does not resolve", toolName, in.Ref)
		}
	}
	return args.Agent, args.Prompt, args.Inputs, spec, ""
}

func (l *Loop) validateEscalatedChild(spec *AgentSpec) string {
	if len(spec.Permissions) == 0 {
		return "escalate requires at least one explicit permission rule"
	}
	for _, name := range spec.Tools {
		if !slices.Contains(l.Spec.Tools, name) {
			return fmt.Sprintf("escalated child tool %q is not in the parent's tool face", name)
		}
	}
	if len(spec.MCP) > 0 {
		return "escalated child cannot declare MCP servers (approval widens permission rules only)"
	}
	return ""
}

func (l *Loop) dynamicRoleSpec(role *InlineRole) (*AgentSpec, string) {
	if strings.TrimSpace(role.Name) == "" || strings.TrimSpace(role.Description) == "" || strings.TrimSpace(role.Instructions) == "" {
		return nil, "role needs non-empty name, description, and instructions"
	}
	// role.Name is UNTRUSTED model output that lands verbatim in the trusted
	// sender prefix "[message from <name> (<sid>)]" (sendmsg.go) — the
	// harness's only attribution channel. An unconstrained name could inject
	// newlines / ")]" to forge a second "from user" header. Bound it to the
	// same identifier alphabet as a session segment (no newlines, no "]",
	// no parens), so the framing can never be broken (INC-12 安全 review P1).
	if !roleNameRe.MatchString(role.Name) {
		return nil, "role name must be 1-64 chars of [A-Za-z0-9_-] (it appears in message attribution)"
	}
	if role.Escalate && len(role.Permissions) == 0 {
		return nil, "role escalate requires at least one explicit permission rule"
	}
	tools := append([]string(nil), l.Spec.Tools...)
	if role.Tools != nil {
		tools = make([]string, 0, len(role.Tools))
		for _, name := range role.Tools {
			if _, ok := tool.Get(name); !ok {
				return nil, fmt.Sprintf("role tool %q is unknown", name)
			}
			if !slices.Contains(l.Spec.Tools, name) {
				return nil, fmt.Sprintf("role tool %q is not in the parent's tool face", name)
			}
			if !slices.Contains(tools, name) {
				tools = append(tools, name)
			}
		}
	}
	return &AgentSpec{
		Name: strings.TrimSpace(role.Name), Description: strings.TrimSpace(role.Description),
		SystemPrompt: role.Instructions, Model: l.Spec.Model, Tools: tools,
		MaxGenerationSteps: l.Spec.MaxGenerationSteps,
		Permissions:        append([]pipeline.PermissionRule(nil), role.Permissions...),
		Budget:             l.Spec.Budget, AgentsDynamic: l.Spec.AgentsDynamic,
		AgentWorkspace: l.Spec.AgentWorkspace,
		Escalate:       role.Escalate, Receipts: l.Spec.Receipts, Sandbox: l.Spec.Sandbox,
	}, ""
}

func dynamicRoleJSON(rawArgs json.RawMessage, spec *AgentSpec) json.RawMessage {
	var args struct {
		Role json.RawMessage `json:"role"`
	}
	if json.Unmarshal(rawArgs, &args) != nil || len(args.Role) == 0 || string(args.Role) == "null" {
		return nil
	}
	raw, _ := json.Marshal(spec)
	return raw
}

// replacePredecessor fires the cancel for a handle being replaced by a new
// delegation (spawn_agent.replaces, INC-30/G25) — the same action the kill
// tool takes, parent-sourced, so the abandoned member stops spending the
// shared budget and its cancellation settles through the ordinary terminal
// path. Unknown or already-settled handles are a silent no-op: replacement
// is idempotent by design.
func (l *Loop) replacePredecessor(handle string) {
	if handle == "" || l.bg == nil {
		return
	}
	l.bg.mu.Lock()
	cancel, ok := l.bg.cancel[handle]
	l.bg.mu.Unlock()
	if ok {
		cancel(&errs.KilledError{Source: "parent"})
	}
}

// isolationNotice tells an isolated child the workspace mechanics it cannot
// otherwise discover: its tree is a spawn-time snapshot, teammates' later work
// is invisible, and its own writes do not flow back. Without this, members
// burn whole budgets searching for files that can never appear (G24: the
// abandoned-reviewer incident spent 195k tokens exactly this way). Prepended
// to the child's opening prompt — never to SpawnRequested.Prompt, so the parent's
// journaled intent stays verbatim.
const isolationNotice = "[workspace note] You work in an ISOLATED snapshot of the parent's workspace, taken the moment you were spawned. Files teammates create or change after that moment are NOT visible here, and your own file changes stay in your snapshot — they do not flow back automatically. If a file you expect is missing, do not keep searching for it: finish with a short report telling the parent what is missing and ask for the content instead.\n\n"

// isolatedPrompt prefixes the mechanics note onto an isolated child's opening
// prompt; shared children see the parent's real workspace and need no note.
func isolatedPrompt(assignment *event.TeamWorkspace, prompt string) string {
	if assignment == nil || assignment.Mode != "isolated" {
		return prompt
	}
	return isolationNotice + prompt
}

func (l *Loop) prepareChildExecutor(ctx context.Context, childDir, childSession string) (*tool.Executor, *event.TeamWorkspace, error) {
	parentPath := ""
	if l.Exec != nil && l.Exec.WS != nil {
		parentPath = l.Exec.WS.Root()
	}
	if l.Spec.AgentWorkspace != "isolated" {
		return l.Exec, &event.TeamWorkspace{Mode: "shared", Path: parentPath}, nil
	}
	if l.Snapshots == nil {
		return nil, nil, fmt.Errorf("isolated child workspace required but snapshot backend is unavailable")
	}
	root := filepath.Join(childDir, "worktree")
	baseRef := ""
	if st, err := os.Stat(root); err == nil && st.IsDir() {
		// Crash retry after materialize but before SpawnRequested: reuse the
		// already-isolated tree; no shared fallback and no destructive reset.
	} else {
		var err error
		baseRef, err = l.Snapshots.Snapshot(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("snapshot child workspace base: %w", err)
		}
		if err := l.Snapshots.Materialize(ctx, baseRef, root); err != nil {
			return nil, nil, fmt.Errorf("materialize child workspace: %w", err)
		}
	}
	ws, err := workspace.New(root)
	if err != nil {
		return nil, nil, fmt.Errorf("child workspace: %w", err)
	}
	exec := &tool.Executor{WS: ws, Session: childSession}
	if l.Exec != nil {
		exec.ProbeSandbox = l.Exec.ProbeSandbox
		if l.Exec.NetworkContained() {
			exec.ContainNetwork()
		}
	}
	return exec, &event.TeamWorkspace{Mode: "isolated", Path: root, BaseRef: baseRef}, nil
}

// buildHandoffRun is the HANDOFF activity's Run closure (S5.4): control
// transfer runs the successor synchronously — the caller acts no more, so
// there is nothing to parallelize. (spawn_agent is always non-blocking,
// 零 legacy 2026-07-05: the blocking spawn path is gone.) Everything that
// reads the fold (allowance, parent mode) is captured HERE, on the drive
// goroutine; the closure itself runs on the activity goroutine and journals
// only through the serialized appendE. The child is a fresh run in its own
// journal under <parent>/sub/; per-attempt directories keep a retried
// handoff from appending onto a dead child's log.
func (l *Loop) buildHandoffRun(call provider.ToolCall, res *tool.Result,
	appendE AppendFunc, allowance int, parentMode string, escalationApproved bool,
	escalationFallback string, coordination spawnPlan) func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {

	attempt := 0
	return func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
		attempt++
		agentName, prompt, inputs, childSpec, problem := l.resolveSpawnTargetFull(call.Name, call.Args)
		if problem == "" && coordination.Problem != "" {
			problem = call.Name + ": " + coordination.Problem
		}
		if problem == "" && !safeCallIDRe.MatchString(call.CallID) {
			problem = call.Name + ": malformed call id"
		}
		if problem != "" {
			*res = errorResult(problem)
			return res.Payload, nil, true, nil
		}
		frozenChild := *childSpec
		frozenChild.EscalationApproved = escalationApproved
		childSpec = &frozenChild

		l.replacePredecessor(coordination.Replaces)

		childDir := filepath.Join(l.Store.Dir(), "sub", fmt.Sprintf("%s-a%d", call.CallID, attempt))
		childStore, err := store.OpenEventStore(childDir)
		if err != nil {
			return nil, nil, false, fmt.Errorf("spawn %s: %w", agentName, err)
		}
		defer func() { _ = childStore.Close() }()
		childSession := fmt.Sprintf("%s-sub-%s-a%d", l.SessionID, call.CallID, attempt)
		childExec, workspaceAssignment, err := l.prepareChildExecutor(ctx, childDir, childSession)
		if err != nil {
			*res = errorResult(fmt.Sprintf("spawn %s: %v", agentName, err))
			return res.Payload, nil, true, nil
		}

		if _, err := appendE(event.TypeSpawnRequested, &event.SpawnRequested{
			CallID: call.CallID, Agent: agentName, Prompt: prompt,
			ChildSession: childSession, Depth: l.Depth + 1, BudgetTokens: allowance,
			RoleSpec:  dynamicRoleJSON(call.Args, childSpec),
			Escalated: childSpec.EscalationApproved, Escalation: escalationOutcome(childSpec),
			EscalationReason: escalationFallback,
			DelegationID:     coordination.DelegationID, DependsOn: coordination.DependsOn,
			LeaseID: fmt.Sprintf("lease-%s-a%d", call.CallID, attempt), Workspace: workspaceAssignment,
			Replaces: coordination.Replaces,
		}); err != nil {
			return nil, nil, false, err
		}
		l.fireLifecycle(ctx, hook.EventSubagentStart,
			map[string]string{"agent": agentName, "child_session": childSession}, false)

		child := l.childLoopWithExec(childSpec, childStore, childSession, allowance, parentMode, childExec)
		child.Inputs = inputs
		cres, cerr := child.Run(ctx, isolatedPrompt(workspaceAssignment, prompt))
		if cerr != nil {
			// The child journaled real spend before dying — RunResult is
			// zero on aborts, so settle from the child's own fold (S5
			// review: an unsettled failed child would let a re-spawn
			// over-grant against the tree cap).
			spent := childFoldUsage(childDir)
			if ctx.Err() != nil {
				// Cancellation: the parent's cancel path owns the terminal
				// event; the usage rides ActivityCancelled and settles.
				return nil, &spent, false, cerr
			}
			// A failed child is a model-visible result, NOT an activity
			// failure: blindly re-running a whole child run would duplicate
			// its side effects; the parent model decides whether to re-spawn.
			if _, aerr := appendE(event.TypeSubagentCompleted, &event.SubagentCompleted{
				CallID: call.CallID, Agent: agentName, ChildSession: childSession,
				Reason: "error", GenSteps: cres.GenSteps, Usage: spent,
			}); aerr != nil {
				return nil, nil, false, aerr
			}
			l.fireLifecycle(ctx, hook.EventSubagentStop,
				map[string]string{"agent": agentName, "child_session": childSession, "reason": "error"}, false)
			*res = errorResult(fmt.Sprintf("sub-agent %s failed: %s",
				agentName, redact.FromEnv().String(cerr.Error())))
			return res.Payload, &spent, true, nil
		}

		if _, err := appendE(event.TypeSubagentCompleted, &event.SubagentCompleted{
			CallID: call.CallID, Agent: agentName, ChildSession: childSession,
			Reason: cres.Reason, GenSteps: cres.GenSteps, Usage: cres.Usage,
		}); err != nil {
			return nil, nil, false, err
		}
		l.fireLifecycle(ctx, hook.EventSubagentStop,
			map[string]string{"agent": agentName, "child_session": childSession, "reason": cres.Reason}, false)

		// A contract-violating child renders as the parent's ERROR result
		// (S5.6): the deliverables were the point of the delegation. The
		// loop continues — the parent model decides what to do about it.
		isError := cres.Reason == "contract_violation"
		payload, _ := json.Marshal(map[string]any{
			"agent": agentName, "child_session": childSession,
			"reason": cres.Reason, "turns": cres.GenSteps,
			"report":                childReport(childDir),
			"permission_escalation": escalationOutcome(childSpec),
			"escalation_reason":     escalationFallback,
			"workspace":             workspaceAssignment,
		})
		*res = tool.Result{Payload: payload, IsError: isError}
		usage := cres.Usage
		return res.Payload, &usage, isError, nil
	}
}

// launchBackgroundSpawn starts a sub-agent in PARALLEL (v2 M3.1): it
// journals SpawnRequested + ActivityStarted{Background} (the fold pairs the
// call with a handle immediately), registers a cancel, and runs the child on
// a goroutine. When the child finishes it pushes a bgOutcome carrying the
// SubagentCompleted fact and the report; settleBackground journals both at
// the next drive-loop safe point, and the report re-enters as a user message
// — activating the parent's next turn. Runs on the drive goroutine.
func (l *Loop) launchBackgroundSpawn(ctx context.Context, appendE AppendFunc,
	call provider.ToolCall, allowance int, parentMode string, escalationApproved bool,
	escalationFallback string, coordination spawnPlan) error {

	l.ensureBackground()
	agentName, prompt, inputs, childSpec, problem := l.resolveSpawnTargetFull(call.Name, call.Args)
	if problem == "" && coordination.Problem != "" {
		problem = call.Name + ": " + coordination.Problem
	}
	if problem == "" && !safeCallIDRe.MatchString(call.CallID) {
		problem = call.Name + ": malformed call id"
	}
	if problem != "" {
		// A resolve failure (bad args, off-whitelist target) is synchronous
		// and model-visible NOW: it pairs the call as an error result — no
		// handle, no background work ever starts.
		payload, _ := json.Marshal(map[string]any{"error": problem})
		activityID := "tool-" + call.CallID
		if _, err := appendE(event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: activityID, Kind: event.KindTool, Name: call.Name,
			Args: redact.FromEnv().JSON(call.Args), CallID: call.CallID,
			Attempt: 1,
		}); err != nil {
			return err
		}
		_, err := appendE(event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: activityID, Result: payload, IsError: true,
		})
		return err
	}
	frozenChild := *childSpec
	frozenChild.EscalationApproved = escalationApproved
	childSpec = &frozenChild

	// Retire the predecessor BEFORE the successor starts (replaces, G25):
	// its cancellation settles asynchronously through its own goroutine.
	l.replacePredecessor(coordination.Replaces)

	childDir := filepath.Join(l.Store.Dir(), "sub", fmt.Sprintf("%s-a1", call.CallID))
	childStore, err := store.OpenEventStore(childDir)
	if err != nil {
		return fmt.Errorf("spawn %s: %w", agentName, err)
	}
	childSession := fmt.Sprintf("%s-sub-%s-a1", l.SessionID, call.CallID)
	activityID := "tool-" + call.CallID
	childExec, workspaceAssignment, err := l.prepareChildExecutor(ctx, childDir, childSession)
	if err != nil {
		_ = childStore.Close()
		payload, _ := json.Marshal(map[string]any{"error": fmt.Sprintf("spawn %s: %v", agentName, err)})
		if _, aerr := appendE(event.TypeActivityStarted, &event.ActivityStarted{
			ActivityID: activityID, Kind: event.KindTool, Name: call.Name,
			Args: redact.FromEnv().JSON(call.Args), CallID: call.CallID, Attempt: 1,
		}); aerr != nil {
			return aerr
		}
		_, aerr := appendE(event.TypeActivityCompleted, &event.ActivityCompleted{
			ActivityID: activityID, Result: payload, IsError: true,
		})
		return aerr
	}

	if _, err := appendE(event.TypeSpawnRequested, &event.SpawnRequested{
		CallID: call.CallID, Agent: agentName, Prompt: prompt,
		ChildSession: childSession, Depth: l.Depth + 1, BudgetTokens: allowance,
		RoleSpec:  dynamicRoleJSON(call.Args, childSpec),
		Escalated: childSpec.EscalationApproved, Escalation: escalationOutcome(childSpec),
		EscalationReason: escalationFallback,
		DelegationID:     coordination.DelegationID, DependsOn: coordination.DependsOn,
		LeaseID: "lease-" + call.CallID + "-a1", Workspace: workspaceAssignment,
		Replaces: coordination.Replaces,
	}); err != nil {
		_ = childStore.Close()
		return err
	}
	l.fireLifecycle(ctx, hook.EventSubagentStart,
		map[string]string{"agent": agentName, "child_session": childSession}, false)
	if _, err := appendE(event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: activityID, Kind: event.KindTool, Name: call.Name,
		Args: redact.FromEnv().JSON(call.Args), CallID: call.CallID,
		Attempt: 1, Background: true, Notice: escalationNotice(childSpec, escalationFallback),
	}); err != nil {
		_ = childStore.Close()
		return err
	}

	workCtx, cancel := context.WithCancelCause(ctx)
	l.bg.mu.Lock()
	l.bg.cancel[call.CallID] = cancel
	l.bg.mu.Unlock()

	child := l.childLoopWithExec(childSpec, childStore, childSession, allowance, parentMode, childExec)
	child.Inputs = inputs
	go func() {
		defer func() { _ = childStore.Close() }()
		cres, cerr := child.Run(workCtx, isolatedPrompt(workspaceAssignment, prompt))
		spent := cres.Usage
		reason := cres.Reason
		canceled := workCtx.Err() != nil
		if cerr != nil {
			// The child journaled real spend before dying; settle from its
			// own fold so the tree cap stays honest (S5 review).
			spent = childFoldUsage(childDir)
			if canceled {
				reason = "canceled"
			} else if reason == "" {
				reason = "error"
			}
		}
		payload, _ := json.Marshal(map[string]any{
			"agent": agentName, "child_session": childSession,
			"reason": reason, "turns": cres.GenSteps,
			"report":                childReport(childDir),
			"permission_escalation": escalationOutcome(childSpec),
			"escalation_reason":     escalationFallback,
			"workspace":             workspaceAssignment,
		})
		usage := spent
		l.bg.done <- bgOutcome{
			handle: call.CallID, activityID: activityID,
			result: payload, isError: reason == "error" || reason == "contract_violation",
			canceled: canceled, usage: &usage,
			subagent: &event.SubagentCompleted{
				CallID: call.CallID, Agent: agentName, ChildSession: childSession,
				Reason: reason, GenSteps: cres.GenSteps, Usage: spent,
			},
		}
	}()
	return nil
}

// childLoopWithExec builds the frozen child run (S5.3/INC-12.5). Normally
// the child pipeline is parent gates followed by child permissions
// (intersection). The sole widening path is a journaled human-approved
// escalation; OS containment can never widen.
func (l *Loop) childLoopWithExec(childSpec *AgentSpec, childStore *store.EventStore,
	childSession string, allowance int, parentMode string, childExec *tool.Executor) *Loop {
	frozen := *childSpec
	if allowance > 0 {
		frozen.Budget.MaxTotalTokens = allowance
	}
	// Mode never widens, but a child spec MAY be narrower than the parent
	// (DESIGN: "mode 不交集——child 的 mode 独立" bounded by the frozen
	// rules): the child starts in the narrower of the two. bypass in a
	// child spec is rejected at LoadSpec; the clear here is the backstop.
	childMode := narrowerMode(parentMode, childSpec.Mode)
	frozen.Mode = ""

	// The approval REPLACES only the permission intersection (决策 #20 红线:
	// "审批只替换 permission layers"). Every other inherited gate — the hard
	// FloorGate, SpawnGate, AND the hook Gate — stays: hooks are a parallel
	// governance mechanism (决策 #8), not a permission layer, and they can
	// deny (DLP / dangerous-command guards). Dropping them would silently
	// buy off hook enforcement, which the approval prompt never offers
	// (INC-12 安全 review P1). So the escalated branch keeps ALL inherited
	// gates except the PermissionGate, which the child's approved rules
	// supersede below.
	var gates []pipeline.Gate
	if l.Pipeline != nil {
		for _, g := range l.Pipeline.Gates {
			if _, isPerm := g.(*pipeline.PermissionGate); isPerm && childSpec.EscalationApproved {
				continue // the approved rules replace this one
			}
			gates = append(gates, rebindChildGate(g, childExec))
		}
	}
	// Backstops when the parent pipeline lacked a Floor/Spawn gate (a bare
	// test pipeline): an escalated child must still get them.
	if childSpec.EscalationApproved {
		var floorFound, spawnFound bool
		for _, g := range gates {
			switch g.(type) {
			case *pipeline.FloorGate:
				floorFound = true
			case *pipeline.SpawnGate:
				spawnFound = true
			}
		}
		if !floorFound {
			floor := &pipeline.FloorGate{Mode: childMode}
			if childExec != nil {
				floor.WS = childExec.WS
			}
			gates = append(gates, floor)
		}
		if !spawnFound {
			gates = append(gates, &pipeline.SpawnGate{})
		}
	}
	if len(childSpec.Permissions) > 0 {
		gate := &pipeline.PermissionGate{Rules: childSpec.Permissions, Mode: childMode}
		if childExec != nil {
			gate.WS = childExec.WS
		}
		gates = append(gates, gate)
	}
	if allowance > 0 {
		gates = append(gates, &pipeline.BudgetGate{MaxTotalTokens: allowance})
	}

	// Post-tool hooks are inherited UNCONDITIONALLY (INC-12 安全 review P1):
	// an escalation never buys off hook enforcement.
	var childHooks *hook.Runner
	if l.Hooks != nil {
		copy := *l.Hooks
		if childExec != nil && childExec.WS != nil {
			copy.Dir = childExec.WS.Root()
		}
		childHooks = &copy
	}
	var childSnapshots snapshot.Store
	if childExec == l.Exec {
		childSnapshots = l.Snapshots
	} else if childExec != nil && childExec.WS != nil {
		if repo, err := snapshot.NewShadowRepo(filepath.Join(childStore.Dir(), "workspace-shadow.git"), childExec.WS.Root()); err == nil {
			childSnapshots = repo
		}
	}

	return &Loop{
		Spec:      &frozen,
		Provider:  l.Provider,
		Exec:      childExec,
		Store:     childStore,
		Clock:     l.Clock,
		SessionID: childSession,
		Version:   l.Version,
		Pipeline:  &pipeline.Pipeline{Gates: gates},
		Hooks:     childHooks,
		Approvals: l.Approvals, // approvals bubble to the same frontend seam
		Mode:      childMode,
		Depth:     l.Depth + 1,
		SubSpecs:  l.SubSpecs,
		Board:     l.Board,     // the collaboration blackboard is tree-shared (S5.4)
		Artifacts: l.Artifacts, // the deliverable CAS is tree-shared too (S5.5)
		Router:    l.Router,    // the tree message fabric is tree-shared (INC-12)
		Out:       l.Out,       // live events share the root sink, tagged per member (INC-12.6)
		Snapshots: childSnapshots,
	}
}

func rebindChildGate(g pipeline.Gate, exec *tool.Executor) pipeline.Gate {
	var ws *workspace.Workspace
	if exec != nil {
		ws = exec.WS
	}
	switch gate := g.(type) {
	case *pipeline.FloorGate:
		return &pipeline.FloorGate{Mode: gate.Mode, WS: ws}
	case *pipeline.PermissionGate:
		return &pipeline.PermissionGate{Rules: append([]pipeline.PermissionRule(nil), gate.Rules...), Mode: gate.Mode, WS: ws}
	case *hook.Gate:
		copy := *gate.Runner
		if ws != nil {
			copy.Dir = ws.Root()
		}
		return &hook.Gate{Runner: &copy, Notes: gate.Notes}
	default:
		return g
	}
}

// childFoldUsage reads the child's settled usage from its journal — the
// truth even when the child aborted (RunResult carries zero on error paths).
func childFoldUsage(childDir string) provider.Usage {
	events, err := store.ReadEvents(childDir)
	if err != nil {
		return provider.Usage{}
	}
	s, err := state.Fold(events)
	if err != nil {
		return provider.Usage{}
	}
	return s.Session.Usage
}

// narrowerMode picks the stricter of two run modes (S5 review): the mode
// ladder is plan < default < acceptEdits < bypass, empty meaning default.
func narrowerMode(a, b string) string {
	rank := func(m string) int {
		switch m {
		case pipeline.ModePlan:
			return 0
		case "", pipeline.ModeDefault:
			return 1
		case pipeline.ModeAcceptEdits:
			return 2
		case pipeline.ModeBypass:
			return 3
		default:
			return 1 // unknown folds to default; LoadSpec rejects it anyway
		}
	}
	if rank(b) < rank(a) {
		return b
	}
	return a
}

// childReport extracts the child's final assistant text from its journal.
func childReport(childDir string) string {
	events, err := store.ReadEvents(childDir)
	if err != nil {
		return ""
	}
	s, err := state.Fold(events)
	if err != nil {
		return ""
	}
	msgs := assistantMessages(s)
	if len(msgs) == 0 {
		return ""
	}
	return assistantText(msgs[len(msgs)-1])
}

func errorResult(msg string) tool.Result {
	payload, _ := json.Marshal(map[string]string{"error": msg})
	return tool.Result{Payload: payload, IsError: true}
}
