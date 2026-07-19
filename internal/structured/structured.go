// Package structured implements headless structured output (INC-26, #91): it
// compiles a JSON Schema, extracts a JSON value from a model's free-text
// answer, and validates it. It is a pure, side-effect-free helper — the CLI
// layer orchestrates the run/retry loop around it, so nothing here touches the
// journal, the loop, or a provider.
package structured

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// Validator is a compiled, resolved JSON Schema ready to validate values.
type Validator struct {
	resolved *jsonschema.Resolved
}

// Compile parses and resolves a JSON Schema. A malformed schema is reported
// here — before any run starts — so a bad output_schema fails fast rather than
// after burning a model turn.
func Compile(raw []byte) (*Validator, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, errors.New("empty schema")
	}
	// A JSON Schema must be a JSON object. A bare array/number/string/bool
	// otherwise fails deep inside the struct unmarshal below, spilling the
	// entire jsonschema.Schema Go type into the message (QA Wave2 frank-03).
	// Catch it up front with an actionable error.
	var top any
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("schema is not valid JSON: %w", err)
	}
	if _, ok := top.(map[string]any); !ok {
		return nil, errors.New(`schema must be a JSON object, e.g. {"type":"object","properties":{...}}`)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("resolve schema: %w", err)
	}
	return &Validator{resolved: resolved}, nil
}

// Validate reports whether raw is a JSON value that conforms to the schema.
// The returned error is human-readable and is fed back to the model on retry.
func (v *Validator) Validate(raw []byte) error {
	var instance any
	if err := json.Unmarshal(raw, &instance); err != nil {
		return fmt.Errorf("not valid JSON: %w", err)
	}
	if err := v.resolved.Validate(instance); err != nil {
		return fmt.Errorf("does not match schema: %w", err)
	}
	return nil
}

// Canonical re-marshals raw as compact, key-sorted JSON so the printed
// structured_output is stable regardless of how the model spaced it. raw is
// assumed to already be valid JSON (Validate passed).
func Canonical(raw []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	// encoding/json sorts object keys when marshaling a map[string]any.
	return json.Marshal(v)
}

// Extract pulls the JSON value out of a model answer. Models wrap JSON in
// ```json fences, precede it with prose, or (ideally) return it bare — this
// recovers the value in all three cases by preferring a fenced block and
// otherwise scanning for the first balanced {...} or [...]. It does NOT
// validate JSON-ness beyond bracket balance; Validate is the real gate.
func Extract(answer string) (json.RawMessage, error) {
	if fenced, ok := fencedBlock(answer); ok {
		answer = fenced
	}
	start, closer, ok := firstStructuralOpen(answer)
	if !ok {
		return nil, errors.New("no JSON object or array found in answer")
	}
	end, ok := matchClose(answer, start, closer)
	if !ok {
		return nil, errors.New("unbalanced JSON in answer")
	}
	return json.RawMessage(answer[start : end+1]), nil
}

// fencedBlock returns the contents of the first ``` fenced code block, if any.
// A leading language tag ("json") on the opening fence line is dropped.
func fencedBlock(s string) (string, bool) {
	const fence = "```"
	i := strings.Index(s, fence)
	if i < 0 {
		return "", false
	}
	rest := s[i+len(fence):]
	// Drop the rest of the opening fence line (the optional language tag).
	nl := strings.IndexByte(rest, '\n')
	if nl < 0 {
		return "", false
	}
	rest = rest[nl+1:]
	j := strings.Index(rest, fence)
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// firstStructuralOpen finds the first '{' or '[' and its matching closer.
func firstStructuralOpen(s string) (idx int, closer byte, ok bool) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			return i, '}', true
		case '[':
			return i, ']', true
		}
	}
	return 0, 0, false
}

// matchClose scans from the opener at start to its matching closer, honoring
// JSON string literals and escapes so braces inside strings do not count.
func matchClose(s string, start int, closer byte) (int, bool) {
	opener := s[start]
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case opener:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}
