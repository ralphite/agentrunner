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
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/pipeline"
)

// Settings is one source's settings.yaml shape.
type Settings struct {
	Permissions []pipeline.PermissionRule `yaml:"permissions,omitempty"`
	Hooks       Hooks                     `yaml:"hooks,omitempty"`
	// Notify is the notifier channel (S6 模块⑤) — a documented carve-out:
	// ONLY the user-level settings are consulted (a cloned repo must never
	// redirect notifications), so Merge ignores it entirely.
	Notify NotifySpec `yaml:"notify,omitempty"`
}

// NotifySpec configures the notification channel: an argv receiving the
// notification JSON on stdin (ntfy, mail, anything). Empty = stderr only.
type NotifySpec struct {
	Command []string `yaml:"command,omitempty"`
}

// Hooks lists shell commands for the 3.8 executor. Lifecycle (INC-15, G19)
// maps event name → commands; event names are validated against the hook
// package's registry at load time so a typo fails loudly, not silently.
type Hooks struct {
	PreTool   []string            `yaml:"pre_tool,omitempty"`
	PostTool  []string            `yaml:"post_tool,omitempty"`
	Lifecycle map[string][]string `yaml:"lifecycle,omitempty"`
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
	for ev := range s.Hooks.Lifecycle {
		if !hook.LifecycleEvents[ev] {
			return Settings{}, fmt.Errorf("settings %s: hooks.lifecycle: unknown event %q", path, ev)
		}
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
// project, then spec. Trust asymmetry (S3 exit review): the project
// settings.yaml is auto-discovered from the workspace WITHOUT the user
// naming it, so it is silent repo content — its allow rules tighten to ask
// until the workspace is trusted, and its hooks never run untrusted. A
// spec is different: the user explicitly names it on the command line (an
// act of trust, like running a script), so its rules pass through — except
// mode: bypass, which spec validation forbids outright as a gate kill
// switch rather than a permission rule.
func Merge(user, project Settings, specRules []pipeline.PermissionRule, projectTrusted bool) Merged {
	m := Merged{ProjectTrusted: projectTrusted}
	m.Permissions = append(m.Permissions, user.Permissions...)
	for _, r := range project.Permissions {
		if !projectTrusted && r.Action == event.VerdictAllow {
			r.Action = event.VerdictAsk // silent repo content may not grant itself
		}
		m.Permissions = append(m.Permissions, r)
	}
	m.Permissions = append(m.Permissions, specRules...)

	m.Hooks.PreTool = append(m.Hooks.PreTool, user.Hooks.PreTool...)
	m.Hooks.PostTool = append(m.Hooks.PostTool, user.Hooks.PostTool...)
	mergeLifecycle(&m.Hooks, user.Hooks.Lifecycle)
	if projectTrusted {
		m.Hooks.PreTool = append(m.Hooks.PreTool, project.Hooks.PreTool...)
		m.Hooks.PostTool = append(m.Hooks.PostTool, project.Hooks.PostTool...)
		mergeLifecycle(&m.Hooks, project.Hooks.Lifecycle)
	}
	return m
}

// mergeLifecycle appends per-event commands (user first, then trusted
// project — same order discipline as PreTool/PostTool).
func mergeLifecycle(dst *Hooks, src map[string][]string) {
	if len(src) == 0 {
		return
	}
	if dst.Lifecycle == nil {
		dst.Lifecycle = make(map[string][]string, len(src))
	}
	for ev, cmds := range src {
		dst.Lifecycle[ev] = append(dst.Lifecycle[ev], cmds...)
	}
}

// AppendRule adds one permission rule to a settings file (INC-17, G5:
// "allow and don't ask again"). It is idempotent — an identical rule already
// present is a no-op, so a replayed/duplicate approve never double-writes —
// and preserves the file's other settings (it reloads, appends, re-marshals).
// The file is created if absent. Returns whether a rule was actually added.
func AppendRule(path string, rule pipeline.PermissionRule) (bool, error) {
	s, err := LoadFile(path)
	if err != nil {
		return false, err
	}
	for _, existing := range s.Permissions {
		if existing == rule {
			return false, nil // already remembered
		}
	}
	s.Permissions = append(s.Permissions, rule)
	raw, err := yaml.Marshal(s)
	if err != nil {
		return false, fmt.Errorf("settings %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, fmt.Errorf("settings %s: %w", path, err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return false, fmt.Errorf("settings %s: %w", path, err)
	}
	return true, nil
}

// --- trust registry ---

type trustFile struct {
	Trusted []string `yaml:"trusted"`
}

func trustPath(dataDir string) string {
	return filepath.Join(dataDir, "trusted.yaml")
}

// resolveTrustPath canonicalizes a workspace root for the trust registry:
// absolute first, then realpath. EvalSymlinks alone kept relative paths
// relative, so `ar trust .` stored a literal "." that could never match a
// runtime root again (QA Round2 F-E5).
func resolveTrustPath(wsRoot string) (string, error) {
	abs, err := filepath.Abs(wsRoot)
	if err != nil {
		return "", fmt.Errorf("trust: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("trust: %w", err)
	}
	return resolved, nil
}

// IsTrusted reports whether the workspace root (realpath) is registered.
func IsTrusted(dataDir, wsRoot string) (bool, error) {
	resolved, err := resolveTrustPath(wsRoot)
	if err != nil {
		return false, err
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

// Trust registers a workspace root (idempotent) and returns the canonical
// path it stored — callers echo it so the user sees exactly what a later
// session will match. Only directories qualify: a trust entry names a
// workspace, and a file (or typo like "-h") in a machine-level allowlist
// is a footgun (QA Round2 F-E5/F-E14).
func Trust(dataDir, wsRoot string) (string, error) {
	resolved, err := resolveTrustPath(wsRoot)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("trust: %w", err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("trust: %s is not a directory", resolved)
	}
	if ok, err := IsTrusted(dataDir, wsRoot); err != nil {
		return "", err
	} else if ok {
		return resolved, nil
	}
	var tf trustFile
	if raw, err := os.ReadFile(trustPath(dataDir)); err == nil {
		_ = yaml.Unmarshal(raw, &tf)
	}
	tf.Trusted = append(tf.Trusted, resolved)
	raw, err := yaml.Marshal(tf)
	if err != nil {
		return "", fmt.Errorf("trust: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", fmt.Errorf("trust: %w", err)
	}
	if err := os.WriteFile(trustPath(dataDir), raw, 0o600); err != nil {
		return "", fmt.Errorf("trust: %w", err)
	}
	return resolved, nil
}
