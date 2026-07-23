package modelconfig

import "testing"

func TestEffortMapping(t *testing.T) {
	tests := []struct {
		effort string
		budget int
		max    int
	}{
		{EffortLight, 2048, 6144},
		{EffortMedium, 6144, 10240},
		{EffortHigh, 12288, 16384},
		{EffortXHigh, 24576, 28672},
	}
	for _, tt := range tests {
		t.Run(tt.effort, func(t *testing.T) {
			spec, err := (Selection{Provider: "gemini", ID: "m", Effort: tt.effort}).Resolve()
			if err != nil {
				t.Fatal(err)
			}
			if spec.Thinking.BudgetTokens != tt.budget || spec.MaxTokens != tt.max {
				t.Fatalf("got budget=%d max=%d, want %d/%d",
					spec.Thinking.BudgetTokens, spec.MaxTokens, tt.budget, tt.max)
			}
		})
	}
}

func TestWithExplicitPrecedence(t *testing.T) {
	configured := Selection{Provider: "anthropic", ID: "claude-x", Effort: EffortLight}
	got, err := WithExplicit(configured, "gemini/gemini-y", EffortHigh)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "gemini" || got.ID != "gemini-y" || got.Effort != EffortHigh {
		t.Fatalf("explicit selection did not win: %+v", got)
	}

	fallback, err := WithExplicit(Selection{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if fallback != Default() {
		t.Fatalf("empty selection = %+v, want compiled default %+v", fallback, Default())
	}
}

func TestParseRefRejectsAmbiguousModel(t *testing.T) {
	for _, ref := range []string{"gemini", "/x", "gemini/"} {
		if _, err := ParseRef(ref, EffortMedium); err == nil {
			t.Fatalf("ParseRef(%q) succeeded", ref)
		}
	}
}
