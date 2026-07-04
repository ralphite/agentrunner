// Package agent holds the agent spec model and (later stages) the agent loop.
package agent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	// CompactAtTokens triggers context compaction (S4.5) when the assembled
	// transcript's estimated size exceeds it — a v0 absolute threshold
	// standing in for DESIGN's trigger_ratio × context_window (which needs a
	// per-model window not yet modeled). Zero disables compaction.
	CompactAtTokens int `yaml:"compact_at_tokens,omitempty"`
	// Thinking requests extended thinking (S4.7); providers map it or
	// downgrade explicitly when their Capabilities.Thinking is false.
	Thinking ThinkingSpec `yaml:"thinking,omitempty"`
}

// ThinkingSpec is the spec-level extended-thinking request (S4.7).
type ThinkingSpec struct {
	Enabled      bool `yaml:"enabled,omitempty"`
	BudgetTokens int  `yaml:"budget_tokens,omitempty"`
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
	// Budget caps the run (3.7); zero values mean unlimited.
	Budget BudgetSpec `yaml:"budget,omitempty"`
	// AllowedTools narrows the MCP tool face (S5.1) to these fully-qualified
	// names (mcp__<server>__<tool>). Empty = every discovered tool. Built-in
	// tools are unaffected — they are selected by Tools.
	AllowedTools []string `yaml:"allowed_tools,omitempty"`
	// Description is what a PARENT's agents directory shows for this spec
	// when it appears as a spawnable sub-agent (S5.3).
	Description string `yaml:"description,omitempty"`
	// Agents whitelists the sub-agent specs this agent may spawn (S5.3).
	// The model only sees — and can only spawn — what is listed here.
	Agents []string `yaml:"agents,omitempty"`
	// OnRunEnd says what happens to still-running background tasks at a
	// run ending (S6.1): "cancel" (default) or "await".
	OnRunEnd string `yaml:"on_run_end,omitempty"`
	// AwaitTimeout bounds the epilogue's await quiesce (S7 还债, DESIGN:
	// await 必有 durable timer 兜底) — a Go duration; empty = 30m. When the
	// timer fires the remaining tasks are cancelled and the run ends.
	AwaitTimeout string `yaml:"await_timeout,omitempty"`
	// Outputs is the deliverable contract (S5.6): at a graceful ending the
	// epilogue auto-publishes each declared output (from its workspace Path
	// unless the run already published the stream) and a missing Required
	// one downgrades the ending to contract_violation.
	Outputs []OutputSpec `yaml:"outputs,omitempty"`
	// Sandbox is the OS containment spec (S7 模块 5). Network "none" removes
	// bash egress via a network namespace — a RATCHET across the shared
	// executor: any spec in the tree demanding none contains the whole
	// tree and a child spec can never widen it back. Empty/"all" = open.
	Sandbox SandboxSpec `yaml:"sandbox,omitempty"`
}

// SandboxSpec declares OS-level containment for executions.
type SandboxSpec struct {
	Network string `yaml:"network,omitempty"` // "" | all | none
}

// OutputSpec is one declared deliverable (DESIGN: name, path, required).
type OutputSpec struct {
	Name     string `yaml:"name"`
	Path     string `yaml:"path,omitempty"`
	Required bool   `yaml:"required,omitempty"`
}

// BudgetSpec is the spec-level resource cap.
type BudgetSpec struct {
	MaxTotalTokens int `yaml:"max_total_tokens,omitempty"`
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

	if s.Budget.MaxTotalTokens < 0 {
		return fail("budget.max_total_tokens", "must be non-negative")
	}
	if s.Mode != "" && !pipeline.ValidMode(s.Mode) {
		return fail("mode", fmt.Sprintf("unknown mode %q (known: default, plan, acceptEdits, bypass)", s.Mode))
	}
	if s.Mode == pipeline.ModeBypass {
		// bypass disables permission/budget; it may only be chosen at the
		// workstation via --mode, never from a spec shipped in a repo.
		return fail("mode", "bypass cannot be set from a spec; use --mode bypass on the command line")
	}

	if s.MaxTurns < 0 {
		return fail("max_turns", "must be positive")
	}
	switch s.OnRunEnd {
	case "", "cancel", "await":
	default:
		return fail("on_run_end", fmt.Sprintf("unknown value %q (known: cancel, await)", s.OnRunEnd))
	}
	if s.AwaitTimeout != "" {
		if d, err := time.ParseDuration(s.AwaitTimeout); err != nil || d <= 0 {
			return fail("await_timeout", fmt.Sprintf("not a positive Go duration: %q", s.AwaitTimeout))
		}
	}
	switch s.Sandbox.Network {
	case "", "all", "none":
	default:
		return fail("sandbox.network", fmt.Sprintf("unknown value %q (known: all, none)", s.Sandbox.Network))
	}
	return nil
}

// defaultAwaitTimeout bounds the await quiesce when the spec is silent.
const defaultAwaitTimeout = 30 * time.Minute

// awaitTimeout is the effective quiesce bound (validated at load).
func (s *AgentSpec) awaitTimeout() time.Duration {
	if s == nil || s.AwaitTimeout == "" {
		return defaultAwaitTimeout
	}
	d, err := time.ParseDuration(s.AwaitTimeout)
	if err != nil || d <= 0 {
		return defaultAwaitTimeout
	}
	return d
}
