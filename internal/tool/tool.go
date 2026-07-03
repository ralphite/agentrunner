// Package tool holds the data-driven tool registry (原则 4: tool 定义即数据).
// Built-in definitions ship as embedded JSON files; implementations register
// against them in exec.go (1.6).
package tool

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ralphite/agentrunner/internal/provider"
)

//go:embed defs/*.json
var defsFS embed.FS

// Class categorizes a tool for permission modes and in-doubt policy
// (read/edit/execute now; wait arrives with interactive tools).
type Class string

const (
	ClassRead    Class = "read"
	ClassEdit    Class = "edit"
	ClassExecute Class = "execute"
	ClassWait    Class = "wait"
)

// Def is one tool definition — pure data.
type Def struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Class       Class           `json:"class"`
	InputSchema json.RawMessage `json:"input_schema"`
}

var registry = mustLoad()

func mustLoad() map[string]Def {
	entries, err := defsFS.ReadDir("defs")
	if err != nil {
		panic(fmt.Sprintf("tool defs: %v", err))
	}
	reg := make(map[string]Def, len(entries))
	for _, e := range entries {
		raw, err := defsFS.ReadFile("defs/" + e.Name())
		if err != nil {
			panic(fmt.Sprintf("tool defs: %v", err))
		}
		var def Def
		if err := json.Unmarshal(raw, &def); err != nil {
			panic(fmt.Sprintf("tool def %s: %v", e.Name(), err))
		}
		if def.Name == "" || def.Class == "" || len(def.InputSchema) == 0 {
			panic(fmt.Sprintf("tool def %s: incomplete definition", e.Name()))
		}
		if _, dup := reg[def.Name]; dup {
			panic(fmt.Sprintf("tool def %s: duplicate name %q", e.Name(), def.Name))
		}
		reg[def.Name] = def
	}
	return reg
}

// Get returns a tool definition by name.
func Get(name string) (Def, bool) {
	def, ok := registry[name]
	return def, ok
}

// Names lists all registered tool names, sorted.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ProviderDefs renders the named tools into wire-level provider defs,
// erroring on unknown names (spec validation should have caught them).
func ProviderDefs(names []string) ([]provider.ToolDef, error) {
	defs := make([]provider.ToolDef, 0, len(names))
	for _, name := range names {
		def, ok := registry[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool %q", name)
		}
		defs = append(defs, provider.ToolDef{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: def.InputSchema,
		})
	}
	return defs, nil
}
