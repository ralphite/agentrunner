package redact

import (
	"strings"
	"testing"
)

// A short credential value must NOT be registered: substring-replacing it
// shreds unrelated text everywhere the redactor runs (the audit-0718
// [REDACTED:…] pollution incident — a *_TOKEN of "test" rewrote "latest",
// "testing", …; a value of "1" destroyed every JSON number).
func TestShortValueNotRegistered(t *testing.T) {
	for _, v := range []string{"test", "1", "true", "abc", "1234567"} {
		t.Setenv("POLLUTE_TOKEN", v)
		out := FromEnv().String("run the test suite; exit_code=1; latest")
		if strings.Contains(out, "[REDACTED") {
			t.Fatalf("value %q polluted output: %s", v, out)
		}
	}
}

// A well-known placeholder clears the length bar but is still skipped.
func TestPlaceholderValueNotRegistered(t *testing.T) {
	t.Setenv("SETUP_API_KEY", "changeme")
	out := FromEnv().String("edit .env and replace changeme with your key")
	if strings.Contains(out, "[REDACTED") {
		t.Fatalf("placeholder polluted output: %s", out)
	}
}

// A realistic credential value still redacts everywhere.
func TestRealValueStillRedacted(t *testing.T) {
	t.Setenv("REAL_API_KEY", "sk-live-abc123def456")
	out := FromEnv().String("header was Bearer sk-live-abc123def456 ok")
	if strings.Contains(out, "sk-live-abc123def456") || !strings.Contains(out, "[REDACTED:REAL_API_KEY]") {
		t.Fatalf("real credential not redacted: %s", out)
	}
}

func TestPlausible(t *testing.T) {
	for v, want := range map[string]bool{
		"":                     false,
		"1":                    false,
		"test":                 false,
		"1234567":              false, // one under MinSecretLen
		"12345678":             true,  // exactly MinSecretLen
		"CHANGEME":             false, // placeholder, case-insensitive
		"sk-live-abc123def456": true,
	} {
		if got := Plausible(v); got != want {
			t.Errorf("Plausible(%q) = %v, want %v", v, got, want)
		}
	}
}
