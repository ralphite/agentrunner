package agent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ralphite/agentrunner/internal/command"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/skill"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/tool"
)

// Skill-gated tools (progressive disclosure applied to the tool face): a
// skill's frontmatter may declare `tools:` that stay OFF the advertised set
// until the model loads that skill. Loading is a plain journaled skill call
// with a non-error result, so the unlock is derived from the conversation —
// resume and crash replay rebuild the same face, and compaction does not
// disturb it (the fold keeps every message; only the assembled view
// summarizes). Unlock widens for the rest of the session: the model may act
// on a loaded skill's instructions many turns later.

// loadedSkillNames scans the conversation for skill loads by either path:
// a successful skill tool call, or a slash-expanded skill body (its
// "Loaded skill" header line, command.SkillLoadHeader).
func loadedSkillNames(s state.State) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for _, m := range s.Conversation.Messages {
		switch m.Role {
		case provider.RoleUser:
			for _, p := range m.Parts {
				if p.Kind != provider.PartText {
					continue
				}
				if name, ok := command.ExpandedSkillName(p.Text); ok {
					add(name)
				}
			}
		case provider.RoleAssistant:
			for _, c := range toolCallsOf(m) {
				if c.Name != "skill" {
					continue
				}
				res, ok := s.Conversation.ToolResults[c.CallID]
				if !ok || res.IsError {
					continue
				}
				var args struct {
					Name string `json:"name"`
				}
				if err := json.Unmarshal(c.Args, &args); err == nil {
					add(args.Name)
				}
			}
		}
	}
	return out
}

// unlockedSkillTools resolves the tools unlocked by the session's loaded
// skills: each skill's raw SKILL.md is read through the same shadow chain
// as the loader (workspace → spec-bundled → shipped) and its frontmatter
// `tools:` collected. Unknown tool names are dropped — a skill author's typo
// must not wedge the face.
func (l *Loop) unlockedSkillTools(s state.State) []string {
	names := loadedSkillNames(s)
	if len(names) == 0 {
		return nil
	}
	var out []string
	for _, name := range names {
		raw, ok := l.skillRaw(name)
		if !ok {
			continue
		}
		for _, t := range skill.UnlockedTools(raw) {
			if _, known := tool.Get(t); known {
				out = append(out, t)
			}
		}
	}
	return out
}

// effectiveToolDefs is the per-step tool face: the run's base defs plus
// whatever the loaded skills unlock. New unlocks are also admitted to the
// dispatch allowlist gate — same drive goroutine as the gate's readers, so
// the map write is ordered before any dispatch that could see the tool.
func (l *Loop) effectiveToolDefs(s state.State, base []provider.ToolDef) []provider.ToolDef {
	unlocked := l.unlockedSkillTools(s)
	if len(unlocked) == 0 {
		return base
	}
	have := make(map[string]bool, len(base))
	for _, d := range base {
		have[d.Name] = true
	}
	var extraNames []string
	for _, n := range unlocked {
		if !have[n] {
			have[n] = true
			extraNames = append(extraNames, n)
		}
	}
	if len(extraNames) == 0 {
		return base
	}
	defs, err := tool.ProviderDefs(extraNames)
	if err != nil {
		return base
	}
	out := make([]provider.ToolDef, 0, len(base)+len(defs))
	out = append(append(out, base...), defs...)
	if l.advertisedTools != nil {
		for _, n := range extraNames {
			l.advertisedTools[n] = true
		}
	}
	return out
}

// skillRaw loads a skill's raw SKILL.md by the loader's shadow order.
func (l *Loop) skillRaw(name string) ([]byte, bool) {
	if l.Exec != nil && l.Exec.WS != nil {
		if p, err := l.Exec.WS.Resolve(filepath.Join(".claude", "skills", name, "SKILL.md")); err == nil {
			if raw, err := os.ReadFile(p); err == nil {
				return raw, true
			}
		}
		if specPath, ok := l.Exec.SkillPath(name); ok {
			if raw, err := os.ReadFile(specPath); err == nil {
				return raw, true
			}
		}
	}
	return skill.BuiltinRaw(name)
}
