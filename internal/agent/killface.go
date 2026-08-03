package agent

import (
	"context"

	"github.com/ralphite/agentrunner/internal/kill"
)

// registerKill publishes one in-flight unit's cancel on the tree's live
// kill table: nil-safe, and it stamps the entry with THIS member's session
// id so a kill aimed at a subagent also reaches the calls that agent is
// running. The returned func unpublishes it.
func (l *Loop) registerKill(id, kind, name string, cancel context.CancelCauseFunc) func() {
	if l.Kills == nil {
		return func() {}
	}
	return l.Kills.Register(kill.Target{
		ID: id, Kind: kind, Name: name, Session: l.SessionID,
	}, cancel)
}
