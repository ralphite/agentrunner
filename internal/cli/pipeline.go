package cli

import (
	"fmt"
	"io"

	"github.com/ralphite/agentrunner/internal/config"
	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/workspace"
	"github.com/ralphite/agentrunner/internal/wsprobe"
)

// buildPipeline assembles the effect pipeline — pre-hooks → permission →
// budget — from the merged three-source configuration (3.4), the run mode
// (3.6), and the budget (3.7). It also returns the hook runner for the
// loop's post-tool hooks and the merged config itself, for the knobs the
// Loop consumes directly (foreground window).
func buildPipeline(ws *workspace.Workspace, specRules []pipeline.PermissionRule,
	mode string, maxTokens int, stderr io.Writer) (*pipeline.Pipeline, *hook.Runner, config.Merged, error) {

	userPath, err := runtime.UserConfigPath()
	if err != nil {
		return nil, nil, config.Merged{}, err
	}
	user, err := config.LoadFile(userPath)
	if err != nil {
		return nil, nil, config.Merged{}, err
	}
	project, err := config.LoadProjectFile(runtime.ProjectConfigPath(ws.Root()))
	if err != nil {
		return nil, nil, config.Merged{}, err
	}
	dataDir, err := runtime.DataDir()
	if err != nil {
		return nil, nil, config.Merged{}, err
	}
	trusted, err := config.IsTrusted(dataDir, ws.Root())
	if err != nil {
		return nil, nil, config.Merged{}, err
	}
	merged := config.Merge(user, project, specRules, trusted)
	stampWorkspaceScale(ws, merged, stderr)
	if len(project.Permissions)+len(project.Hooks.PreTool)+len(project.Hooks.PostTool) > 0 && !trusted {
		fmt.Fprintf(stderr, "note: project settings present but workspace is untrusted — hooks ignored, allows tightened (agentrunner trust %s)\n", ws.Root())
	}

	runner := &hook.Runner{
		PreTool:   merged.Hooks.PreTool,
		PostTool:  merged.Hooks.PostTool,
		Lifecycle: merged.Hooks.Lifecycle,
		Dir:       ws.Root(),
	}
	return assemblePipeline(ws, [][]pipeline.PermissionRule{merged.Permissions},
		runner, mode, maxTokens, stderr), runner, merged, nil
}

// buildPipelineFromLayers rebuilds a resumed session's pipeline from the
// permission layers journaled in its SessionStarted (S6, S5 回访: 权限交集物化
// 为数据). The layers — one gate each, chained — are the run's FROZEN
// effective rules: a child session resumed standalone keeps its parent's
// bounds, and live config drift does not silently rewrite a run's
// permissions mid-flight. Hooks still come from live config (they are code,
// not materializable data).
func buildPipelineFromLayers(ws *workspace.Workspace, layers [][]pipeline.PermissionRule,
	mode string, maxTokens int, stderr io.Writer) (*pipeline.Pipeline, *hook.Runner, config.Merged, error) {

	userPath, err := runtime.UserConfigPath()
	if err != nil {
		return nil, nil, config.Merged{}, err
	}
	user, err := config.LoadFile(userPath)
	if err != nil {
		return nil, nil, config.Merged{}, err
	}
	project, err := config.LoadProjectFile(runtime.ProjectConfigPath(ws.Root()))
	if err != nil {
		return nil, nil, config.Merged{}, err
	}
	dataDir, err := runtime.DataDir()
	if err != nil {
		return nil, nil, config.Merged{}, err
	}
	trusted, err := config.IsTrusted(dataDir, ws.Root())
	if err != nil {
		return nil, nil, config.Merged{}, err
	}
	merged := config.Merge(user, project, nil, trusted)
	stampWorkspaceScale(ws, merged, stderr)
	runner := &hook.Runner{
		PreTool:   merged.Hooks.PreTool,
		PostTool:  merged.Hooks.PostTool,
		Lifecycle: merged.Hooks.Lifecycle,
		Dir:       ws.Root(),
	}
	return assemblePipeline(ws, layers, runner, mode, maxTokens, stderr), runner, merged, nil
}

// stampWorkspaceScale resolves the large-workspace verdict once per run and
// records it on the Workspace, where all three whole-tree gates read it
// (IndexStore via the tool list, shadow snapshot via snapshotStoreFor, the
// sandbox credential scan). This is the ONLY place the probe runs: every run
// path reaches exactly one of the two pipeline builders, so the bounded walk
// is paid once and no gate has to re-measure.
//
// Degradation is announced, never silent — a run that quietly stopped
// snapshotting would look like a harness that lost rewind for no reason.
func stampWorkspaceScale(ws *workspace.Workspace, merged config.Merged, stderr io.Writer) {
	v := wsprobe.Resolve(ws.Root(), merged.LargeWorkspaceThreshold, merged.LargeWorkspaceMode)
	ws.SetScale(v.Files, v.Large)
	if v.Large {
		fmt.Fprintf(stderr, "note: large workspace (%s) — indexed search off (use grep/glob), "+
			"snapshots off (fork/rewind unavailable). Override with large_workspace.mode in settings.yaml\n", v.Reason)
	}
}

// assemblePipeline lays the fixed gate order — floor → spawn → hooks →
// permission layer(s) → budget — around the given permission layers. Zero
// layers still get ONE empty gate: mode defaults must apply.
func assemblePipeline(ws *workspace.Workspace, layers [][]pipeline.PermissionRule,
	runner *hook.Runner, mode string, maxTokens int, stderr io.Writer) *pipeline.Pipeline {

	gates := []pipeline.Gate{
		// FloorGate runs FIRST so hard denials (workspace escape, plan-mode
		// edit/execute) short-circuit BEFORE any side-effecting pre-hook.
		// SpawnGate (S5.3 tree caps) is equally pure and cheap, so it also
		// runs before the hooks.
		&pipeline.FloorGate{Mode: mode, WS: ws},
		&pipeline.SpawnGate{},
		&hook.Gate{Runner: runner, Notes: func(n string) {
			fmt.Fprintf(stderr, "hook: %s\n", n)
		}},
	}
	if len(layers) == 0 {
		layers = [][]pipeline.PermissionRule{nil}
	}
	for _, rules := range layers {
		gates = append(gates, &pipeline.PermissionGate{Rules: rules, Mode: mode, WS: ws})
	}
	gates = append(gates, &pipeline.BudgetGate{MaxTotalTokens: maxTokens})
	return &pipeline.Pipeline{Gates: gates}
}
