package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// PermissionRule is one row of the permissions list (3.3). Sources are
// concatenated user > project > spec and the FIRST matching rule wins —
// order is precedence.
type PermissionRule struct {
	Tool    string `yaml:"tool,omitempty"`    // exact tool name; empty = any
	Path    string `yaml:"path,omitempty"`    // glob over the workspace-relative path (** crosses /)
	Command string `yaml:"command,omitempty"` // glob over the whole command (* matches anything)
	// Network matches the effect's egress scope (S7 模块 5): an uncontained
	// execute-class effect carries "all"; a netns-contained one carries no
	// scope and network rules never match it. `network: "*"` therefore
	// means "any execution that WOULD have network egress".
	Network string `yaml:"network,omitempty"`
	Action  string `yaml:"action"` // allow | ask | deny
}

// Run modes and their no-rule-matched defaults per tool class.
const (
	ModeDefault     = "default"
	ModePlan        = "plan"
	ModeAcceptEdits = "acceptEdits"
	ModeBypass      = "bypass"
)

// PermissionGate adjudicates tool effects against the rule list, falling
// back to the mode's per-class defaults.
type PermissionGate struct {
	Rules []PermissionRule
	Mode  string // empty = ModeDefault
	WS    *workspace.Workspace
}

func (g *PermissionGate) Name() string { return "permission" }

func (g *PermissionGate) Check(_ context.Context, eff Effect) Decision {
	if eff.Kind != "tool_call" {
		return Allow // LLM calls are budget's business (3.7)
	}

	// Hard denials that NO rule may override run first (they also live in
	// FloorGate, ahead of hooks; this is defense in depth).
	if d, hard := g.hardFloor(eff); hard {
		return d
	}

	args := effArgs(eff)
	relPath, _ := g.resolveRel(args.Path)

	// exit_plan_mode is transition policy, not rule material: leaving plan
	// mode always requires approval (3.6c); outside plan it is meaningless.
	if eff.ToolName == "exit_plan_mode" {
		if g.effectiveMode(eff) == ModePlan {
			return Ask("exit plan mode → default")
		}
		return Deny("not in plan mode")
	}

	for i, rule := range g.Rules {
		if rule.matches(eff.ToolName, relPath, args.Path, args.Command, eff.Network) {
			switch rule.Action {
			case event.VerdictAllow:
				return Allow
			case event.VerdictAsk:
				return Ask(fmt.Sprintf("rule %d: %s", i+1, rule.describe()))
			case event.VerdictDeny:
				return Deny(fmt.Sprintf("rule %d: %s", i+1, rule.describe()))
			default:
				return Deny(fmt.Sprintf("rule %d has invalid action %q", i+1, rule.Action))
			}
		}
	}
	return modeDefault(g.effectiveMode(eff), eff.Class)
}

// hardFloor returns the unconditional denials that precede rules AND mode
// defaults: a workspace escape, and plan mode's edit/execute prohibition.
// No permission rule and no mode (not even bypass, for the escape) may
// override these. exit_plan_mode is exempt from the plan-mode floor — it
// is the sanctioned way out.
func (g *PermissionGate) hardFloor(eff Effect) (Decision, bool) {
	args := effArgs(eff)
	if _, escaped := g.resolveRel(args.Path); escaped {
		return Deny(fmt.Sprintf("path escapes workspace: %s", args.Path)), true
	}
	if g.effectiveMode(eff) == ModePlan && eff.ToolName != "exit_plan_mode" &&
		(eff.Class == "edit" || eff.Class == "execute") {
		return Deny("plan mode: " + eff.Class + " tools are disabled (no rule can override)"), true
	}
	return Decision{}, false
}

// FloorGate enforces the hard-deny floor BEFORE any other gate (hooks
// included), so a to-be-denied effect never triggers a side-effecting
// pre-hook and no rule can grant what the floor forbids. It is pure.
type FloorGate struct {
	Mode string
	WS   *workspace.Workspace
}

func (g *FloorGate) Name() string { return "floor" }

func (g *FloorGate) Check(_ context.Context, eff Effect) Decision {
	if eff.Kind != "tool_call" {
		return Allow
	}
	pg := &PermissionGate{Mode: g.Mode, WS: g.WS}
	if d, hard := pg.hardFloor(eff); hard {
		return d
	}
	return Allow
}

