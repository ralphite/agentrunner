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

type Redactor struct {
	replacer *strings.Replacer
	empty    bool
}

// FromEnv builds a redactor over the current environment.
func FromEnv() *Redactor {
	var pairs []string
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || v == "" {
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
