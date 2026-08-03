package agent

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ralphite/agentrunner/internal/index"
	"github.com/ralphite/agentrunner/internal/redact"
)

// G60: the env-based redactor only knows values the PROCESS holds. A
// daemon-hosted session whose workspace carries a .env the daemon never
// loaded would journal `cat .env` output verbatim. At session start/resume
// we scan the workspace roots for credential-shaped files (the same
// index.SkipFile set the search tools exclude), parse candidate values, and
// register them with the redactor. Best-effort and bounded — this narrows
// the documented residual risk, it does not close it.
const (
	credScanMaxEntries   = 50_000     // walk entries per root (large-workspace bound)
	credScanMaxFileBytes = 256 * 1024 // per-file read cap
	credScanMaxValues    = 512        // registered values per scan
)

// registerWorkspaceCredentials scans every workspace root and registers
// credential-file values with the process redactor. Idempotent; called from
// Run and Resume after the sandbox is applied.
func (l *Loop) registerWorkspaceCredentials() {
	if l.Exec == nil || l.Exec.WS == nil {
		return
	}
	budget := credScanMaxValues
	for _, root := range l.Exec.WS.Roots() {
		if budget <= 0 {
			return
		}
		budget -= scanCredentialRoot(root, budget)
	}
}

// scanCredentialRoot walks one root, pruning vendored/derived trees but
// deliberately ENTERING credential dirs (.ssh/.aws/…): the goal here is to
// harvest values for redaction, the opposite of the search walks. Returns
// how many values it registered.
func scanCredentialRoot(root string, budget int) int {
	registered := 0
	entries := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		entries++
		if entries > credScanMaxEntries {
			return fs.SkipAll
		}
		if d.IsDir() {
			if path != root && index.VendoredDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !index.SkipFile(d.Name()) {
			return nil
		}
		if registered >= budget {
			return fs.SkipAll
		}
		registered += registerCredentialFile(path, budget-registered)
		return nil
	})
	return registered
}

// registerCredentialFile parses one credential-shaped file and registers up
// to budget candidate values. Format dispatch is by name: JSON files walk
// string leaves; key material (*.pem/*.key/id_*) registers content lines;
// everything else parses env/ini-style KEY=VALUE lines.
func registerCredentialFile(path string, budget int) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, credScanMaxFileBytes)
	n, _ := f.Read(buf)
	content := string(buf[:n])
	base := filepath.Base(path)
	switch {
	case strings.HasSuffix(base, ".json"):
		return registerJSONLeaves(base, content, budget)
	case strings.HasSuffix(base, ".pem"), strings.HasSuffix(base, ".key"),
		strings.HasPrefix(base, "id_rsa"), strings.HasPrefix(base, "id_ed25519"):
		return registerKeyMaterial(base, content, budget)
	default:
		return registerEnvStyle(base, content, budget)
	}
}

// registerEnvStyle handles dotenv / ini / netrc-ish lines: KEY=VALUE and
// `key = value`, quotes stripped, comments skipped. The Plausible gate in
// RegisterSecret drops short/placeholder values.
func registerEnvStyle(label, content string, budget int) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if count >= budget {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			// netrc shape: `password VALUE` / `login VALUE` tokens.
			fields := strings.Fields(line)
			for i := 0; i+1 < len(fields); i += 2 {
				if strings.EqualFold(fields[i], "password") || strings.EqualFold(fields[i], "token") {
					redact.RegisterSecret(label, fields[i+1])
					count++
				}
			}
			continue
		}
		key := strings.TrimSpace(k)
		val := strings.Trim(strings.TrimSpace(v), `"'`)
		if key == "" || val == "" {
			continue
		}
		redact.RegisterSecret(label+":"+key, val)
		count++
	}
	return count
}

// registerKeyMaterial registers each content line of a key file (BEGIN/END
// markers excluded) so `cat id_rsa` is unreadable in the journal.
func registerKeyMaterial(label, content string, budget int) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if count >= budget {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "-----") {
			continue
		}
		redact.RegisterSecret(label, line)
		count++
	}
	return count
}

// registerJSONLeaves walks a JSON document and registers every string leaf
// (credentials.json holds tokens under arbitrary keys).
func registerJSONLeaves(label, content string, budget int) int {
	var doc any
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return 0
	}
	count := 0
	var walk func(v any)
	walk = func(v any) {
		if count >= budget {
			return
		}
		switch t := v.(type) {
		case string:
			redact.RegisterSecret(label, t)
			count++
		case map[string]any:
			for _, mv := range t {
				walk(mv)
			}
		case []any:
			for _, av := range t {
				walk(av)
			}
		}
	}
	walk(doc)
	return count
}
