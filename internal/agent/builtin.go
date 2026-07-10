package agent

import (
	"embed"

	"gopkg.in/yaml.v3"
)

// Built-in read-only sub-agents (INC-25, #78): shipped with the binary so a
// workspace can spawn `explore` / `plan` by name without authoring a spec
// file. They carry only read tools — no edit/write/execute — so they are
// side-effect-free. The `agents:` whitelist still governs (a spec must list
// the name to spawn it); these are just an additional resolution source.
//
//go:embed builtin/*.yaml
var builtinFS embed.FS

// builtinNames is the fixed set of built-in agent names, for the directory
// listing and quick membership checks.
var builtinNames = []string{"explore", "plan"}

// IsBuiltinAgent reports whether name is a shipped built-in agent.
func IsBuiltinAgent(name string) bool {
	for _, n := range builtinNames {
		if n == name {
			return true
		}
	}
	return false
}

// BuiltinSpec loads a shipped built-in agent spec by name. The returned spec
// is a fresh copy each call (the caller may set Model to inherit the parent's).
// ok is false for unknown names.
func BuiltinSpec(name string) (*AgentSpec, bool) {
	if !IsBuiltinAgent(name) {
		return nil, false
	}
	raw, err := builtinFS.ReadFile("builtin/" + name + ".yaml")
	if err != nil {
		return nil, false
	}
	var spec AgentSpec
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		return nil, false
	}
	// Apply the same defaults LoadSpec would (the embed skips that path).
	if spec.MaxGenerationSteps == 0 {
		spec.MaxGenerationSteps = DefaultMaxGenerationSteps
	}
	if spec.Model.MaxTokens == 0 {
		spec.Model.MaxTokens = DefaultMaxTokens
	}
	if spec.AgentWorkspace == "" {
		spec.AgentWorkspace = "isolated"
	}
	return &spec, true
}
