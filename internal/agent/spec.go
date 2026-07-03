// Package agent holds the agent spec model and (later stages) the agent loop.
package agent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/tool"
)

// DefaultMaxTurns applies when a spec omits max_turns (S1 defaults pack).
const DefaultMaxTurns = 40

// DefaultMaxTokens applies when a spec omits model.max_tokens.
const DefaultMaxTokens = 8192

// ModelSpec selects the provider and model for an agent.
type ModelSpec struct {
	Provider  string `yaml:"provider"`
	ID        string `yaml:"id"`
	MaxTokens int    `yaml:"max_tokens"`
}

// AgentSpec is the declarative agent definition (S1 minimal shape).
// After LoadSpec returns, SystemPrompt always holds the final prompt text —
// system_prompt_file is resolved at load time.
type AgentSpec struct {
	Name             string    `yaml:"name"`
	Model            ModelSpec `yaml:"model"`
	SystemPrompt     string    `yaml:"system_prompt"`
	SystemPromptFile string    `yaml:"system_prompt_file"`
	Tools            []string  `yaml:"tools"`
	MaxTurns         int       `yaml:"max_turns"`
	// Permissions is the spec-level rule source (3.4): lowest precedence
	// in the user > project > spec merge.
	Permissions []pipeline.PermissionRule `yaml:"permissions,omitempty"`
	// Mode is the starting run mode (3.6); CLI --mode overrides. Empty =
	// "default".
	Mode string `yaml:"mode,omitempty"`
}

// LoadSpec reads, parses, validates, and resolves an agent spec.
// Error format (S1 defaults pack): "spec <path>: field <name>: <problem>".
func LoadSpec(path string) (*AgentSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("spec %s: %w", path, err)
	}

	var spec AgentSpec
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("spec %s: %v", path, err)
	}

	if err := spec.validate(path); err != nil {
		return nil, err
	}

	if spec.SystemPromptFile != "" {
		promptPath := spec.SystemPromptFile
		if !filepath.IsAbs(promptPath) {
			promptPath = filepath.Join(filepath.Dir(path), promptPath)
		}
		content, err := os.ReadFile(promptPath)
		if err != nil {
			return nil, fmt.Errorf("spec %s: field system_prompt_file: %v", path, err)
		}
		spec.SystemPrompt = string(content)
		spec.SystemPromptFile = ""
	}

	if spec.MaxTurns == 0 {
		spec.MaxTurns = DefaultMaxTurns
	}
	if spec.Model.MaxTokens == 0 {
		spec.Model.MaxTokens = DefaultMaxTokens
	}
	return &spec, nil
}

func (s *AgentSpec) validate(path string) error {
	fail := func(field, problem string) error {
		return fmt.Errorf("spec %s: field %s: %s", path, field, problem)
	}

	if s.Name == "" {
		return fail("name", "required")
	}
	if s.Model.Provider == "" {
		return fail("model.provider", "required")
	}
	if s.Model.ID == "" {
		return fail("model.id", "required")
	}
	if s.Model.MaxTokens < 0 {
		return fail("model.max_tokens", "must be positive")
	}

	hasInline := s.SystemPrompt != ""
	hasFile := s.SystemPromptFile != ""
	if hasInline && hasFile {
		return fail("system_prompt", "exactly one of system_prompt and system_prompt_file must be set, got both")
	}
	if !hasInline && !hasFile {
		return fail("system_prompt", "exactly one of system_prompt and system_prompt_file must be set, got neither")
	}

	for _, name := range s.Tools {
		if _, ok := tool.Get(name); !ok {
			return fail("tools", fmt.Sprintf("unknown tool %q (known: %v)", name, tool.Names()))
		}
	}

	if s.Mode != "" && !pipeline.ValidMode(s.Mode) {
		return fail("mode", fmt.Sprintf("unknown mode %q (known: default, plan, acceptEdits, bypass)", s.Mode))
	}

	if s.MaxTurns < 0 {
		return fail("max_turns", "must be positive")
	}
	return nil
}
