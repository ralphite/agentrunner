package runtime

import (
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/store"
)

// IngestInput is the journal-inputs-first discipline (2.7): every external
// input becomes a durable InputReceived fact BEFORE anything consumes it.
// Callers act only on the returned (appended) envelope — never on the raw
// input — so a crash after this call loses nothing.
func IngestInput(s *store.EventStore, correlationID, text, source string) (event.Envelope, error) {
	env, err := event.New(event.TypeInputReceived, &event.InputReceived{Text: text, Source: source})
	if err != nil {
		return event.Envelope{}, err
	}
	// The raw input is a command; the journaled fact is caused by it. The
	// Append assigns the fact's own evt-<seq> id.
	env.CausationID = event.NewCommandID()
	env.CorrelationID = correlationID
	env.Sender = source
	appended, err := s.Append(env)
	if err != nil {
		return event.Envelope{}, err
	}
	return appended, nil
}
