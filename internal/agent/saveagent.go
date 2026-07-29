package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/tool"
)

// save_agent (user-catalog authoring): the one sanctioned channel for writing
// an agent definition OUTSIDE the workspace. File tools stay workspace-bounded
// (DESIGN invariant); this tool has a fixed destination — the user agents dir
// — with only the file NAME under model control, the same shape of exemption
// as publish_artifact writing the internal store. It runs on the loop side
// (like progress_update) because validation needs agent.LoadSpec, which the
// tool package cannot import without a cycle.

// saveAgentNameRe pins names to bare identifiers: they become filenames and
// catalog keys, so no separators, dots, or case games.
var saveAgentNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func runSaveAgentTool(args json.RawMessage) tool.Result {
	errRes := func(format string, a ...any) tool.Result {
		p, _ := json.Marshal(map[string]string{"error": fmt.Sprintf(format, a...)})
		return tool.Result{Payload: p, IsError: true}
	}
	var in struct {
		Name      string `json:"name"`
		YAML      string `json:"yaml"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.Unmarshal(args, &in); err != nil || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.YAML) == "" {
		return errRes(`save_agent: invalid args: need {"name", "yaml"}`)
	}
	name := strings.TrimSpace(in.Name)
	if !saveAgentNameRe.MatchString(name) {
		return errRes("save_agent: invalid name %q (use lowercase letters, digits, - and _; it becomes the filename %s.yaml)", name, name)
	}
	dir, err := runtime.UserAgentsDir()
	if err != nil {
		return errRes("save_agent: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errRes("save_agent: %v", err)
	}
	target := filepath.Join(dir, name+".yaml")
	if _, err := os.Stat(target); err == nil && !in.Overwrite {
		return errRes("save_agent: agent %q already exists at %s — pass overwrite:true to replace it, or pick another name", name, target)
	}

	// Validate BEFORE landing: write to a temp sibling (no .yaml suffix, so a
	// concurrent catalog List never sees it) and run the full spec loader on
	// it. Sibling sub-agent references resolve against this directory — the
	// same directory the real file will live in.
	tmp, err := os.CreateTemp(dir, ".tmp-"+name+"-*")
	if err != nil {
		return errRes("save_agent: %v", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.WriteString(in.YAML); err != nil {
		_ = tmp.Close()
		return errRes("save_agent: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return errRes("save_agent: %v", err)
	}
	spec, err := LoadSpec(tmpPath)
	if err != nil {
		// The loader's message names the temp path; point at the would-be
		// file instead so the model's fix loop reads naturally.
		return errRes("save_agent: %s", strings.ReplaceAll(err.Error(), tmpPath, name+".yaml"))
	}
	if spec.Name != name {
		return errRes("save_agent: spec name %q must equal the agent name %q (the user catalog requires filename and name to match)", spec.Name, name)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return errRes("save_agent: %v", err)
	}
	payload, _ := json.Marshal(map[string]any{
		"path":  target,
		"agent": name,
		"note":  fmt.Sprintf("saved — immediately usable: webui New session agent picker, or `agentrunner new %s \"...\"`", name),
	})
	return tool.Result{Payload: payload}
}
