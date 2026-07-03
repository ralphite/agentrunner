// Package config merges the three settings sources — user, project, spec —
// and owns the trust registry (3.4). Project-level configuration is code
// from the repo you cloned: its hooks NEVER run untrusted, and its
// permission rules may only tighten (allow downgrades to ask) until the
// workspace is explicitly trusted via `agentrunner trust`.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/pipeline"
)

// Settings is one source's settings.yaml shape.
type Settings struct {
	Permissions []pipeline.PermissionRule `yaml:"permissions,omitempty"`
	Hooks       Hooks                     `yaml:"hooks,omitempty"`
}

// Hooks lists shell commands for the 3.8 executor.
type Hooks struct {
	PreTool  []string `yaml:"pre_tool,omitempty"`
	PostTool []string `yaml:"post_tool,omitempty"`
}

// Merged is the effective configuration for one run.
type Merged struct {
	Permissions []pipeline.PermissionRule // precedence order: user, project, spec
	Hooks       Hooks
	// ProjectTrusted records the trust verdict for observability.
	ProjectTrusted bool
}

// LoadFile reads one settings.yaml strictly (unknown keys are errors).
// A missing file is empty settings, not an error.
func LoadFile(path string) (Settings, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("settings %s: %w", path, err)
	}
	var s Settings
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return Settings{}, fmt.Errorf("settings %s: %w", path, err)
	}
	if err := validateRules(s.Permissions); err != nil {
		return Settings{}, fmt.Errorf("settings %s: %w", path, err)
	}
	return s, nil
}

func validateRules(rules []pipeline.PermissionRule) error {
	for i, r := range rules {
		switch r.Action {
		case event.VerdictAllow, event.VerdictAsk, event.VerdictDeny:
		default:
			return fmt.Errorf("permissions[%d]: invalid action %q", i, r.Action)
		}
	}
	return nil
}

// Merge builds the effective config. Precedence: user rules first, then
// project (tightened when untrusted), then spec. Hooks: user always;
// project only when trusted; spec never carries hooks (it is portable
// content, not workstation policy).
func Merge(user, project Settings, specRules []pipeline.PermissionRule, projectTrusted bool) Merged {
	m := Merged{ProjectTrusted: projectTrusted}
	m.Permissions = append(m.Permissions, user.Permissions...)
	for _, r := range project.Permissions {
		if !projectTrusted && r.Action == event.VerdictAllow {
			// An untrusted repo may not grant itself anything: its allow
			// becomes ask (tighten-only).
			r.Action = event.VerdictAsk
		}
		m.Permissions = append(m.Permissions, r)
	}
	m.Permissions = append(m.Permissions, specRules...)

	m.Hooks.PreTool = append(m.Hooks.PreTool, user.Hooks.PreTool...)
	m.Hooks.PostTool = append(m.Hooks.PostTool, user.Hooks.PostTool...)
	if projectTrusted {
		m.Hooks.PreTool = append(m.Hooks.PreTool, project.Hooks.PreTool...)
		m.Hooks.PostTool = append(m.Hooks.PostTool, project.Hooks.PostTool...)
	}
	return m
}

// --- trust registry ---

type trustFile struct {
	Trusted []string `yaml:"trusted"`
}

func trustPath(dataDir string) string {
	return filepath.Join(dataDir, "trusted.yaml")
}

// IsTrusted reports whether the workspace root (realpath) is registered.
func IsTrusted(dataDir, wsRoot string) (bool, error) {
	resolved, err := filepath.EvalSymlinks(wsRoot)
	if err != nil {
		return false, fmt.Errorf("trust: %w", err)
	}
	raw, err := os.ReadFile(trustPath(dataDir))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("trust: %w", err)
	}
	var tf trustFile
	if err := yaml.Unmarshal(raw, &tf); err != nil {
		return false, fmt.Errorf("trust: %s: %w", trustPath(dataDir), err)
	}
	for _, dir := range tf.Trusted {
		if dir == resolved {
			return true, nil
		}
	}
	return false, nil
}

// Trust registers a workspace root (idempotent).
func Trust(dataDir, wsRoot string) error {
	resolved, err := filepath.EvalSymlinks(wsRoot)
	if err != nil {
		return fmt.Errorf("trust: %w", err)
	}
	if ok, err := IsTrusted(dataDir, wsRoot); err != nil {
		return err
	} else if ok {
		return nil
	}
	var tf trustFile
	if raw, err := os.ReadFile(trustPath(dataDir)); err == nil {
		_ = yaml.Unmarshal(raw, &tf)
	}
	tf.Trusted = append(tf.Trusted, resolved)
	raw, err := yaml.Marshal(tf)
	if err != nil {
		return fmt.Errorf("trust: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("trust: %w", err)
	}
	if err := os.WriteFile(trustPath(dataDir), raw, 0o600); err != nil {
		return fmt.Errorf("trust: %w", err)
	}
	return nil
}
