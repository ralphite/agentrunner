package agent

import (
	"log/slog"

	"github.com/ralphite/agentrunner/internal/skill"
)

// Spec-bundled skills (the spec `skills:` field): entries were materialized
// at load time — shipped names stay names, path entries became absolute
// directories. These helpers turn the entries into the two runtime shapes:
// directory entries for the frozen prompt block, and a name→SKILL.md map for
// the on-demand body loaders (skill tool, fork expansion).

// specSkillEntries loads directory entries for a spec's path-based skills.
// Shipped names are skipped — the builtin layer already lists them. A path
// that went bad since load degrades to a warning, not a broken session.
func specSkillEntries(entries []string) []skill.Skill {
	var out []skill.Skill
	for _, e := range entries {
		if !skillEntryIsPath(e) {
			continue
		}
		s, err := skill.FromDir(e)
		if err != nil {
			slog.Warn("spec skill unavailable; continuing without", "dir", e, "err", err)
			continue
		}
		out = append(out, s)
	}
	return out
}

// specSkillPaths maps a spec's path-based skill names to their SKILL.md
// files, for the readers that load bodies on demand.
func specSkillPaths(entries []string) map[string]string {
	m := make(map[string]string)
	for _, s := range specSkillEntries(entries) {
		m[s.Name] = s.Path
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// applySkills hands the spec's bundled-skill map to the shared executor so
// the skill tool can load their bodies. Run and Resume both pass through
// here, mirroring applySandbox; on a shared executor the first (root) spec
// wins.
func (l *Loop) applySkills() {
	if l.Spec == nil || l.Exec == nil || len(l.Spec.Skills) == 0 {
		return
	}
	l.Exec.SetSkillPaths(specSkillPaths(l.Spec.Skills))
}
