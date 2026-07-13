package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// PermissionRule is one row of the permissions list (3.3). Sources are
// concatenated user > project > spec and the FIRST matching rule wins —
// order is precedence.
type PermissionRule struct {
	Tool    string `yaml:"tool,omitempty" json:"tool,omitempty"`       // exact tool name; empty = any
	Path    string `yaml:"path,omitempty" json:"path,omitempty"`       // glob over the workspace-relative path (** crosses /)
	Command string `yaml:"command,omitempty" json:"command,omitempty"` // glob over the whole command (* matches anything)
	// Network matches the effect's egress scope (S7 模块 5): an uncontained
	// execute-class effect carries "all"; an OS-network-contained one carries no
	// scope and network rules never match it. `network: "*"` therefore
	// means "any execution that WOULD have network egress".
	Network string `yaml:"network,omitempty" json:"network,omitempty"`
	Action  string `yaml:"action" json:"action"` // allow | ask | deny
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
	// A command tool (INC-55) supplies its FIXED manifest command out-of-band
	// (eff.Command); every other effect's command rides its args. Either way
	// the gate adjudicates one command string with identical machinery.
	command := args.Command
	if eff.Command != "" {
		command = eff.Command
	}

	// exit_plan_mode is transition policy, not rule material: leaving plan
	// mode always requires approval (3.6c); outside plan it is meaningless.
	if eff.ToolName == "exit_plan_mode" {
		if g.effectiveMode(eff) == ModePlan {
			return Ask("exit plan mode → default")
		}
		return Deny("not in plan mode")
	}

	// A command with top-level separators is adjudicated PER SEGMENT
	// (INC-16, #53): one allow-matched segment may not wave through the rest.
	// The whole command's verdict is the STRICTEST segment verdict — a single
	// unmatched or asked/denied segment holds back the compound. Non-command
	// effects (empty command) fall through to the single-shot path below.
	if command != "" {
		if segs := splitCompound(command); len(segs) > 1 {
			return g.adjudicateSegments(eff, relPath, args, command, segs)
		}
	}

	if d, matched := g.matchOneCommand(eff, relPath, args, command); matched {
		return d
	}
	d := modeDefault(g.effectiveMode(eff), eff.Class)
	// Protected write paths (INC-18, #59): acceptEdits auto-allows every edit,
	// but a WRITE to a sensitive config/system path (.git, .claude, rc files,
	// …) must still be approved. This ONLY re-tightens the mode default — an
	// explicit allow rule (matched above) and bypass both still pass; it never
	// denies, only escalates the auto-allow to ask.
	if d.Action == event.VerdictAllow && eff.Class == "edit" &&
		g.effectiveMode(eff) != ModeBypass && isProtectedWritePath(relPath) {
		return Ask("protected path: writing " + relPath + " requires approval")
	}
	return d
}

// adjudicateSegments takes the strictest verdict across a compound command's
// segments (deny > ask > allow). An unmatched segment contributes the mode
// default — so with a `Bash(git *)` allow, `git status && rm -rf x` is NOT
// allowed: the rm segment matches no rule and falls to the default (ask in
// default mode), which dominates the git segment's allow.
func (g *PermissionGate) adjudicateSegments(eff Effect, relPath string, args toolArgs, command string, segs []string) Decision {
	worst := Allow
	worstReason := ""
	rank := func(d Decision) int {
		switch d.Action {
		case event.VerdictDeny:
			return 2
		case event.VerdictAsk:
			return 1
		default:
			return 0
		}
	}
	for _, seg := range segs {
		d, matched := g.matchOneCommand(eff, relPath, args, seg)
		if !matched {
			d = modeDefault(g.effectiveMode(eff), eff.Class)
		}
		if rank(d) > rank(worst) {
			worst, worstReason = d, d.Reason
			if seg != command {
				worstReason = "segment " + strconv.Quote(seg) + ": " + d.Reason
			}
		}
	}
	if worst.Action != event.VerdictAllow && worstReason != "" {
		worst.Reason = worstReason
	}
	return worst
}

