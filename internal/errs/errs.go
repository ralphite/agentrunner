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

// ErrUserInterrupt is the cancellation CAUSE for a collaborative steering
// interrupt (S4.2, first Ctrl-C): the current activity is cancelled but
// the run CONTINUES. Effect implementations use a shorter kill grace for
// it than for a hard cancel, and the loop resumes rather than aborting.
var ErrUserInterrupt = New(Canceled, "user interrupt")

// ErrSessionStopped is the cancellation CAUSE for daemon `stop`: a user asked
// to tear down hosting without closing the session. The loop records a
// restartable SessionClosed{stopped} mark instead of looking stranded.
var ErrSessionStopped = errors.New("session stopped")

// KilledError is the cancellation CAUSE of an explicit kill (决策 #30): it
// records WHO asked ("user" via ar kill / the surface, "parent" via the
// parent model's kill tool), so the dying child journals a
// SessionClosed{killed, source} mark and revival checks can honor the
// origin (a user-killed child revives only for the user). Process teardown
// carries no KilledError and leaves no mark.
type KilledError struct{ Source string } // user | parent

func (e *KilledError) Error() string { return "killed by " + e.Source }

// KillSource extracts the kill origin from a context's cancel cause; ""
// when the cancellation was not an explicit kill.
func KillSource(ctx context.Context) string {
	var ke *KilledError
	if errors.As(context.Cause(ctx), &ke) {
		return ke.Source
	}
	return ""
}

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
// and explicit kills map to canceled/timeout; everything else is internal.
func ClassOf(err error) Class {
	var e *Error
	if errors.As(err, &e) {
		return e.Class
	}
	var ke *KilledError
	if errors.As(err, &ke) {
		return Canceled
	}
	if errors.Is(err, context.Canceled) {
		return Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Timeout
	}
	return Internal
}

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
