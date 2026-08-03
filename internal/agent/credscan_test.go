package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ralphite/agentrunner/internal/redact"
)

// G60: values from workspace credential files (which the process env never
// held) are registered at scan time and redacted on every journal-bound
// surface; short/placeholder values stay untouched, and ordinary files are
// never parsed.
func TestWorkspaceCredentialScanRegistersValues(t *testing.T) {
	redact.ResetRegistered()
	t.Cleanup(redact.ResetRegistered)
	root := t.TempDir()
	mk := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk(".env", "PROBE_SECRET=workspacesecret123\nSHORT=abc\n# COMMENT=nope\n")
	mk(".netrc", "machine example.com login bob password netrcsecret999\n")
	mk(".ssh/id_rsa", "-----BEGIN OPENSSH PRIVATE KEY-----\nkeymaterialline0001\n-----END OPENSSH PRIVATE KEY-----\n")
	mk("credentials.json", `{"installed":{"client_secret":"jsonleafsecret42"}}`)
	mk("node_modules/pkg/.env", "VENDORED_SECRET=vendoredsecret777\n")
	mk("README.md", "innocentcontent999\n")

	if got := scanCredentialRoot(root, credScanMaxValues); got < 4 {
		t.Fatalf("registered %d values, want >= 4", got)
	}
	r := redact.FromEnv()
	for _, secret := range []string{"workspacesecret123", "netrcsecret999", "keymaterialline0001", "jsonleafsecret42"} {
		if out := r.String("x " + secret + " y"); out == "x "+secret+" y" {
			t.Errorf("%q not redacted: %q", secret, out)
		}
	}
	// Vendored trees are pruned; ordinary content and short values untouched.
	for _, keep := range []string{"vendoredsecret777", "abc", "innocentcontent999"} {
		if out := r.String(keep); out != keep {
			t.Errorf("%q unexpectedly rewritten to %q", keep, out)
		}
	}
}
