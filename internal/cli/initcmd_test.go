package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/agent"
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
