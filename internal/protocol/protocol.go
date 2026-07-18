// Package protocol is the harness's OUTPUT event stream — what a surface
// (CLI, future web UI) renders. It is deliberately distinct from
// provider.StreamEvent (the provider INPUT side): this is the harness
// speaking to a human, carrying turn structure, tool activity, approvals,
// and USER-visible errors. Model-visible errors (errs.RenderForModel) go
// into the fold for the model; they are a different channel.
package protocol

import (
	"encoding/json"
	"io"
	"sync"
)

// Kind discriminates output events.
type Kind string

const (
	KindSessionStart    Kind = "session_start"
	KindGenerationStart Kind = "generation_start"
	KindTextDelta       Kind = "text_delta" // ephemeral: streamed assistant text
	KindMessage         Kind = "message"    // durable: the assembled assistant text
	KindUserInput       Kind = "user_input" // durable: a user/inbound message (shown in replay so history has both halves)
	KindToolCall        Kind = "tool_call"
	KindToolResult      Kind = "tool_result"
	KindApprovalRequest Kind = "approval_request"
	KindModeChanged     Kind = "mode_changed"
	KindDiscard         Kind = "discard" // a streamed turn was thrown away; reopen the stream
	KindError           Kind = "error"   // USER-visible error (not the model-visible render)
	KindRunEnd          Kind = "run_end"
	KindNote            Kind = "note"      // blackboard publish mirrored to watchers (S6, ephemeral)
	KindBgOutput        Kind = "bg_output" // ephemeral: a background work's stdout/stderr chunk (2.10 进度 tail; CallID = handle)
	KindIdle            Kind = "idle"      // conversational session idle for input (v2 M1.1)
	KindIteration       Kind = "iteration" // one driver iteration completed (S6; Turn = iteration N)
)

// Event is one output event. Fields are sparse — only those relevant to
// Kind are set. JSON tags drive the `--json` line stream.
type Event struct {
	Kind    Kind   `json:"kind"`
	N       int    `json:"n,omitempty"`    // generation step 序号,或 KindIteration 的迭代序号
	Text    string `json:"text,omitempty"` // text_delta / message / error / discard reason
	Tool    string `json:"tool,omitempty"` // tool_call / tool_result
	CallID  string `json:"call_id,omitempty"`
	Args    string `json:"args,omitempty"`     // tool_call args (compact JSON)
	Result  string `json:"result,omitempty"`   // tool_result payload (compact JSON)
	IsError bool   `json:"is_error,omitempty"` // tool_result / error
	Mode    string `json:"mode,omitempty"`
	Reason  string `json:"reason,omitempty"` // run_end reason
	// ApprovalID names a pending ask (approval_request) so a detached
	// client can answer it (`agentrunner approve <session> <id> ...`).
	ApprovalID string `json:"approval_id,omitempty"`
	// Session tags which run the event belongs to. Local single-run
	// rendering leaves it empty; the daemon (S6) sets it on every forwarded
	// event so a multiplexed client can tell streams apart.
	Session string `json:"session,omitempty"`
	// Seq is the follower's OWN input DeliverySeq, echoed on the daemon's
	// "delivered" ack so a blocking send/new knows which input it is — the
	// anchor for per-command output scoping (INC-73). 0 = not scoped (an older
	// daemon, or `new`): the follower falls back to render-all/detach-at-idle.
	Seq int64 `json:"seq,omitempty"`
	// InputSeqs names the input DeliverySeqs a generation just consumed
	// (KindGenerationStart only). Empty on tool-loop continuation gen-steps —
	// their owner carries over from the turn's first step. Lets a follower
	// render only its own turn under concurrent same-session sends (INC-73).
	InputSeqs []int64 `json:"input_seqs,omitempty"`
}

// Sink receives output events. Implementations must be safe for calls from
// the drive goroutine and, during a streaming LLM activity, the provider's
// delta callback — so Emit is expected to be internally synchronized.
type Sink interface {
	Emit(Event)
}

// JSONSink writes one compact JSON object per line. Concurrency-safe.
type JSONSink struct {
	mu  sync.Mutex
	w   io.Writer
	enc *json.Encoder
}

func NewJSONSink(w io.Writer) *JSONSink {
	return &JSONSink{w: w, enc: json.NewEncoder(w)}
}

func (s *JSONSink) Emit(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.enc.Encode(e)
}

// discardSink drops everything (nil-safe default).
type discardSink struct{}

func (discardSink) Emit(Event) {}

// Discard is a no-op sink.
var Discard Sink = discardSink{}
