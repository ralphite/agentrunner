package structured

import (
	"encoding/json"
	"strings"
	"testing"
)

const personSchema = `{
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "lines": {"type": "integer"}
  },
  "required": ["name", "lines"],
  "additionalProperties": false
}`

func TestStructuredCompile(t *testing.T) {
	if _, err := Compile([]byte(personSchema)); err != nil {
		t.Fatalf("valid schema failed to compile: %v", err)
	}
	if _, err := Compile([]byte("")); err == nil {
		t.Error("empty schema compiled without error")
	}
	if _, err := Compile([]byte("{not json")); err == nil {
		t.Error("malformed schema compiled without error")
	}
}

func TestStructuredValidate(t *testing.T) {
	v, err := Compile([]byte(personSchema))
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Validate([]byte(`{"name":"a.go","lines":42}`)); err != nil {
		t.Errorf("conforming value rejected: %v", err)
	}
	// Wrong type for lines.
	if err := v.Validate([]byte(`{"name":"a.go","lines":"many"}`)); err == nil {
		t.Error("type mismatch accepted")
	}
	// Missing required field.
	if err := v.Validate([]byte(`{"name":"a.go"}`)); err == nil {
		t.Error("missing required field accepted")
	}
	// Extra field barred by additionalProperties:false.
	if err := v.Validate([]byte(`{"name":"a.go","lines":1,"x":2}`)); err == nil {
		t.Error("additional property accepted")
	}
	// Not JSON at all.
	if err := v.Validate([]byte(`not json`)); err == nil {
		t.Error("non-JSON accepted")
	}
}

func TestStructuredExtract(t *testing.T) {
	want := `{"name":"a.go","lines":42}`
	cases := []struct {
		desc, in string
		wantErr  bool
	}{
		{"bare object", `{"name":"a.go","lines":42}`, false},
		{"json fence", "```json\n" + want + "\n```", false},
		{"bare fence", "```\n" + want + "\n```", false},
		{"prose then json", "Here is the result:\n" + want + "\nThanks!", false},
		{"prose then fence", "Sure!\n```json\n" + want + "\n```\nDone.", false},
		{"array top level", `[{"a":1},{"b":2}]`, false},
		{"brace inside string", `{"name":"a}b","lines":1}`, false},
		{"no json", "there is no json here", true},
		{"unbalanced", `{"name":"a.go"`, true},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			got, err := Extract(c.in)
			if c.wantErr {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// The extracted bytes must be valid JSON.
			var v any
			if e := json.Unmarshal(got, &v); e != nil {
				t.Errorf("extracted non-JSON %q: %v", got, e)
			}
		})
	}
}

// TestStructuredExtractThenValidate is the end-to-end unit: pull JSON out of a
// messy answer and confirm it validates — the exact CLI sequence.
func TestStructuredExtractThenValidate(t *testing.T) {
	v, err := Compile([]byte(personSchema))
	if err != nil {
		t.Fatal(err)
	}
	answer := "I counted the lines.\n```json\n{\n  \"name\": \"main.go\",\n  \"lines\": 128\n}\n```\nLet me know if you need more."
	raw, err := Extract(answer)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if err := v.Validate(raw); err != nil {
		t.Fatalf("validate: %v", err)
	}
	canon, err := Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Canonical form is compact and key-sorted.
	if got := string(canon); got != `{"lines":128,"name":"main.go"}` {
		t.Errorf("canonical = %q, want compact key-sorted", got)
	}
	if strings.Contains(string(canon), "\n") {
		t.Error("canonical form should be compact (no newlines)")
	}
}
