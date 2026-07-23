// Package agentcatalog resolves UI-independent Agent definitions from either
// the user config layer or the shipped embedded catalog.
package agentcatalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/runtime"
)

type Entry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"` // user | shipped
	YAML        string `json:"yaml"`
}

// Resolve accepts an effective catalog name or an explicit YAML path. Existing
// paths always mean paths; bare names check user overrides before shipped data.
func Resolve(ref string) (*agent.AgentSpec, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, "", fmt.Errorf("agent is required")
	}
	if looksLikePath(ref) {
		abs, err := filepath.Abs(ref)
		if err != nil {
			return nil, "", err
		}
		spec, err := agent.LoadSpec(abs)
		if err != nil {
			return nil, "", err
		}
		return spec, abs, nil
	}

	userDir, err := runtime.UserAgentsDir()
	if err != nil {
		return nil, "", err
	}
	userPath := filepath.Join(userDir, ref+".yaml")
	if st, err := os.Stat(userPath); err == nil && !st.IsDir() {
		spec, err := agent.LoadSpec(userPath)
		if err != nil {
			return nil, "", fmt.Errorf("user Agent %q: %w", ref, err)
		}
		if spec.Name != ref {
			return nil, "", fmt.Errorf("user Agent %q: field name is %q; filename and name must match", ref, spec.Name)
		}
		return spec, userPath, nil
	}
	if spec, ok := agent.BuiltinSpec(ref); ok {
		return spec, "builtin:" + ref, nil
	}
	return nil, "", fmt.Errorf("unknown Agent %q (run `agentrunner agents` to list available names, or pass a YAML path)", ref)
}

// ResolveLegacySibling is only for a frozen session being resumed. It accepts
// an old sibling model block but discards it; the caller binds the parent's
// already-frozen model.
func ResolveLegacySibling(parentRef, name string) (*agent.AgentSpec, string, error) {
	if parentRef != "" && !strings.HasPrefix(parentRef, "builtin:") && looksLikePath(parentRef) {
		sibling := filepath.Join(filepath.Dir(parentRef), name+".yaml")
		if st, err := os.Stat(sibling); err == nil && !st.IsDir() {
			spec, err := agent.LoadLegacySpec(sibling)
			return spec, sibling, err
		}
	}
	return Resolve(name)
}

func List() ([]Entry, error) {
	entries := make(map[string]Entry)
	order := agent.BuiltinNames()
	for _, name := range order {
		raw, ok := agent.BuiltinYAML(name)
		if !ok {
			return nil, fmt.Errorf("shipped Agent %q is unreadable", name)
		}
		spec, ok := agent.BuiltinSpec(name)
		if !ok {
			return nil, fmt.Errorf("shipped Agent %q is invalid", name)
		}
		entries[name] = Entry{Name: name, Description: spec.Description, Source: "shipped", YAML: string(raw)}
	}

	userDir, err := runtime.UserAgentsDir()
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(userDir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var userNames []string
	for _, path := range files {
		spec, err := agent.LoadSpec(path)
		if err != nil {
			return nil, fmt.Errorf("user Agent %s: %w", path, err)
		}
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if spec.Name != base {
			return nil, fmt.Errorf("user Agent %s: field name is %q; filename and name must match", path, spec.Name)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if _, shipped := entries[spec.Name]; !shipped {
			userNames = append(userNames, spec.Name)
		}
		entries[spec.Name] = Entry{Name: spec.Name, Description: spec.Description, Source: "user", YAML: string(raw)}
	}
	sort.Strings(userNames)
	order = append(order, userNames...)
	out := make([]Entry, 0, len(order))
	seen := make(map[string]bool)
	for _, name := range order {
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, entries[name])
	}
	return out, nil
}

func looksLikePath(ref string) bool {
	if strings.HasPrefix(ref, ".") || filepath.IsAbs(ref) || strings.ContainsAny(ref, `/\`) {
		return true
	}
	return strings.HasSuffix(strings.ToLower(ref), ".yaml") || strings.HasSuffix(strings.ToLower(ref), ".yml")
}
