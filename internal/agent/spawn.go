package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
	"github.com/ralphite/agentrunner/internal/tool"
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

// safeCallIDRe: the provider-issued CallID lands in the child journal's
// directory name (<parent>/sub/<call_id>-aN), so it must never carry path
// syntax (M3 security review, defense-in-depth — both wire formats today
// are already safe).
var safeCallIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

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
func (l *Loop) resolveSpawnTarget(toolName string, rawArgs json.RawMessage) (agent, task string, spec *AgentSpec, problem string) {
	agent, task, _, spec, problem = l.resolveSpawnTargetFull(toolName, rawArgs)
	return agent, task, spec, problem
}

// resolveSpawnTargetFull additionally returns validated artifact inputs
// (S5.8): every ref must resolve in the tree store BEFORE the child starts —
// a dangling input is the parent model's mistake, reported to it.
func (l *Loop) resolveSpawnTargetFull(toolName string, rawArgs json.RawMessage) (agent, task string, inputs []event.ArtifactInput, spec *AgentSpec, problem string) {
	var args struct {
		Agent  string                `json:"agent"`
		Role   *InlineRole           `json:"role"`
		Task   string                `json:"task"`
		Inputs []event.ArtifactInput `json:"inputs"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Task == "" || (args.Agent == "") == (args.Role == nil) {
		return "", "", nil, nil, toolName + ": invalid args: need task and exactly one of agent or role"
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
	return args.Agent, args.Task, args.Inputs, spec, ""
}

func (l *Loop) dynamicRoleSpec(role *InlineRole) (*AgentSpec, string) {
	if strings.TrimSpace(role.Name) == "" || strings.TrimSpace(role.Description) == "" || strings.TrimSpace(role.Instructions) == "" {
		return nil, "role needs non-empty name, description, and instructions"
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
		Escalate: role.Escalate, Receipts: l.Spec.Receipts, Sandbox: l.Spec.Sandbox,
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
	appendE AppendFunc, allowance int, parentMode string) func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {

	attempt := 0
	return func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
		attempt++
		agentName, task, inputs, childSpec, problem := l.resolveSpawnTargetFull(call.Name, call.Args)
		if problem == "" && !safeCallIDRe.MatchString(call.CallID) {
			problem = call.Name + ": malformed call id"
		}
		if problem != "" {
			*res = errorResult(problem)
			return res.Payload, nil, true, nil
		}

		childDir := filepath.Join(l.Store.Dir(), "sub", fmt.Sprintf("%s-a%d", call.CallID, attempt))
		childStore, err := store.OpenEventStore(childDir)
		if err != nil {
			return nil, nil, false, fmt.Errorf("spawn %s: %w", agentName, err)
		}
		defer func() { _ = childStore.Close() }()
		childSession := fmt.Sprintf("%s-sub-%s-a%d", l.SessionID, call.CallID, attempt)

		if _, err := appendE(event.TypeSpawnRequested, &event.SpawnRequested{
			CallID: call.CallID, Agent: agentName, Task: task,
			ChildSession: childSession, Depth: l.Depth + 1, BudgetTokens: allowance,
			RoleSpec:  dynamicRoleJSON(call.Args, childSpec),
			Escalated: childSpec.Escalate,
		}); err != nil {
			return nil, nil, false, err
		}

		child := l.childLoopEscalated(childSpec, childStore, childSession, allowance, parentMode, childSpec.Escalate)
		child.Inputs = inputs
		cres, cerr := child.Run(ctx, task)
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

		// A contract-violating child renders as the parent's ERROR result
		// (S5.6): the deliverables were the point of the delegation. The
		// loop continues — the parent model decides what to do about it.
		isError := cres.Reason == "contract_violation"
		payload, _ := json.Marshal(map[string]any{
			"agent": agentName, "child_session": childSession,
			"reason": cres.Reason, "turns": cres.GenSteps,
			"report": childReport(childDir),
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
	call provider.ToolCall, allowance int, parentMode string) error {

	l.ensureBackground()
	agentName, task, inputs, childSpec, problem := l.resolveSpawnTargetFull(call.Name, call.Args)
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

	childDir := filepath.Join(l.Store.Dir(), "sub", fmt.Sprintf("%s-a1", call.CallID))
	childStore, err := store.OpenEventStore(childDir)
	if err != nil {
		return fmt.Errorf("spawn %s: %w", agentName, err)
	}
	childSession := fmt.Sprintf("%s-sub-%s-a1", l.SessionID, call.CallID)
	activityID := "tool-" + call.CallID

	if _, err := appendE(event.TypeSpawnRequested, &event.SpawnRequested{
		CallID: call.CallID, Agent: agentName, Task: task,
		ChildSession: childSession, Depth: l.Depth + 1, BudgetTokens: allowance,
		RoleSpec:  dynamicRoleJSON(call.Args, childSpec),
		Escalated: childSpec.Escalate,
	}); err != nil {
		_ = childStore.Close()
		return err
	}
	if _, err := appendE(event.TypeActivityStarted, &event.ActivityStarted{
		ActivityID: activityID, Kind: event.KindTool, Name: call.Name,
		Args: redact.FromEnv().JSON(call.Args), CallID: call.CallID,
		Attempt: 1, Background: true,
	}); err != nil {
		_ = childStore.Close()
		return err
	}

	taskCtx, cancel := context.WithCancelCause(ctx)
	l.bg.mu.Lock()
	l.bg.cancel[call.CallID] = cancel
	l.bg.mu.Unlock()

	child := l.childLoopEscalated(childSpec, childStore, childSession, allowance, parentMode, childSpec.Escalate)
	child.Inputs = inputs
	go func() {
		defer func() { _ = childStore.Close() }()
		cres, cerr := child.Run(taskCtx, task)
		spent := cres.Usage
		reason := cres.Reason
		canceled := taskCtx.Err() != nil
		if cerr != nil {
			// The child journaled real spend before dying; settle from its
			// own fold so the tree cap stays honest (S5 review).
			spent = childFoldUsage(childDir)
			if reason == "" {
				reason = "error"
			}
		}
		payload, _ := json.Marshal(map[string]any{
			"agent": agentName, "child_session": childSession,
			"reason": reason, "turns": cres.GenSteps,
			"report": childReport(childDir),
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

// childLoop builds the frozen child run (S5.3). The intersection contract:
// the child pipeline is the PARENT's gates followed by the child's own —
// every gate must allow, so the child face can only be narrower. The budget
// cap is the min-aggregated allowance; the mode is the parent's live mode at
// spawn (never wider). Interrupts stay with the parent (a parent cancel
// reaches the child through ctx); the child's surface is silent — its
// report returns as the tool result.
func (l *Loop) childLoop(childSpec *AgentSpec, childStore *store.EventStore,
	childSession string, allowance int, parentMode string) *Loop {
	return l.childLoopEscalated(childSpec, childStore, childSession, allowance, parentMode, false)
}

// childLoopEscalated is childLoop with the USER-APPROVED escalation branch
// (INC-12.5, 决策 #20 修订): an escalated child's pipeline REPLACES the
// parent's frozen permission intersection with the child's own approved
// rules. The escalation reached execution only through an ApprovalRequested
// the user answered (the spawn effect force-asks), so "wider than the
// parent" is always a recorded human decision. Tree budget min-aggregation,
// depth/fan-out caps, and the containment ratchet are NOT exempted.
func (l *Loop) childLoopEscalated(childSpec *AgentSpec, childStore *store.EventStore,
	childSession string, allowance int, parentMode string, escalated bool) *Loop {

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

	var ws *pipeline.PermissionGate
	if l.Pipeline != nil {
		for _, g := range l.Pipeline.Gates {
			if pg, ok := g.(*pipeline.PermissionGate); ok {
				ws = pg
			}
		}
	}
	var gates []pipeline.Gate
	if l.Pipeline != nil && !escalated {
		gates = append(gates, l.Pipeline.Gates...)
	} else if l.Pipeline != nil {
		// Escalated: every NON-permission gate stays inherited (hooks,
		// budget, spawn caps); only the permission intersection is replaced
		// by the approved rules below.
		for _, g := range l.Pipeline.Gates {
			if _, isPerm := g.(*pipeline.PermissionGate); !isPerm {
				gates = append(gates, g)
			}
		}
	}
	if allowance > 0 {
		gates = append(gates, &pipeline.BudgetGate{MaxTotalTokens: allowance})
	}
	if len(childSpec.Permissions) > 0 || escalated {
		gate := &pipeline.PermissionGate{Rules: childSpec.Permissions}
		if ws != nil {
			gate.WS = ws.WS
		}
		gates = append(gates, gate)
	}

	return &Loop{
		Spec:      &frozen,
		Provider:  l.Provider,
		Exec:      l.Exec,
		Store:     childStore,
		Clock:     l.Clock,
		SessionID: childSession,
		Version:   l.Version,
		Pipeline:  &pipeline.Pipeline{Gates: gates},
		Approvals: l.Approvals, // approvals bubble to the same frontend seam
		Mode:      childMode,
		Depth:     l.Depth + 1,
		SubSpecs:  l.SubSpecs,
		Board:     l.Board,     // the collaboration blackboard is tree-shared (S5.4)
		Artifacts: l.Artifacts, // the deliverable CAS is tree-shared too (S5.5)
		Router:    l.Router,    // the tree message fabric is tree-shared (INC-12)
		Out:       l.Out,       // live events share the root sink, tagged per member (INC-12.6)
		// Snapshots deliberately NOT inherited: barriers are cut by the tree
		// root only — the root's vector already covers child streams (S7.2).
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
