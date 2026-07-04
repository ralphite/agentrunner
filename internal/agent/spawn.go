package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
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

// renderAgentsDirectory freezes the sub-agent directory block (S5.3): the
// model spawns only what it can see here. Unresolvable names are listed
// without a description rather than hidden — the whitelist is the truth.
func renderAgentsDirectory(names []string, resolve SubSpecResolver) string {
	if len(names) == 0 || resolve == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("<agents>\nSub-agents you can spawn with the spawn_agent tool:\n")
	for _, name := range names {
		desc := ""
		if spec, err := resolve(name); err == nil && spec.Description != "" {
			desc = ": " + spec.Description
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
		parentRemaining = l.Spec.Budget.MaxTotalTokens - s.Run.Usage.Billed() - s.Budget.ReservedTotal()
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
	var args struct {
		Agent string `json:"agent"`
		Task  string `json:"task"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Agent == "" || args.Task == "" {
		return "", "", nil, toolName + ": invalid args: need {\"agent\", \"task\"}"
	}
	if !slices.Contains(l.Spec.Agents, args.Agent) {
		return "", "", nil, fmt.Sprintf("%s: %q is not in this agent's directory", toolName, args.Agent)
	}
	if l.SubSpecs == nil {
		return "", "", nil, toolName + ": no sub-agent specs available"
	}
	spec, err := l.SubSpecs(args.Agent)
	if err != nil {
		return "", "", nil, fmt.Sprintf("%s: %v", toolName, err)
	}
	return args.Agent, args.Task, spec, ""
}

// buildSpawnRun is the spawn activity's Run closure (S5.3). Everything that
// reads the fold (allowance, parent mode) is captured HERE, on the drive
// goroutine; the closure itself runs on the activity goroutine and journals
// only through the serialized appendE. The child is a fresh run in its own
// journal under <parent>/sub/; per-attempt directories keep a retried spawn
// from appending onto a dead child's log.
func (l *Loop) buildSpawnRun(call provider.ToolCall, res *tool.Result,
	appendE AppendFunc, allowance int, parentMode string) func(context.Context) (json.RawMessage, *provider.Usage, bool, error) {

	attempt := 0
	return func(ctx context.Context) (json.RawMessage, *provider.Usage, bool, error) {
		attempt++
		agentName, task, childSpec, problem := l.resolveSpawnTarget(call.Name, call.Args)
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
		}); err != nil {
			return nil, nil, false, err
		}

		child := l.childLoop(childSpec, childStore, childSession, allowance, parentMode)
		cres, cerr := child.Run(ctx, task)
		if cerr != nil {
			if ctx.Err() != nil {
				return nil, nil, false, cerr // cancellation: parent's cancel path owns it
			}
			// A failed child is a model-visible result, NOT an activity
			// failure: blindly re-running a whole child run would duplicate
			// its side effects; the parent model decides whether to re-spawn.
			if _, aerr := appendE(event.TypeSubagentCompleted, &event.SubagentCompleted{
				CallID: call.CallID, Agent: agentName, ChildSession: childSession,
				Reason: "error", Turns: cres.Turns, Usage: cres.Usage,
			}); aerr != nil {
				return nil, nil, false, aerr
			}
			*res = errorResult(fmt.Sprintf("sub-agent %s failed: %s",
				agentName, redact.FromEnv().String(cerr.Error())))
			usage := cres.Usage
			return res.Payload, &usage, true, nil
		}

		if _, err := appendE(event.TypeSubagentCompleted, &event.SubagentCompleted{
			CallID: call.CallID, Agent: agentName, ChildSession: childSession,
			Reason: cres.Reason, Turns: cres.Turns, Usage: cres.Usage,
		}); err != nil {
			return nil, nil, false, err
		}

		// A contract-violating child renders as the parent's ERROR result
		// (S5.6): the deliverables were the point of the delegation. The
		// loop continues — the parent model decides what to do about it.
		isError := cres.Reason == "contract_violation"
		payload, _ := json.Marshal(map[string]any{
			"agent": agentName, "child_session": childSession,
			"reason": cres.Reason, "turns": cres.Turns,
			"report": childReport(childDir),
		})
		*res = tool.Result{Payload: payload, IsError: isError}
		usage := cres.Usage
		return res.Payload, &usage, isError, nil
	}
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

	frozen := *childSpec
	if allowance > 0 {
		frozen.Budget.MaxTotalTokens = allowance
	}
	frozen.Mode = "" // the parent's live mode wins; the child spec cannot widen

	var gates []pipeline.Gate
	if l.Pipeline != nil {
		gates = append(gates, l.Pipeline.Gates...)
	}
	if allowance > 0 {
		gates = append(gates, &pipeline.BudgetGate{MaxTotalTokens: allowance})
	}
	if len(childSpec.Permissions) > 0 {
		var ws *pipeline.PermissionGate
		if l.Pipeline != nil {
			for _, g := range l.Pipeline.Gates {
				if pg, ok := g.(*pipeline.PermissionGate); ok {
					ws = pg
				}
			}
		}
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
		Mode:      parentMode,
		Depth:     l.Depth + 1,
		SubSpecs:  l.SubSpecs,
		Board:     l.Board,     // the collaboration blackboard is tree-shared (S5.4)
		Artifacts: l.Artifacts, // the deliverable CAS is tree-shared too (S5.5)
	}
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
