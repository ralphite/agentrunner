package scripted

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/ralphite/agentrunner/internal/provider"
)

// Router serves concurrent agents (a parent and its sub-agents) each from
// its OWN scripted fixture, so a multi-agent test is DETERMINISTIC even
// though the children race (v2 M3.0, GAPS G4). Each route is keyed by a
// substring the request's conversation must contain (typically the agent's
// prompt text or a marker in its system prompt); the matching sub-provider
// keeps its own independent step counter.
//
// Routing is by first match in Routes order — put the most specific key
// first. A request matching no route yields an explicit error (never a
// silent wrong-script answer).
type Router struct {
	Routes []Route
}

// Route binds a conversation-substring key to a fixture.
type Route struct {
	Key string // matched against the request's system prompt + user messages
	P   *Provider
}

// NewRouter builds a router from (key, fixture) pairs, in match-priority order.
func NewRouter(pairs ...RoutePair) *Router {
	r := &Router{}
	for _, p := range pairs {
		r.Routes = append(r.Routes, Route{Key: p.Key, P: New(p.Fixture)})
	}
	return r
}

// RoutePair is one (key → fixture) binding for NewRouter.
type RoutePair struct {
	Key     string
	Fixture Fixture
}

func (r *Router) Capabilities() provider.Capabilities { return provider.Capabilities{} }

// Complete routes to the sub-provider whose Key appears in the request, then
// delegates (the sub-provider owns its own step counter, so each agent's
// script advances independently and deterministically under concurrency).
func (r *Router) Complete(ctx context.Context, req provider.CompleteRequest) iter.Seq2[provider.StreamEvent, error] {
	hay := routingHaystack(req)
	for _, rt := range r.Routes {
		if strings.Contains(hay, rt.Key) {
			return rt.P.Complete(ctx, req)
		}
	}
	return func(yield func(provider.StreamEvent, error) bool) {
		yield(provider.StreamEvent{}, fmt.Errorf(
			"scripted router: no route matched request (keys: %s)", r.keys()))
	}
}

func (r *Router) keys() string {
	var ks []string
	for _, rt := range r.Routes {
		ks = append(ks, rt.Key)
	}
	return strings.Join(ks, ", ")
}

// routingHaystack is the text a route key is matched against: the system
// prompt plus every user-role message's text.
func routingHaystack(req provider.CompleteRequest) string {
	var b strings.Builder
	b.WriteString(req.System)
	b.WriteByte('\n')
	for _, m := range req.Messages {
		if m.Role != provider.RoleUser {
			continue
		}
		for _, p := range m.Parts {
			if p.Kind == provider.PartText {
				b.WriteString(p.Text)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}
