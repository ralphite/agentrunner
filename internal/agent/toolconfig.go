package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ralphite/agentrunner/internal/commandtool"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/tool"
)

// tool_config (user-catalog authoring, the save_agent pattern): the one
// sanctioned channel for managing command-tool manifests OUTSIDE the
// workspace. Fixed destination — the user tools dir — with only the file
// name under model control; validation is commandtool.Validate, the same
// rule discovery applies, so nothing can land that discovery would reject.
// Loop-side because the write must consult the built-in tool registry
// (reserved names) exactly like session-start discovery does.

func runToolConfigTool(args json.RawMessage) tool.Result {
	errRes := func(format string, a ...any) tool.Result {
		p, _ := json.Marshal(map[string]string{"error": fmt.Sprintf(format, a...)})
		return tool.Result{Payload: p, IsError: true}
	}
	var in struct {
		Action    string `json:"action"`
		Name      string `json:"name"`
		Manifest  string `json:"manifest"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return errRes(`tool_config: invalid args: need {"action": "save"|"remove"|"list"}`)
	}
	dir, err := runtime.UserToolsDir()
	if err != nil {
		return errRes("tool_config: %v", err)
	}

	switch in.Action {
	case "save":
		name := strings.TrimSpace(in.Name)
		if name == "" || strings.TrimSpace(in.Manifest) == "" {
			return errRes(`tool_config save: need {"name", "manifest"}`)
		}
		if !saveAgentNameRe.MatchString(name) {
			return errRes("tool_config: invalid name %q (use lowercase letters, digits, - and _; it becomes %s.json)", name, name)
		}
		reserved := make(map[string]bool)
		for _, n := range tool.Names() {
			reserved[n] = true
		}
		t, err := commandtool.Validate([]byte(in.Manifest), reserved)
		if err != nil {
			return errRes("tool_config: manifest rejected: %v", err)
		}
		if t.Name != name {
			return errRes("tool_config: manifest name %q must equal the name argument %q (filename and name must match)", t.Name, name)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return errRes("tool_config: %v", err)
		}
		target := filepath.Join(dir, name+".json")
		if _, err := os.Stat(target); err == nil && !in.Overwrite {
			return errRes("tool_config: tool %q already exists at %s — pass overwrite:true to replace it", name, target)
		}
		tmp, err := os.CreateTemp(dir, ".tmp-"+name+"-*")
		if err != nil {
			return errRes("tool_config: %v", err)
		}
		tmpPath := tmp.Name()
		defer func() { _ = os.Remove(tmpPath) }()
		if _, err := tmp.WriteString(in.Manifest); err != nil {
			_ = tmp.Close()
			return errRes("tool_config: %v", err)
		}
		if err := tmp.Close(); err != nil {
			return errRes("tool_config: %v", err)
		}
		if err := os.Rename(tmpPath, target); err != nil {
			return errRes("tool_config: %v", err)
		}
		payload, _ := json.Marshal(map[string]any{
			"path": target,
			"tool": name,
			"note": "saved — available to NEW sessions (this session's tool set was frozen at start)",
		})
		return tool.Result{Payload: payload}

	case "remove":
		name := strings.TrimSpace(in.Name)
		if name == "" || !saveAgentNameRe.MatchString(name) {
			return errRes(`tool_config remove: need a valid {"name"}`)
		}
		target := filepath.Join(dir, name+".json")
		if err := os.Remove(target); err != nil {
			if os.IsNotExist(err) {
				return errRes("tool_config: no user tool named %q (%s does not exist)", name, target)
			}
			return errRes("tool_config: %v", err)
		}
		payload, _ := json.Marshal(map[string]any{
			"removed": name,
			"note":    "gone for NEW sessions; sessions already running keep their frozen tool set",
		})
		return tool.Result{Payload: payload}

	case "list":
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			return errRes("tool_config: %v", err)
		}
		type row struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			Command     string `json:"command"`
			Error       string `json:"error,omitempty"`
		}
		rows := []row{}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			t, verr := commandtool.Validate(raw, nil)
			if verr != nil {
				rows = append(rows, row{Name: strings.TrimSuffix(e.Name(), ".json"), Error: verr.Error()})
				continue
			}
			rows = append(rows, row{Name: t.Name, Description: t.Description, Command: t.Command})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
		payload, _ := json.Marshal(map[string]any{"dir": dir, "tools": rows})
		return tool.Result{Payload: payload}

	default:
		return errRes("tool_config: unknown action %q (save | remove | list)", in.Action)
	}
}
