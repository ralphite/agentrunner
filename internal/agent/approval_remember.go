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
	if req.ToolName == "" || len(req.Args) == 0 {
		return pipeline.PermissionRule{}, false
	}
	var a struct {
		Command string `json:"command"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(req.Args, &a); err != nil {
		return pipeline.PermissionRule{}, false
	}
	switch req.ToolName {
	case "bash":
		if a.Command == "" {
			return pipeline.PermissionRule{}, false
		}
		return pipeline.PermissionRule{Tool: "bash", Command: a.Command, Action: "allow"}, true
	case "edit_file", "write_file", "notebook_edit":
		if a.Path == "" {
			return pipeline.PermissionRule{}, false
		}
		return pipeline.PermissionRule{Tool: req.ToolName, Path: a.Path, Action: "allow"}, true
	default:
		// Other execute-class tools (e.g. web_fetch) are not remembered in
		// this first cut — their criteria (host allowlists, etc.) deserve a
		// dedicated shape rather than an exact-arg match.
		return pipeline.PermissionRule{}, false
	}
}
