package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ralphite/agentrunner/internal/provider"
	"gopkg.in/yaml.v3"
)

// skill context:fork (INC-31, #45/§3.5 余项): a skill whose frontmatter
// declares `context: fork` runs in a ONE-SHOT sub-agent instead of inline.
// The mechanism is ingest expansion — the same precedent as 命令=用户宏: the
// model's skill(...) call is rewritten into spawn_agent{role} BEFORE the
// assistant message is journaled, so the fold, the effect pipeline (tree
// budget, depth/fan-out caps, approvals), the frozen RoleSpec event, and
// crash replay all see an ordinary dynamic-role spawn. Zero spawn-machinery
// changes; replay never re-runs the transform (the journal already carries
// the expanded call).
//
// Gate: expansion happens ONLY under `agents_dynamic: true`. A skill file is
// workspace content — without the spec author's opt-in it must not widen the
// multi-agent face (决策: 多 agent 面永不静默变宽). With the gate off, a
// fork skill simply runs inline like any other (the safe degradation).

// skillFrontmatter is the harness-read slice of a SKILL.md frontmatter.
// Only fork-relevant fields are parsed; model/hooks/budget deliberately do
// not exist here (InlineRole's harness-control ruling stands).
type skillFrontmatter struct {
	Description  string   `yaml:"description"`
	Context      string   `yaml:"context"`
	AllowedTools []string `yaml:"allowed-tools"`
}

// expandForkSkills rewrites every skill(...) tool call that targets a
// context:fork skill into the spawn_agent{role} call it expands to. Message
// parts and the collected ToolCalls are rewritten together so the journaled
// assistant message and the dispatch view never disagree. Calls that do not
// expand (gate off, unknown skill, no fork, empty body, unsafe name) are left
// untouched — the inline executor path stays authoritative for them.
func (l *Loop) expandForkSkills(turn *provider.GenStep) {
	if !l.Spec.AgentsDynamic || l.Exec == nil || l.Exec.WS == nil {
		return
	}
	for i := range turn.Message.Parts {
		p := &turn.Message.Parts[i]
		if p.Kind != provider.PartToolCall || p.ToolName != "skill" {
			continue
		}
		expanded, ok := l.forkSkillArgs(p.Args)
		if !ok {
			continue
		}
		p.ToolName = "spawn_agent"
		p.Args = expanded
		for j := range turn.ToolCalls {
			if turn.ToolCalls[j].CallID == p.CallID {
				turn.ToolCalls[j].Name = "spawn_agent"
				turn.ToolCalls[j].Args = expanded
			}
		}
	}
}

// forkSkillArgs loads the named skill and, when it declares context:fork,
// returns the spawn_agent{role, task} args it expands to. ok=false keeps the
// call on the inline path — the executor's own error reporting stays
// authoritative for missing/invalid skills.
func (l *Loop) forkSkillArgs(raw json.RawMessage) (json.RawMessage, bool) {
	var args struct {
		Name string `json:"name"`
		Task string `json:"task"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || args.Name == "" {
		return nil, false
	}
	// Same identifier discipline as the skill executor (no separators, no
	// traversal) PLUS the role attribution bound: the skill name becomes the
	// child's role name, which lands in trusted message framing (roleNameRe).
	if strings.ContainsAny(args.Name, "/\\") || strings.Contains(args.Name, "..") ||
		!roleNameRe.MatchString(args.Name) {
		return nil, false
	}
	path, err := l.Exec.WS.Resolve(filepath.Join(".claude", "skills", args.Name, "SKILL.md"))
	if err != nil {
		return nil, false
	}
	rawSkill, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	fm, body, ok := parseSkillFile(string(rawSkill))
	if !ok || fm.Context != "fork" {
		return nil, false
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, false // frontmatter-only skill: nothing to run forked
	}
	desc := strings.TrimSpace(fm.Description)
	if desc == "" {
		desc = "skill " + args.Name
	}
	task := strings.TrimSpace(args.Task)
	if task == "" {
		task = "Execute this skill's instructions now."
	}
	// AllowedTools ride into the role verbatim; dynamicRoleSpec enforces
	// unknown-tool and ⊆-parent at resolve time, so a skill author's mistake
	// surfaces as the spawn's model-visible problem.
	expanded, err := json.Marshal(struct {
		Role InlineRole `json:"role"`
		Task string     `json:"task"`
	}{
		Role: InlineRole{Name: args.Name, Description: desc, Instructions: body, Tools: fm.AllowedTools},
		Task: task,
	})
	if err != nil {
		return nil, false
	}
	return expanded, true
}

// parseSkillFile splits a SKILL.md into parsed frontmatter and body. ok=false
// when there is no well-formed frontmatter block (fork requires one — a bare
// skill can only run inline).
func parseSkillFile(s string) (fm skillFrontmatter, body string, ok bool) {
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return fm, "", false
	}
	rest := s[strings.Index(s, "\n")+1:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return fm, "", false // unterminated frontmatter
	}
	block := rest[:end]
	if yaml.Unmarshal([]byte(block), &fm) != nil {
		return skillFrontmatter{}, "", false
	}
	body = rest[end+len("\n---"):]
	if nl := strings.IndexByte(body, '\n'); nl >= 0 {
		body = body[nl+1:]
	} else {
		body = ""
	}
	return fm, body, true
}
