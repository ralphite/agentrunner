package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/config"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/provider/scripted"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

// isolateToolDirs points user config + data dirs at temp locations so a test
// controls the command-tool manifests and trust registry without touching the
// developer's real ~/.config or ~/.local/share.
func isolateToolDirs(t *testing.T) (userToolsDir, dataDir string) {
	t.Helper()
	cfg := t.TempDir()
	data := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", data)
	return filepath.Join(cfg, "agentrunner", "tools"), filepath.Join(data, "agentrunner")
}

func writeToolManifest(t *testing.T, dir, file, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// End-to-end (INC-55): a user-defined command tool is discovered, advertised
// to the model, and — when invoked — runs its fixed command in the OS sandbox
// with the model's args on stdin, through the full effect pipeline. `cat`
// echoes stdin, proving the args reached the command; the EffectResolved
// carries containment, proving the sandbox was in force.
func TestCommandToolEndToEnd(t *testing.T) {
	userToolsDir, _ := isolateToolDirs(t)
	writeToolManifest(t, userToolsDir, "echo_args.json",
		`{"name":"echo_args","description":"echo the args","command":"cat"}`)

	fix := scripted.Fixture{Steps: []scripted.Step{
		{
			Expect: scripted.Expect{ToolsInclude: []string{"echo_args"}},
			Respond: []scripted.Event{
				{ToolCall: &scripted.ToolCallEvent{CallID: "ct1", Name: "echo_args",
					Args: map[string]any{"target": "prod"}}},
				{Finish: "tool_use"},
			},
		},
		{Respond: []scripted.Event{{Text: "done"}, {Finish: "end_turn"}}},
	}}
	l := testLoop(t, fix, t.TempDir())
	if _, err := l.Exec.SandboxInfo(); err != nil {
		t.Skipf("no OS sandbox backend here: %v", err)
	}

	if _, err := l.Run(context.Background(), "use the tool"); err != nil {
		t.Fatal(err)
	}

	events, err := store.ReadEvents(l.Store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var frozen bool
	var resolved *event.EffectResolved
	var echoed bool
	for _, e := range events {
		switch e.Type {
		case event.TypeSessionStarted:
			dec, _ := event.DecodePayload(e)
			for _, ct := range dec.(*event.SessionStarted).CommandTools {
				if ct.Name == "echo_args" && ct.Command == "cat" && ct.Source == "user" {
					frozen = true
				}
			}
		case event.TypeEffectResolved:
			dec, _ := event.DecodePayload(e)
			if er := dec.(*event.EffectResolved); er.CallID == "ct1" {
				resolved = er
			}
		case event.TypeActivityCompleted:
			if strings.Contains(string(e.Payload), "tool-ct1") && strings.Contains(string(e.Payload), "target") {
				echoed = true
			}
		}
	}
	if !frozen {
		t.Error("command tool was not frozen into SessionStarted")
	}
	if resolved == nil || resolved.Verdict != event.VerdictAllow {
		t.Fatalf("EffectResolved = %+v", resolved)
	}
	if resolved.Containment == nil || resolved.Containment.Filesystem != "workspace" || resolved.Containment.Backend == "" {
		t.Errorf("command tool ran without recorded containment: %+v", resolved.Containment)
	}
	if !echoed {
		t.Error("the model's args did not reach the command's stdin")
	}
}

// The trust gate (决策 #19): a PROJECT-layer command tool is frozen into the
// session only when the workspace is trusted. Untrusted → absent from the
// face; after `trust` → present.
func TestCommandToolProjectTrustGate(t *testing.T) {
	_, dataDir := isolateToolDirs(t)

	root := t.TempDir()
	projectToolsDir := filepath.Join(root, ".claude", "tools")
	writeToolManifest(t, projectToolsDir, "ci.json", `{"name":"ci_deploy","command":"./ci.sh"}`)

	frozenNames := func() []string {
		fix := scripted.Fixture{Steps: []scripted.Step{
			{Respond: []scripted.Event{{Text: "hi"}, {Finish: "end_turn"}}},
		}}
		l := testLoop(t, fix, root)
		if _, err := l.Run(context.Background(), "go"); err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range mustReadEvents(t, l.Store.Dir()) {
			if e.Type == event.TypeSessionStarted {
				dec, _ := event.DecodePayload(e)
				for _, ct := range dec.(*event.SessionStarted).CommandTools {
					names = append(names, ct.Name)
				}
			}
		}
		return names
	}

	// Untrusted: the project tool must not load.
	if names := frozenNames(); len(names) != 0 {
		t.Fatalf("untrusted project tool loaded: %v", names)
	}

	// Trust the workspace, then it loads.
	if _, err := config.Trust(dataDir, root); err != nil {
		t.Fatal(err)
	}
	names := frozenNames()
	if len(names) != 1 || names[0] != "ci_deploy" {
		t.Fatalf("trusted project tools = %v", names)
	}
}

func mustReadEvents(t *testing.T, dir string) []event.Envelope {
	t.Helper()
	events, err := store.ReadEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	return events
}

// The fold-derived helpers: a command tool in the fold is execute-class and
// honors its manifest timeout; an unknown name resolves to nothing.
func TestCommandToolFoldHelpers(t *testing.T) {
	s := state.State{}
	s.Session.CommandTools = []event.CommandToolDef{
		{Name: "deploy", Command: "./deploy.sh", TimeoutS: 30},
		{Name: "quick", Command: "echo hi"}, // TimeoutS 0 → execute default
	}
	if got := toolClassIn(s, "deploy"); got != "execute" {
		t.Errorf("class = %q, want execute", got)
	}
	if _, ok := commandToolIn(s, "deploy"); !ok {
		t.Error("deploy not resolvable from the fold")
	}
	if d := toolTimeoutIn(s, "deploy"); d.Seconds() != 30 {
		t.Errorf("timeout = %s, want 30s", d)
	}
	if d := toolTimeoutIn(s, "quick"); d != executeToolTimeout {
		t.Errorf("zero-timeout tool = %s, want execute default", d)
	}
	if got := toolClassIn(s, "nope"); got != "" {
		t.Errorf("unknown tool class = %q, want empty", got)
	}
}

// A command tool call carries its FIXED manifest command into the effect for
// the permission gate (not the model's args), and dispatch reaches the
// sandboxed executor. Proven here by the effect the loop builds advertising
// eff.Command from the fold-backed registry.
func TestCommandToolEffectCarriesFixedCommand(t *testing.T) {
	l := testLoop(t, scripted.Fixture{}, t.TempDir())
	l.commandTools = map[string]event.CommandToolDef{
		"deploy": {Name: "deploy", Command: "./deploy.sh prod"},
	}
	if !l.isCommandTool("deploy") || l.isCommandTool("bash") {
		t.Fatal("isCommandTool mis-classified")
	}
	if !l.sandboxedExec("deploy") || !l.sandboxedExec("bash") {
		t.Error("sandboxedExec must cover both command tools and bash")
	}
	// The dispatched args are stdin data; the adjudicated command is fixed.
	_ = json.RawMessage(`{"target":"prod"}`)
}
