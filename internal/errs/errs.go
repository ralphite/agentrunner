// Package errs is the error taxonomy (2.8). Everything downstream — retry
// policy (2.10), in-doubt handling (2.15), model-visible rendering (3.9) —
// consumes ONLY the class, never provider-specific error shapes.
package errs

import (
	"context"
	"errors"
	"fmt"
)

type Class string

const (
	ProviderRateLimit Class = "provider_rate_limit"
	ProviderServer    Class = "provider_server"
	ProviderAuth      Class = "provider_auth"
	ProviderInvalid   Class = "provider_invalid"
	ToolFailed        Class = "tool_failed"
	Timeout           Class = "timeout"
	Canceled          Class = "canceled"
	Internal          Class = "internal"
)

// Retryable is the retry policy's single input: transient classes only.
func (c Class) Retryable() bool {
	switch c {
	case ProviderRateLimit, ProviderServer, Timeout:
		return true
	}
	return false
}

// ErrActivityTimeout is the cancellation CAUSE used when a durable
// activity-timeout timer fires (2.11). Effect implementations inspect
// context.Cause to render "timed out" instead of "canceled".
var ErrActivityTimeout = New(Timeout, "activity timeout")

// Error carries a class through wrapping.
type Error struct {
	Class Class
	Msg   string
	Err   error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s [%s]: %v", e.Msg, e.Class, e.Err)
	}
	return fmt.Sprintf("%s [%s]", e.Msg, e.Class)
}

func (e *Error) Unwrap() error { return e.Err }

func New(class Class, format string, args ...any) *Error {
	return &Error{Class: class, Msg: fmt.Sprintf(format, args...)}
}

func Wrap(class Class, err error, msg string) *Error {
	return &Error{Class: class, Msg: msg, Err: err}
}

// ClassOf classifies any error: a wrapped *Error wins; context sentinels
// map to canceled/timeout; everything else is internal.
func ClassOf(err error) Class {
	var e *Error
	if errors.As(err, &e) {
		return e.Class
	}
	if errors.Is(err, context.Canceled) {
		return Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Timeout
	}
	return Internal
}

// Retryable reports whether the error's class permits a retry.
func Retryable(err error) bool { return ClassOf(err).Retryable() }

// FromHTTPStatus maps a provider HTTP status to a class (Gemini and
// Anthropic both follow these conventions).
func FromHTTPStatus(code int) Class {
	switch {
	case code == 429:
		return ProviderRateLimit
	case code >= 500:
		return ProviderServer
	case code == 401 || code == 403:
		return ProviderAuth
	case code >= 400:
		return ProviderInvalid
	default:
		return Internal
	}
}
