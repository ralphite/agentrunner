package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/agent"
	"github.com/ralphite/agentrunner/internal/driver"
)

// INC-2 BB-me-3: `init` writes a spec that actually loads — the template is
// the spec schema's discoverable form, so it must never drift from LoadSpec.
func TestInitWritesLoadableSpec(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec.yaml")
	var out, errOut bytes.Buffer
	if code := initCmd([]string{path}, &out, &errOut); code != ExitOK {
		t.Fatalf("init: exit %d, stderr %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "wrote "+path) {
		t.Fatalf("stdout = %q, want wrote %s", out.String(), path)
	}
	spec, err := agent.LoadSpec(path)
	if err != nil {
		t.Fatalf("generated spec does not load: %v", err)
	}
	if spec.Name == "" || spec.Model.Provider == "" {
		t.Fatalf("generated spec incomplete: %+v", spec)
	}
}

// init must refuse to clobber an existing file.
func TestInitRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec.yaml")
	var out, errOut bytes.Buffer
	if code := initCmd([]string{path}, &out, &errOut); code != ExitOK {
		t.Fatalf("first init: exit %d", code)
	}
	out.Reset()
	errOut.Reset()
	if code := initCmd([]string{path}, &out, &errOut); code != ExitUsage {
		t.Fatalf("second init: exit %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut.String(), "already exists") {
		t.Fatalf("stderr = %q, want already exists", errOut.String())
	}
}

// The --driver template must itself load (next to a default spec.yaml) —
// the driver schema's discoverable form (QA Round2 F-E9).
func TestInitDriverSpecLoads(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	var out, errOut bytes.Buffer
	if code := initCmd(nil, &out, &errOut); code != ExitOK {
		t.Fatalf("init: %d %s", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := initCmd([]string{"--driver"}, &out, &errOut); code != ExitOK {
		t.Fatalf("init --driver: %d %s", code, errOut.String())
	}
	spec, err := driver.LoadSpec(filepath.Join(dir, "driver.yaml"))
	if err != nil {
		t.Fatalf("driver template does not load: %v", err)
	}
	if len(spec.Verifiers) == 0 || spec.Verifiers[0].Kind != driver.VerifierCommand {
		t.Fatalf("template verifiers = %+v", spec.Verifiers)
	}
}
