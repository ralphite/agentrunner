package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/agentcatalog"
	"github.com/ralphite/agentrunner/internal/config"
	"github.com/ralphite/agentrunner/internal/modelconfig"
	"github.com/ralphite/agentrunner/internal/runtime"
)

type modelFlagSet struct {
	ref    *string
	effort *string
}

func addModelFlags(fs *flag.FlagSet) modelFlagSet {
	return modelFlagSet{
		ref:    fs.String("model", "", "session model as <provider>/<id> (default: user settings, then gemini/gemini-flash-latest)"),
		effort: fs.String("effort", "", "reasoning effort: light|medium|high|xhigh (default: user settings, then medium)"),
	}
}

func resolveModelInput(modelRef, effort string) (modelconfig.Selection, agent.ModelSpec, error) {
	path, err := runtime.UserConfigPath()
	if err != nil {
		return modelconfig.Selection{}, agent.ModelSpec{}, err
	}
	settings, err := config.LoadFile(path)
	if err != nil {
		return modelconfig.Selection{}, agent.ModelSpec{}, err
	}
	base := settings.DefaultModel
	if base.Provider == "" && base.ID == "" && base.Effort == "" {
		base = modelconfig.Default()
	}
	selection, err := modelconfig.WithExplicit(base, modelRef, effort)
	if err != nil {
		return modelconfig.Selection{}, agent.ModelSpec{}, err
	}
	if !knownProviderName(selection.Provider) {
		return modelconfig.Selection{}, agent.ModelSpec{}, fmt.Errorf(
			"unknown provider %q (available: gemini, anthropic, scripted)", selection.Provider)
	}
	resolved, err := selection.Resolve()
	return selection, resolved, err
}

func resolveModelSelection(selection modelconfig.Selection) (modelconfig.Selection, agent.ModelSpec, error) {
	if selection.Provider == "" && selection.ID == "" && selection.Effort == "" {
		return resolveModelInput("", "")
	}
	if selection.Effort == "" {
		selection.Effort = modelconfig.DefaultEffort
	}
	if err := selection.Validate(); err != nil {
		return modelconfig.Selection{}, agent.ModelSpec{}, err
	}
	if !knownProviderName(selection.Provider) {
		return modelconfig.Selection{}, agent.ModelSpec{}, fmt.Errorf(
			"unknown provider %q (available: gemini, anthropic, scripted)", selection.Provider)
	}
	resolved, err := selection.Resolve()
	return selection, resolved, err
}

func resolveAgent(ref string, model agent.ModelSpec) (*agent.AgentSpec, string, error) {
	spec, source, err := agentcatalog.Resolve(ref)
	if err != nil {
		return nil, "", err
	}
	if err := agent.BindModel(spec, model, source); err != nil {
		return nil, "", err
	}
	return spec, source, nil
}

func siblingSpecResolver(parentRef string, model agent.ModelSpec, allowLegacy bool) agent.SubSpecResolver {
	return func(name string) (*agent.AgentSpec, error) {
		var (
			spec *agent.AgentSpec
			path string
			err  error
		)
		if allowLegacy {
			spec, path, err = agentcatalog.ResolveLegacySibling(parentRef, name)
		} else {
			// Explicit file definitions retain sibling resolution before the
			// effective catalog, matching the original spec-directory contract.
			spec, path, err = agentcatalog.ResolveLegacySibling(parentRef, name)
			if err == nil && path != "" && !strings.HasPrefix(path, "builtin:") {
				// New launches must reject legacy model fields even when the
				// compatibility resolver found a sibling.
				if spec, _, err = agentcatalog.Resolve(path); err != nil {
					return nil, err
				}
			}
		}
		if err != nil {
			return nil, err
		}
		if err := agent.BindModel(spec, model, path); err != nil {
			return nil, err
		}
		return spec, nil
	}
}

func agentsCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agents", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit the effective catalog as JSON")
	if ok, code := parseFlags(fs, args); !ok {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: agentrunner agents [--json]")
		return ExitUsage
	}
	entries, err := agentcatalog.List()
	if err != nil {
		fmt.Fprintf(stderr, "agentrunner: %v\n", err)
		return ExitUsage
	}
	if *jsonOut {
		if err := json.NewEncoder(stdout).Encode(entries); err != nil {
			fmt.Fprintln(stderr, err)
			return ExitRun
		}
		return ExitOK
	}
	for _, entry := range entries {
		fmt.Fprintf(stdout, "%-12s %-7s %s\n", entry.Name, entry.Source, entry.Description)
	}
	return ExitOK
}