// matchOneCommand evaluates ONE command string (a whole command or one
// segment). It first honors the read-only allow-set (a wrapper-stripped
// builtin like `ls` needs no rule), then walks the rules with the command
// matched under wrapper stripping so `timeout 60 npm test` still hits
// `Bash(npm test)`. Returns (decision, matched); matched=false means no rule
// applied and the caller supplies the mode default.
func (g *PermissionGate) matchOneCommand(eff Effect, relPath string, args toolArgs, command string) (Decision, bool) {
	stripped := command
	if command != "" {
		stripped = stripWrappers(command)
	}
	// Rules FIRST: a deny/ask rule must beat the read-only set — an explicit
	// `deny cat *` outranks cat's read-only status (SECURITY: relaxation
	// never overrides an operator's explicit restriction).
	for i, rule := range g.Rules {
		// Try the command as written AND wrapper-stripped: a rule may target
		// either the bare command or the wrapped form.
		matched := rule.matches(eff.ToolName, relPath, args.Path, command, eff.Network) ||
			(stripped != command && rule.matches(eff.ToolName, relPath, args.Path, stripped, eff.Network))
		if !matched {
			continue
		}
		if rule.isCatchAllAsk() {
			continue
		}
		switch rule.Action {
		case event.VerdictAllow:
			return Allow, true
		case event.VerdictAsk:
			return Ask(fmt.Sprintf("rule %d: %s", i+1, rule.describe())), true
		case event.VerdictDeny:
			return Deny(fmt.Sprintf("rule %d: %s", i+1, rule.describe())), true
		default:
			return Deny(fmt.Sprintf("rule %d has invalid action %q", i+1, rule.Action)), true
		}
	}
	// No rule matched: a read-only builtin is safe without one (INC-16), so
	// it is allowed rather than falling to the mode default. Only for actual
	// bash commands (never path/network effects). The OS sandbox still bounds
	// what `cat`/`ls` can reach (决策 #34).
	if command != "" && isReadOnlyCommand(stripped) {
		return Allow, true
	}
	return Decision{}, false
}

// hardFloor returns the unconditional denials that precede rules AND mode
// defaults: a workspace escape, and plan mode's edit/execute prohibition.
// No permission rule and no mode (not even bypass, for the escape) may
// override these. exit_plan_mode is exempt from the plan-mode floor — it
// is the sanctioned way out.
func (g *PermissionGate) hardFloor(eff Effect) (Decision, bool) {
	args := effArgs(eff)
	relPath, escaped := g.resolveRel(args.Path)
	if escaped {
		return Deny(fmt.Sprintf("path escapes workspace: %s", args.Path)), true
	}
	// Credential files never reach the model (C3): read_file on a hard-excluded
	// credential path is denied at the floor, no rule or mode may override.
	// The snapshot layer already refuses to checkpoint these same files; this
	// closes the read side. Bash is independently bounded by the mandatory OS
	// workspace sandbox, so `cat` cannot bypass this floor.
	if eff.ToolName == "read_file" && args.Path != "" && isCredentialPath(relPath) {
		return Deny(fmt.Sprintf("credential files are not readable (hard floor): %s", args.Path)), true
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

func (r PermissionRule) isCatchAllAsk() bool {
	return r.Action == event.VerdictAsk && r.Tool == "" && r.Path == "" && r.Command == "" && r.Network == ""
}

// credentialBasenames name files whose contents are credentials. A read of
// any of them — at any depth in the workspace — is denied by the hard floor
// (C3). Mirrors snapshot.hardExcludes (which refuses to checkpoint the same
// files); keep the two lists in sync.
var credentialBasenames = []string{
	".env", ".env.*", ".envrc",
	"*.pem", "*.key",
	"id_rsa", "id_rsa.*", "id_ed25519", "id_ed25519.*",
	".git-credentials", ".netrc", ".npmrc", ".pypirc",
	"credentials.json",
}

// isCredentialPath reports whether a workspace-relative (forward-slash) path
// names a credential file whose contents must never reach the model. Matches
// the basename at any depth, plus anything under a .ssh/ dir and .aws/credentials.
func isCredentialPath(rel string) bool {
	if rel == "" {
		return false
	}
	rel = strings.TrimPrefix(rel, "./")
	base := rel
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		base = rel[i+1:]
	}
	for _, pat := range credentialBasenames {
		if ok, _ := path.Match(pat, base); ok {
			return true
		}
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".ssh" {
			return true
		}
	}
	if base == "credentials" && strings.Contains(rel+"/", ".aws/") {
		return true
	}
	return false
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
