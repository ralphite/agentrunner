package errs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRetryablePolicy(t *testing.T) {
	cases := []struct {
		class Class
		want  bool
	}{
		{ProviderRateLimit, true},
		{ProviderServer, true},
		{Timeout, true},
		{ProviderAuth, false},
		{ProviderInvalid, false},
		{ToolFailed, false},
		{Canceled, false},
		{Internal, false},
	}
	for _, tc := range cases {
		if got := tc.class.Retryable(); got != tc.want {
			t.Errorf("%s.Retryable() = %v, want %v", tc.class, got, tc.want)
		}
	}
}

func TestFromHTTPStatus(t *testing.T) {
	cases := []struct {
		code int
		want Class
	}{
		{429, ProviderRateLimit},
		{500, ProviderServer},
		{503, ProviderServer},
		{401, ProviderAuth},
		{403, ProviderAuth},
		{400, ProviderInvalid},
		{404, ProviderInvalid},
		{200, Internal},
	}
	for _, tc := range cases {
		if got := FromHTTPStatus(tc.code); got != tc.want {
			t.Errorf("FromHTTPStatus(%d) = %s, want %s", tc.code, got, tc.want)
		}
	}
}

func TestClassOf(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", New(ProviderRateLimit, "quota"))
	cases := []struct {
		name string
		err  error
		want Class
	}{
		{"typed error through wrapping", wrapped, ProviderRateLimit},
		{"context canceled", fmt.Errorf("run: %w", context.Canceled), Canceled},
		{"deadline exceeded", context.DeadlineExceeded, Timeout},
		{"plain error", errors.New("mystery"), Internal},
		{"nil-safe default", fmt.Errorf("x"), Internal},
	}
	for _, tc := range cases {
		if got := ClassOf(tc.err); got != tc.want {
			t.Errorf("%s: ClassOf = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestErrorFormatAndUnwrap(t *testing.T) {
	base := errors.New("boom")
	e := Wrap(ProviderServer, base, "gemini")
	if !errors.Is(e, base) {
		t.Error("Wrap must preserve the chain")
	}
	if got := e.Error(); got != "gemini [provider_server]: boom" {
		t.Errorf("Error() = %q", got)
	}
	if !Retryable(e) {
		t.Error("wrapped provider_server must be retryable")
	}
}

// 3.9: every class has a rendering row.
func TestRenderForModelTable(t *testing.T) {
	cases := []struct {
		class Class
		want  string
	}{
		{ProviderRateLimit, "rate limited"},
		{ProviderServer, "server error"},
		{ProviderAuth, "rejected the harness credentials"},
		{ProviderInvalid, "invalid"},
		{ToolFailed, "tool failed"},
		{Timeout, "timed out"},
		{Canceled, "canceled"},
		{Internal, "internal harness error"},
		{Class("martian"), "internal harness error"}, // unknown → internal
	}
	for _, tc := range cases {
		raw := RenderForModel(tc.class, "extra detail")
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("%s: %v", tc.class, err)
		}
		if !strings.Contains(m["error"], tc.want) || !strings.Contains(m["error"], "extra detail") {
			t.Errorf("%s: rendered = %q", tc.class, m["error"])
		}
	}
}
