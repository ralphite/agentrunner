// Package commandtool discovers user-defined command tools (INC-55,
// HANDA-PARITY #4): local commands packaged as model-callable tools by a JSON
// manifest (name / description / command / timeout / params schema). The
// model's arguments arrive as JSON on the command's stdin; each invocation
// runs as an execute-class command effect through the full permission
// pipeline and the mandatory OS sandbox (决策 #34) — this package only owns
// DISCOVERY, not execution.
//
// Discovery honors the trust model (决策 #19). A command tool is 可执行配置
// (it runs a command with model-controlled stdin), so it sits on the
// execution side of the trust line, exactly like a hook:
//
//   - USER layer (~/.config/agentrunner/tools/*.json) is the user's own
//     machine — always loaded.
//   - PROJECT layer (<ws>/.claude/tools/*.json) is repo content that travels
//     with a clone — loaded ONLY when the workspace is trusted. Cloning an
//     untrusted repo must not equal handing over arbitrary code execution.
//
// Collision rules: a manifest whose name matches a built-in tool is refused
// (the built-in wins, the manifest is dropped with a warning); a name already
// taken by the user layer shadows the project layer (user precedence); a
// duplicate within one layer keeps the first (filename order) and warns.
package commandtool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// MaxTimeoutS clamps a manifest's declared timeout: a command tool is a
// foreground effect, not a daemon. TimeoutS == 0 means "use the harness
// execute-class default" (owned by the durable-timer substrate, not here).
const MaxTimeoutS = 3600

// nameRe bounds a tool name to the provider function-name shape (Gemini and
// Anthropic both accept ^[A-Za-z0-9_-]{1,64}$) and, being a bare identifier,
// forecloses any path-traversal or namespace games.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Manifest is the on-disk JSON shape of one command tool.
type Manifest struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Command     string          `json:"command"`
	TimeoutS    int             `json:"timeout_s,omitempty"`
	Params      json.RawMessage `json:"params,omitempty"`
}

// Tool is a validated, resolved command tool ready for the harness: the
// manifest normalized, tagged with the layer it came from, with a
// provider-ready input schema.
type Tool struct {
	Name        string
	Description string
	Command     string
	TimeoutS    int
	Source      string // "user" | "project"
	InputSchema json.RawMessage
}

// defaultSchema is the input schema for a manifest that declares no params: a
// parameterless tool still needs a well-formed object schema on the wire.
var defaultSchema = json.RawMessage(`{"type":"object","properties":{}}`)

// parseManifest strictly decodes one manifest (unknown keys are errors, so a
// typo fails loudly rather than being silently ignored).
func parseManifest(raw []byte) (Manifest, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, err
	}
	if dec.More() {
		return Manifest{}, fmt.Errorf("trailing content after JSON object")
	}
	return m, nil
}

// resolve validates a manifest and normalizes it into a Tool. A malformed
// manifest is an error (the caller drops it with a warning, never aborts).
func resolve(m Manifest, source string) (Tool, error) {
	if !nameRe.MatchString(m.Name) {
		return Tool{}, fmt.Errorf("invalid name %q (want %s)", m.Name, nameRe.String())
	}
	// mcp__ is the MCP face's reserved namespace (a command tool could never
	// dispatch there); banning the prefix also keeps names collision-free
	// against the out-of-band MCP tools discovered later.
	if strings.HasPrefix(m.Name, "mcp__") {
		return Tool{}, fmt.Errorf("name %q uses the reserved mcp__ prefix", m.Name)
	}
	if strings.TrimSpace(m.Command) == "" {
		return Tool{}, fmt.Errorf("tool %q: command is required", m.Name)
	}
	if m.TimeoutS < 0 {
		return Tool{}, fmt.Errorf("tool %q: negative timeout_s", m.Name)
	}
	timeout := m.TimeoutS
	if timeout > MaxTimeoutS {
		timeout = MaxTimeoutS
	}
	schema := m.Params
	if len(bytes.TrimSpace(schema)) == 0 {
		schema = defaultSchema
	} else {
		// params must be a JSON object (a tool's arguments are named fields);
		// anything else is a malformed schema.
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(schema, &probe); err != nil {
			return Tool{}, fmt.Errorf("tool %q: params must be a JSON object schema: %w", m.Name, err)
		}
	}
	return Tool{
		Name:        m.Name,
		Description: m.Description,
		Command:     m.Command,
		TimeoutS:    timeout,
		Source:      source,
		InputSchema: schema,
	}, nil
}

// Discover loads the user and (only when trusted) project manifest
// directories, applying the trust gate and collision rules. It never fails:
// unreadable dirs are "no tools", and every malformed/rejected manifest
// becomes a warning string the caller surfaces. Results are sorted by name.
//
// reserved is the set of names a command tool may not take (the built-in tool
// registry); the caller supplies it so this package stays free of a tool
// import.
func Discover(userDir, projectDir string, projectTrusted bool, reserved map[string]bool) ([]Tool, []string) {
	byName := map[string]Tool{}
	var warnings []string

	load := func(dir, source string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return // a missing tools dir is the common case, not an error
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			raw, err := os.ReadFile(path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
				continue
			}
			m, err := parseManifest(raw)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
				continue
			}
			tool, err := resolve(m, source)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
				continue
			}
			if reserved[tool.Name] {
				warnings = append(warnings, fmt.Sprintf("%s: tool %q collides with a built-in tool; not loaded", path, tool.Name))
				continue
			}
			if prev, ok := byName[tool.Name]; ok {
				if prev.Source == source {
					warnings = append(warnings, fmt.Sprintf("%s: duplicate tool name %q in %s layer; keeping the first", path, tool.Name, source))
				} else {
					warnings = append(warnings, fmt.Sprintf("%s: project tool %q is shadowed by a user tool of the same name", path, tool.Name))
				}
				continue
			}
			byName[tool.Name] = tool
		}
	}

	// User first so its names win the precedence tie against the project layer.
	load(userDir, "user")
	if projectTrusted {
		load(projectDir, "project")
	} else if hasManifests(projectDir) {
		warnings = append(warnings, fmt.Sprintf(
			"project command tools present under %s but the workspace is untrusted — ignored (run: agentrunner trust <workspace>)", projectDir))
	}

	tools := make([]Tool, 0, len(byName))
	for _, t := range byName {
		tools = append(tools, t)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, warnings
}

// hasManifests reports whether a directory holds any *.json entry — used only
// to decide whether an untrusted project warrants the trust-gate warning.
func hasManifests(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			return true
		}
	}
	return false
}
