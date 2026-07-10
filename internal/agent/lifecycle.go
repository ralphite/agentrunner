// Lifecycle hooks (INC-15, G19 first batch): fire user-configured commands
// at the loop's journal points. Same doctrine as pre/post tool hooks (决策
// #11): hooks are pipeline machinery, not effects — observe + block only,
// never journaled as activities, never replayed on resume (a hook fires when
// its point is LIVE-crossed, not when the journal is re-read). Observe
// events fire AFTER their fact landed (the event is already true); blockable
// events fire BEFORE their action and an exit-2 vetoes just that action.
package agent

import (
	"context"
	"encoding/json"

	"github.com/ralphite/agentrunner/internal/hook"
	"github.com/ralphite/agentrunner/internal/protocol"
)

// fireLifecycle runs the hooks for one lifecycle event. Nil-safe: without a
// Runner (or with no hooks for the event) it is a zero-cost no-op. Notes
// surface on the live event stream only — they are operator feedback, not
// conversation content.
func (l *Loop) fireLifecycle(ctx context.Context, event string, detail any, blockable bool) hook.LifecycleResult {
	if l.Hooks == nil {
		return hook.LifecycleResult{}
	}
	var raw json.RawMessage
	if detail != nil {
		raw, _ = json.Marshal(detail)
	}
	res := l.Hooks.RunLifecycle(ctx, hook.LifecycleInput{
		Event: event, Session: l.SessionID, Detail: raw,
	}, blockable)
	for _, n := range res.Notes {
		l.emit(protocol.Event{Kind: protocol.KindMessage, Text: "hook: " + n})
	}
	return res
}
