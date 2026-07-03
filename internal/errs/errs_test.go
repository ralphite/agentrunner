package errs

import (
	"context"
	"errors"
	"fmt"
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
