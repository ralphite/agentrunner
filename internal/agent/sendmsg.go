package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/redact"
	"github.com/ralphite/agentrunner/internal/tool"
)

// runSendMessage executes the send_message tool (INC-12, DESIGN §3 树内
// 消息): resolve the recipient inside this session tree, deliver durably to
// its inbox through the TreeRouter, and report the delivery. Every failure
// is model-visible; the harness never fails on a message call. Runs on an
// activity goroutine — it touches no fold state beyond the snapshots
// captured at dispatch (children, commandID).
func (l *Loop) runSendMessage(children []string, commandID string, rawArgs json.RawMessage) tool.Result {
	if l.Router == nil {
		return errorResult("send_message: no session tree here (multi-agent face closed)")
	}
	var args struct {
		To   string `json:"to"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.To == "" || args.Text == "" {
		return errorResult("send_message: invalid args: need {\"to\", \"text\"}")
	}

	to := strings.TrimSpace(args.To)
	switch {
	case to == "parent":
		if to = ParentOf(l.SessionID); to == "" {
			return errorResult("send_message: this session has no parent")
		}
	case l.Router.InTree(to):
		// A full tree session id — use as is.
	default:
		// A handle the caller owns: resolve to the child session with the
		// highest attempt for that call id.
		if resolved := resolveChildHandle(children, l.SessionID, to); resolved != "" {
			to = resolved
			break
		}
		return errorResult(fmt.Sprintf(
			"send_message: %q is neither \"parent\", a session id in this tree, nor a handle you own", args.To))
	}
	if to == l.SessionID {
		return errorResult("send_message: cannot message yourself")
	}

	// The sender rides as a text prefix (weak-typed Input, 裁决 #9); the
	// journal-side source is metadata. The body is redacted like every
	// journaled input — it crosses session boundaries and lands durably.
	text := fmt.Sprintf("[message from %s (%s)]\n%s",
		l.Spec.Name, l.SessionID, redact.FromEnv().String(args.Text))
	seq, err := l.Router.Send(to, protocol.UserInput{
		Text: text, Source: "agent", CommandID: commandID,
	})
	if err != nil {
		return errorResult("send_message: " + err.Error())
	}
	payload, _ := json.Marshal(map[string]any{
		"delivered_to": to, "delivery_seq": seq,
		"note": "durable; a running recipient sees it at its next safe point, an idle one is woken",
	})
	return tool.Result{Payload: payload}
}

// resolveChildHandle maps a spawn handle (call id) owned by parentSID to the
// child session id with the highest attempt, from the fold's child-session
// list. "" when the handle matches nothing.
func resolveChildHandle(children []string, parentSID, handle string) string {
	prefix := parentSID + "-sub-" + handle + "-a"
	best := ""
	for _, c := range children {
		if !strings.HasPrefix(c, prefix) {
			continue
		}
		// Reject deeper descendants: the attempt suffix must be the LAST
		// segment (no further "-sub-").
		if strings.Contains(c[len(prefix):], "-sub-") {
			continue
		}
		if best == "" || attemptOf(c) > attemptOf(best) {
			best = c
		}
	}
	return best
}

func attemptOf(sid string) int {
	idx := strings.LastIndex(sid, "-a")
	if idx < 0 {
		return 0
	}
	n := 0
	for _, r := range sid[idx+2:] {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
