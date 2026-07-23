// Package modelconfig owns session-level model selection. Agent YAML describes
// behavior; it never chooses a provider/model.
package modelconfig

import (
	"fmt"
	"strings"
)

const (
	EffortLight  = "light"
	EffortMedium = "medium"
	EffortHigh   = "high"
	EffortXHigh  = "xhigh"

	DefaultProvider = "gemini"
	DefaultModelID  = "gemini-flash-latest"
	DefaultEffort   = EffortMedium

	answerRoom = 4096
)

// Thinking is the effective provider request frozen into a session.
type Thinking struct {
	Enabled      bool `yaml:"enabled,omitempty"`
	BudgetTokens int  `yaml:"budget_tokens,omitempty"`
}

// Spec is the effective model configuration frozen into AgentSpec. It is not
// decoded from Agent YAML.
type Spec struct {
	Provider  string `yaml:"provider"`
	ID        string `yaml:"id"`
	MaxTokens int    `yaml:"max_tokens"`
	// CompactAtTokens and MicrocompactAtTokens are copied from Agent context
	// policy when the session binds this model selection.
	CompactAtTokens      int      `yaml:"compact_at_tokens,omitempty"`
	MicrocompactAtTokens int      `yaml:"microcompact_at_tokens,omitempty"`
	Thinking             Thinking `yaml:"thinking,omitempty"`
}

// Selection is the user-facing model choice accepted from CLI/API and
// settings.yaml's default_model.
type Selection struct {
	Provider string `yaml:"provider" json:"provider"`
	ID       string `yaml:"id" json:"id"`
	Effort   string `yaml:"effort" json:"effort"`
}

func Default() Selection {
	return Selection{Provider: DefaultProvider, ID: DefaultModelID, Effort: DefaultEffort}
}

// ParseRef parses the explicit CLI spelling "<provider>/<id>".
func ParseRef(ref, effort string) (Selection, error) {
	provider, id, ok := strings.Cut(strings.TrimSpace(ref), "/")
	if !ok || strings.TrimSpace(provider) == "" || strings.TrimSpace(id) == "" {
		return Selection{}, fmt.Errorf("model must be <provider>/<id>, for example gemini/gemini-flash-latest")
	}
	s := Selection{Provider: strings.TrimSpace(provider), ID: strings.TrimSpace(id), Effort: effort}
	if s.Effort == "" {
		s.Effort = DefaultEffort
	}
	if err := s.Validate(); err != nil {
		return Selection{}, err
	}
	return s, nil
}

// WithExplicit overlays CLI/API input on a configured default. A supplied
// model ref is one atomic provider/id choice; effort can be overridden alone.
func WithExplicit(base Selection, modelRef, effort string) (Selection, error) {
	if base.Provider == "" && base.ID == "" && base.Effort == "" {
		base = Default()
	}
	if modelRef != "" {
		explicit, err := ParseRef(modelRef, base.Effort)
		if err != nil {
			return Selection{}, err
		}
		base.Provider, base.ID = explicit.Provider, explicit.ID
	}
	if effort != "" {
		base.Effort = effort
	}
	if base.Effort == "" {
		base.Effort = DefaultEffort
	}
	if err := base.Validate(); err != nil {
		return Selection{}, err
	}
	return base, nil
}

func (s Selection) Validate() error {
	if strings.TrimSpace(s.Provider) == "" {
		return fmt.Errorf("default_model.provider is required")
	}
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("default_model.id is required")
	}
	switch s.Effort {
	case EffortLight, EffortMedium, EffortHigh, EffortXHigh:
	default:
		return fmt.Errorf("effort %q is invalid (valid: light, medium, high, xhigh)", s.Effort)
	}
	return nil
}

func effortBudget(effort string) int {
	switch effort {
	case EffortLight:
		return 2048
	case EffortHigh:
		return 12288
	case EffortXHigh:
		return 24576
	default:
		return 6144
	}
}

// Resolve derives the only effective max_tokens/thinking shape from effort.
func (s Selection) Resolve() (Spec, error) {
	if err := s.Validate(); err != nil {
		return Spec{}, err
	}
	budget := effortBudget(s.Effort)
	return Spec{
		Provider:  s.Provider,
		ID:        s.ID,
		MaxTokens: answerRoom + budget,
		Thinking:  Thinking{Enabled: true, BudgetTokens: budget},
	}, nil
}

// FromSpec reconstructs the stable user-facing selection from a frozen
// effective model. Unknown historical budgets map to medium for presentation;
// keeping the original Spec is still preferred when no override is requested.
func FromSpec(spec Spec) Selection {
	effort := DefaultEffort
	switch spec.Thinking.BudgetTokens {
	case 2048:
		effort = EffortLight
	case 12288:
		effort = EffortHigh
	case 24576:
		effort = EffortXHigh
	}
	return Selection{Provider: spec.Provider, ID: spec.ID, Effort: effort}
}
