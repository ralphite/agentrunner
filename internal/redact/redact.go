// Package redact scrubs credential values from anything headed for durable
// storage (events, fixtures) or the model-visible face. The harness never
// journals a credential: values of env vars matching Suffixes are replaced
// by [REDACTED:<VAR>].
package redact

import (
	"encoding/json"
	"os"
	"strings"
)

// Suffixes marks env vars whose values must never be persisted.
var Suffixes = []string{"_API_KEY", "_TOKEN", "_SECRET"}

// MinSecretLen is the shortest value the redactor registers. Substring
// replacement of a shorter value shreds unrelated text — a `*_TOKEN` of
// "test" or "1" rewrote every occurrence of that string on every journaled
// and model-visible surface (audit-2026-07-18-guardrails P0-1). A value
// this short is a placeholder, not a credential: real API keys run 20+
// characters, and an sub-8-char true secret could not be meaningfully
// protected by substring replacement anyway (documented residual risk).
const MinSecretLen = 8

// placeholders are common stand-in values that clear MinSecretLen yet are
// never real credentials; registering them guarantees false positives.
// Compared case-insensitively.
var placeholders = map[string]bool{
	"changeme": true, "changeit": true, "password": true, "passw0rd": true,
	"placeholder": true, "redacted": true, "disabled": true, "undefined": true,
	"xxxxxxxx": true, "youtokenhere": true, "yourkeyhere": true,
}

// Plausible reports whether a credential-suffixed env value is worth
// registering as a secret: long enough that substring replacement cannot
// shred ordinary text, and not a well-known placeholder. Shared with the
// fixture recorder so both redaction faces apply one rule.
func Plausible(v string) bool {
	if len(v) < MinSecretLen {
		return false
	}
	return !placeholders[strings.ToLower(v)]
}

type Redactor struct {
	replacer *strings.Replacer
	empty    bool
}

// FromEnv builds a redactor over the current environment.
func FromEnv() *Redactor {
	var pairs []string
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || !Plausible(v) {
			continue
		}
		for _, suffix := range Suffixes {
			if strings.HasSuffix(k, suffix) {
				pairs = append(pairs, v, "[REDACTED:"+k+"]")
				break
			}
		}
	}
	return &Redactor{replacer: strings.NewReplacer(pairs...), empty: len(pairs) == 0}
}

func (r *Redactor) String(s string) string {
	if r.empty {
		return s
	}
	return r.replacer.Replace(s)
}

// JSON redacts at the text level; a secret containing JSON metacharacters
// could in theory corrupt the payload, which is acceptable — corrupt
// beats leaked.
func (r *Redactor) JSON(raw json.RawMessage) json.RawMessage {
	if r.empty || raw == nil {
		return raw
	}
	return json.RawMessage(r.String(string(raw)))
}
