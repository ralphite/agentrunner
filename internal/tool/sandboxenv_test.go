package tool

import (
	"fmt"
	"strings"
	"testing"
)

// The sandbox env withholds credential vars by default, reports their NAMES,
// and passes through exactly the ones the sealed spec list allows
// (audit-0718 P0-2/P0-3). Pure function — no OS sandbox backend needed.
func TestSandboxEnvironmentWithholdsAndPassesThrough(t *testing.T) {
	t.Setenv("SBXTEST_API_KEY", "value-a-12345")
	t.Setenv("SBXTEST_TOKEN", "value-b-12345")
	t.Setenv("SBXTEST_PLAIN", "plain-value")

	env, withheld := sandboxEnvironment("/sandbox-home", "sess", []string{"SBXTEST_TOKEN"})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "value-a-12345") {
		t.Fatal("withheld credential leaked into sandbox env")
	}
	if !strings.Contains(joined, "SBXTEST_TOKEN=value-b-12345") {
		t.Fatal("passthrough var missing from sandbox env")
	}
	if !strings.Contains(joined, "SBXTEST_PLAIN=plain-value") {
		t.Fatal("non-credential var should always pass")
	}
	got := strings.Join(withheld, ",")
	if !strings.Contains(got, "SBXTEST_API_KEY") {
		t.Fatalf("withheld list missing SBXTEST_API_KEY: %v", withheld)
	}
	if strings.Contains(got, "SBXTEST_TOKEN") {
		t.Fatalf("passthrough var wrongly reported withheld: %v", withheld)
	}
}

// Passthrough can never rescue sandbox-critical vars: HOME/TMP stay isolated.
func TestSandboxEnvironmentPassthroughNeverRescuesHome(t *testing.T) {
	env, _ := sandboxEnvironment("/sandbox-home", "", []string{"HOME"})
	for _, kv := range env {
		if strings.HasPrefix(kv, "HOME=") && kv != "HOME=/sandbox-home" {
			t.Fatalf("HOME escaped isolation: %s", kv)
		}
	}
}

// The seal is first-wins: a later (child) seal cannot widen the face.
func TestSealEnvPassthroughFirstWins(t *testing.T) {
	e := &Executor{}
	e.SealEnvPassthrough(nil)                      // root spec: nothing passed through
	e.SealEnvPassthrough([]string{"EVIL_API_KEY"}) // child attempt: must be a no-op
	if got := e.EnvPassthrough(); len(got) != 0 {
		t.Fatalf("child seal widened the passthrough face: %v", got)
	}
}

// End-to-end through the real OS sandbox: the bash result carries the
// explicit withheld list, and a passthrough var is visible to the command.
func TestBashCredentialWithholdingExplicitAndPassthrough(t *testing.T) {
	e, _ := newExec(t)
	t.Setenv("SBXE2E_API_KEY", "with-held-value-1234")
	t.Setenv("SBXE2E_TOKEN", "passed-thru-value-1234")
	if _, err := e.SandboxInfo(); err != nil {
		t.Skipf("no OS sandbox backend here: %v", err)
	}
	e.SealEnvPassthrough([]string{"SBXE2E_TOKEN"})

	out, _ := run(t, e, "bash", `{"command":"printf -- \"tok=$SBXE2E_TOKEN key=$SBXE2E_API_KEY\""}`)
	stdout, _ := out["stdout"].(string)
	if !strings.Contains(stdout, "tok=passed-thru-value-1234") {
		t.Fatalf("passthrough var not visible in bash: %v", out)
	}
	if strings.Contains(stdout, "with-held-value-1234") {
		t.Fatalf("withheld credential visible in bash: %v", out)
	}
	names := fmt.Sprintf("%v", out["credential_env_withheld"])
	if !strings.Contains(names, "SBXE2E_API_KEY") {
		t.Fatalf("result does not report the withheld var: %v", out)
	}
	if strings.Contains(names, "SBXE2E_TOKEN") {
		t.Fatalf("passthrough var wrongly reported withheld: %v", out)
	}
}
