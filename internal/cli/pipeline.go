package cli

import (
	"fmt"
	"io"

	"github.com/ralphite/agentrunner/internal/config"
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// buildPipeline assembles the effect pipeline — pre-hooks → permission →
// budget — from the merged three-source configuration (3.4), the run mode
// (3.6), and the budget (3.7). It also returns the hook runner for the
// loop's post-tool hooks.
func buildPipeline(ws *workspace.Workspace, specRules []pipeline.PermissionRule,
	mode string, maxTokens int, stderr io.Writer) (*pipeline.Pipeline, *hook.Runner, error) {

	userPath, err := runtime.UserConfigPath()
	if err != nil {
		return nil, nil, err
	}
	user, err := config.LoadFile(userPath)
	if err != nil {
		return nil, nil, err
	}
	project, err := config.LoadFile(runtime.ProjectConfigPath(ws.Root()))
	if err != nil {
		return nil, nil, err
	}
	dataDir, err := runtime.DataDir()
	if err != nil {
		return nil, nil, err
	}
	trusted, err := config.IsTrusted(dataDir, ws.Root())
	if err != nil {
		return nil, nil, err
	}
	merged := config.Merge(user, project, specRules, trusted)
	if len(project.Permissions)+len(project.Hooks.PreTool)+len(project.Hooks.PostTool) > 0 && !trusted {
		fmt.Fprintf(stderr, "note: project settings present but workspace is untrusted — hooks ignored, allows tightened (agentrunner trust %s)\n", ws.Root())
	}

	runner := &hook.Runner{
		PreTool:  merged.Hooks.PreTool,
		PostTool: merged.Hooks.PostTool,
		Dir:      ws.Root(),
	}
	return &pipeline.Pipeline{Gates: []pipeline.Gate{
		// FloorGate runs FIRST so hard denials (workspace escape, plan-mode
		// edit/execute) short-circuit BEFORE any side-effecting pre-hook.
		// SpawnGate (S5.3 tree caps) is equally pure and cheap, so it also
		// runs before the hooks.
		&pipeline.FloorGate{Mode: mode, WS: ws},
		&pipeline.SpawnGate{},
		&hook.Gate{Runner: runner, Notes: func(n string) {
			fmt.Fprintf(stderr, "hook: %s\n", n)
		}},
		&pipeline.PermissionGate{Rules: merged.Permissions, Mode: mode, WS: ws},
		&pipeline.BudgetGate{MaxTotalTokens: maxTokens},
	}}, runner, nil
}
