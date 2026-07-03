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
	Action  string `yaml:"action"`            // allow | ask | deny
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

	args := effArgs(eff)

	// Workspace escape denies unconditionally — before rules, before mode,
	// even in bypass (standing hook 1: the boundary is not a preference).
	relPath, escaped := g.resolveRel(args.Path)
	if escaped {
		return Deny(fmt.Sprintf("path escapes workspace: %s", args.Path))
	}

	for i, rule := range g.Rules {
		if rule.matches(eff.ToolName, relPath, args.Path, args.Command) {
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
	return modeDefault(g.mode(), eff.Class)
}

func (g *PermissionGate) mode() string {
	if g.Mode == "" {
		return ModeDefault
	}
	return g.Mode
}

// modeDefault is the no-rule-matched policy table (S3 执行包).
func modeDefault(mode, class string) Decision {
	switch mode {
	case ModeBypass:
		return Allow
	case ModePlan:
		switch class {
		case "edit", "execute":
			return Deny("plan mode: " + class + " tools are disabled")
		}
		return Allow
	case ModeAcceptEdits:
		switch class {
		case "edit":
			return Allow
		case "execute":
			return Ask("execute requires approval")
		}
		return Allow
	default: // ModeDefault
		switch class {
		case "edit":
			return Ask("edit requires approval")
		case "execute":
			return Ask("execute requires approval")
		}
		return Allow
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

func (r PermissionRule) matches(toolName, relPath, rawPath, command string) bool {
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
	sb.WriteString("^")
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
