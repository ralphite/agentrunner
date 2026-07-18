package tool

import (
	"context"
	"io"
)

// Live background output (2.10 进度通道, audit-0717 B9): a sandboxed
// execution whose ctx carries a live sink tees its stdout/stderr chunks to
// that sink AS they are written, so the agent can show a running background
// work's tail (`output` tool) and mirror it to surfaces ephemerally. The
// journal is untouched — the durable record stays the completion result,
// the same doctrine as token deltas.

type liveOutputKey struct{}

// WithLiveOutput returns a ctx whose sandboxed executions tee their combined
// stdout/stderr chunks to fn. fn must be safe for concurrent calls: the
// stdout and stderr pipe copiers run on separate goroutines.
func WithLiveOutput(ctx context.Context, fn func(chunk []byte)) context.Context {
	return context.WithValue(ctx, liveOutputKey{}, fn)
}

func liveOutput(ctx context.Context) func([]byte) {
	fn, _ := ctx.Value(liveOutputKey{}).(func(chunk []byte))
	return fn
}

// teeWriter mirrors every chunk written to dst into fn, copying first — the
// caller may reuse p after Write returns.
type teeWriter struct {
	dst io.Writer
	fn  func([]byte)
}

func (w teeWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	if n > 0 {
		c := make([]byte, n)
		copy(c, p[:n])
		w.fn(c)
	}
	return n, err
}
