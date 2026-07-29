// Approval "allow and don't ask again" (INC-17, G5). When an approve carries
// Remember, the approved effect's criterion is written back to the USER
// settings as an allow rule so the NEXT session no longer asks. Taking the
// "next session" path (决策 INC-D5 取 A) keeps the current run's frozen
// PermissionLayers untouched — no invariant is crossed. Writing the USER
// layer (not project) means the rule is never downgraded by an untrusted
// workspace (config.Merge tightens project allows to ask until trusted); the
// tradeoff is that it is global, softened by matching the EXACT command/path.
package agent

import (
	"encoding/json"

	"github.com/ralphite/agentrunner/internal/config"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/runtime"
)

// rememberApproval persists an approved effect's criterion to the user config
// (INC-17). Best effort: any failure emits a note but never fails the
// approval — the user already approved this call. The write is idempotent
// (config.AppendRule dedups), so a replayed approval never double-writes.
func (l *Loop) rememberApproval(req event.ApprovalRequested) {
	rule, ok := rememberRule(req)
	if !ok {
		return
	}
	path, err := runtime.UserConfigPath()
	if err != nil {
		l.emit(protocol.Event{Kind: protocol.KindMessage, Text: "remember: " + err.Error()})
		return
	}
	added, err := config.AppendRule(path, rule)
	if err != nil {
		l.emit(protocol.Event{Kind: protocol.KindMessage, Text: "remember: " + err.Error()})
		return
	}
	if added {
		l.emit(protocol.Event{Kind: protocol.KindMessage,
			Text: "remembered: future sessions will allow this (" + rule.Tool + ") without asking"})
	}
}

// rememberRule derives the allow rule to persist for an approved effect. It
// matches EXACTLY (not a broad glob): the precise command for bash, the
// precise path for file edits — a `git push` approval must never widen into
// `git *` (which would allow `git reset --hard`). Returns (rule, true) for
// effects worth remembering; (zero, false) otherwise (e.g. no usable
// criterion), in which case the approve simply does not persist anything.
func rememberRule(req event.ApprovalRequested) (pipeline.PermissionRule, bool) {
	c, ok := standingCriterion(req.ToolName, req.Args)
	if !ok {
		return pipeline.PermissionRule{}, false
	}
	return pipeline.PermissionRule{Tool: c.Tool, Path: c.Path, Command: c.Command, Action: "allow"}, true
}

// standingCriterion extracts the ONE exact criterion an always-allow answer
// stands for (INC-62). Both memories derive from this single function — the
// in-session standing answer (Effects.Standing) and the next-session config
// rule (rememberRule) — so they can never disagree about what was approved.
func standingCriterion(toolName string, args json.RawMessage) (event.StandingRule, bool) {
	if toolName == "" {
		return event.StandingRule{}, false
	}
	var a struct {
		Command string `json:"command"`
		Path    string `json:"path"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return event.StandingRule{}, false
		}
	}
	switch toolName {
	case "bash":
		if a.Command == "" {
			return event.StandingRule{}, false
		}
		return event.StandingRule{Tool: "bash", Command: a.Command}, true
	case "edit_file", "write_file", "notebook_edit":
		if a.Path == "" {
			return event.StandingRule{}, false
		}
		return event.StandingRule{Tool: toolName, Path: a.Path}, true
	case "spawn_agent":
		// Tool-level (G35 裁定): "always allow spawning" is the user's
		// intent, and PermissionRule has no agent dimension — scoping to one
		// child name would silently re-ask on the next teammate and repeat
		// the exact failure G35 records.
		return event.StandingRule{Tool: "spawn_agent"}, true
	default:
		// Other execute-class tools (e.g. web_fetch) are not remembered in
		// this first cut — their criteria (host allowlists, etc.) deserve a
		// dedicated shape rather than an exact-arg match.
		return event.StandingRule{}, false
	}
}

// grantApprovedPath hands an approved out-of-workspace path to the executor.
//
// The permission gate asks about such a path rather than denying it (LOG
// 2026-07-29), but the ask alone is not enough: the file tools bound every
// path with WS.Resolve and the bash sandbox makes only the workspace writable,
// so an approved edit to ~/.zshrc would still fail at execution. The grant is
// what carries the human's answer across to both.
//
// Deliberately narrow: the exact path that was approved, this session only,
// held in the executor's memory rather than written anywhere. A grant is a
// decision about the work in front of you, not a standing widening of the
// boundary — the next session asks again.
func (l *Loop) grantApprovedPath(args json.RawMessage) {
	if l.Exec == nil || len(args) == 0 {
		return
	}
	var a struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil || a.Path == "" {
		return
	}
	l.Exec.GrantPath(a.Path)
}
