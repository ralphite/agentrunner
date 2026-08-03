package agent

import (
	"context"

	"github.com/ralphite/agentrunner/internal/kill"
)

// killID scopes a call id to this tree member. Call ids are unique only
// WITHIN a session — a child's first tool call and its parent's first call
// are both "call_1_0" — so a tree-wide table keyed on the bare id lets the
// child's call silently displace the parent's handle FOR that child, and
// "stop this agent" then stops only whatever command it happened to be
// running. The root keeps bare ids, because that is what `ar ps` and the
// webui's background rows hand back; every deeper member is qualified.
func (l *Loop) killID(id string) string {
	if l.Depth == 0 || l.SessionID == "" {
		return id
	}
	return l.SessionID + "#" + id
}

// registerKill publishes one in-flight unit's cancel on the tree's live
// kill table: nil-safe, and it stamps the entry with THIS member's session
// id so a kill aimed at a subagent also reaches the calls that agent is
// running. The returned func unpublishes it.
func (l *Loop) registerKill(id, kind, name string, cancel context.CancelCauseFunc) func() {
	if l.Kills == nil {
		return func() {}
	}
	return l.Kills.Register(kill.Target{
		ID: l.killID(id), Kind: kind, Name: name, Session: l.SessionID,
	}, cancel)
}
