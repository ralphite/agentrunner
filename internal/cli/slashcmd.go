package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ralphite/agentrunner/internal/command"
	"github.com/ralphite/agentrunner/internal/skill"
)

// slashCmd lists the workspace's slash surface — custom commands and skills —
// for pickers (the webui composer's "/" menu is the consumer, thin-shell
// doctrine). Both expand through the same ingest macro (command.Expand):
// /name injects the command body, or the skill body when no command file
// exists.
func slashCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("slash", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceDir := fs.String("workspace", ".", "workspace root to list for")
	jsonOut := fs.Bool("json", false, "emit JSON")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	root, err := filepath.Abs(*workspaceDir)
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}

	type entry struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		Source      string `json:"source,omitempty"`
	}
	var commands []entry
	for _, c := range command.Discover(root) {
		commands = append(commands, entry{Name: c.Name, Description: c.Description})
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })

	skills, derr := skill.DiscoverWith(root, nil)
	if derr != nil {
		fmt.Fprintf(stderr, "agentrunner: warning: %v\n", derr)
	}
	var skillEntries []entry
	for _, s := range skills {
		source := "workspace"
		if strings.HasPrefix(s.Path, "builtin:") {
			source = "shipped"
		}
		skillEntries = append(skillEntries, entry{Name: s.Name, Description: s.Description, Source: source})
	}

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		return exitOn(enc.Encode(map[string]any{
			"commands": commands,
			"skills":   skillEntries,
		}), stderr)
	}
	for _, c := range commands {
		fmt.Fprintf(stdout, "/%s\tcommand\t%s\n", c.Name, c.Description)
	}
	for _, s := range skillEntries {
		fmt.Fprintf(stdout, "/%s\tskill (%s)\t%s\n", s.Name, s.Source, s.Description)
	}
	return ExitOK
}

func exitOn(err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitRun
	}
	return ExitOK
}
