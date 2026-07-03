package cli

import (
	"fmt"
	"io"

	"github.com/ralphite/agentrunner/internal/config"
	"github.com/ralphite/agentrunner/internal/pipeline"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/workspace"
)

// buildPipeline assembles the effect pipeline from the merged three-source
// configuration (3.4) and the run mode (3.6). Hook and budget gates join
// at 3.7/3.8.
func buildPipeline(ws *workspace.Workspace, specRules []pipeline.PermissionRule,
	mode string, stderr io.Writer) (*pipeline.Pipeline, error) {

	userPath, err := runtime.UserConfigPath()
	if err != nil {
		return nil, err
	}
	user, err := config.LoadFile(userPath)
	if err != nil {
		return nil, err
	}
	project, err := config.LoadFile(runtime.ProjectConfigPath(ws.Root()))
	if err != nil {
		return nil, err
	}
	dataDir, err := runtime.DataDir()
	if err != nil {
		return nil, err
	}
	trusted, err := config.IsTrusted(dataDir, ws.Root())
	if err != nil {
		return nil, err
	}
	merged := config.Merge(user, project, specRules, trusted)
	if len(project.Permissions)+len(project.Hooks.PreTool)+len(project.Hooks.PostTool) > 0 && !trusted {
		fmt.Fprintf(stderr, "note: project settings present but workspace is untrusted — hooks ignored, allows tightened (agentrunner trust %s)\n", ws.Root())
	}

	return &pipeline.Pipeline{Gates: []pipeline.Gate{
		&pipeline.PermissionGate{Rules: merged.Permissions, Mode: mode, WS: ws},
	}}, nil
}