// effectiveMode prefers the effect's mode (live fold state — the mode can
// change mid-run) over the gate's construction-time fallback.
func (g *PermissionGate) effectiveMode(eff Effect) string {
	if eff.Mode != "" {
		return eff.Mode
	}
	if g.Mode == "" {
		return ModeDefault
	}
	return g.Mode
}

// modeDefault is the no-rule-matched policy table (S3 执行包). Bypass
// allows everything; otherwise read/wait always allow and any UNKNOWN
// class fails closed (never a silent allow for an unclassified tool).
func modeDefault(mode, class string) Decision {
	if mode == ModeBypass {
		return Allow
	}
	if class == "read" || class == "wait" {
		return Allow
	}
	known := class == "edit" || class == "execute"
	switch mode {
	case ModePlan:
		if known {
			return Deny("plan mode: " + class + " tools are disabled")
		}
		return Deny("plan mode: unclassified tool disabled")
	case ModeAcceptEdits:
		if class == "edit" {
			return Allow
		}
		if class == "execute" {
			return Ask("execute requires approval")
		}
		return Ask("unclassified tool requires approval")
	default: // ModeDefault
		if known {
			return Ask(class + " requires approval")
		}
		return Ask("unclassified tool requires approval")
	}
}

type toolArgs struct {
	Path    string `json:"path"`
	Command string `json:"command"`
}

func effArgs(eff Effect) toolArgs {
	var args toolArgs
	_ = json.Unmarshal(eff.Args, &args) // malformed args fail at execution; gate on what parses
	return args
}

// resolveRel maps the tool's path arg to a workspace-relative path.
// escaped=true means the path resolves outside the workspace.
func (g *PermissionGate) resolveRel(path string) (string, bool) {
	if path == "" || g.WS == nil {
		return "", false
	}
	abs, err := g.WS.Resolve(path)
	if err != nil {
		return "", true
	}
	rel := strings.TrimPrefix(abs, g.WS.Root())
	return strings.TrimPrefix(rel, "/"), false
}

func (r PermissionRule) matches(toolName, relPath, rawPath, command, network string) bool {
	if r.Tool != "" && r.Tool != toolName {
		return false
	}
	if r.Path != "" {
		if rawPath == "" || !globMatch(r.Path, relPath, true) {
			return false
		}
	}
	if r.Command != "" {
		if command == "" || !globMatch(r.Command, command, false) {
			return false
		}
	}
	if r.Network != "" {
		if network == "" || !globMatch(r.Network, network, false) {
			return false
		}
	}
	return true
}

func (r PermissionRule) describe() string {
	parts := []string{}
	if r.Tool != "" {
		parts = append(parts, "tool="+r.Tool)
	}
	if r.Path != "" {
		parts = append(parts, "path="+r.Path)
	}
	if r.Command != "" {
		parts = append(parts, "command="+r.Command)
	}
	if r.Network != "" {
		parts = append(parts, "network="+r.Network)
	}
	if len(parts) == 0 {
		parts = append(parts, "any")
	}
	return strings.Join(parts, " ") + " → " + r.Action
}

// globMatch translates a glob to a regexp. Path semantics (pathish=true):
// `*`/`?` stop at "/", `**` crosses. Command semantics: `*` matches
// anything including spaces and slashes.
func globMatch(pattern, s string, pathish bool) bool {
	var sb strings.Builder
	// (?s): `.`/`.*` cross newlines, so a command deny rule cannot be
	// evaded by putting the dangerous part on a second line. (?i) on paths
	// closes case-folding evasion on case-insensitive filesystems.
	if pathish {
		sb.WriteString("(?is)^")
	} else {
		sb.WriteString("(?s)^")
	}
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if pathish {
				if i+1 < len(pattern) && pattern[i+1] == '*' {
					sb.WriteString(".*")
					i++
				} else {
					sb.WriteString("[^/]*")
				}
			} else {
				sb.WriteString(".*")
			}
		case '?':
			if pathish {
				sb.WriteString("[^/]")
			} else {
				sb.WriteString(".")
			}
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString("$")
	re, err := regexp.Compile(sb.String())
	if err != nil {
		return false
	}
	return re.MatchString(s)
}
