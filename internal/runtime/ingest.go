package runtime

import (
	"github.com/ralphite/agentrunner/internal/crash"
	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/store"
)

// IngestInput is the journal-inputs-first discipline (2.7): every external
// input becomes a durable InputReceived fact BEFORE anything consumes it.
// Callers act only on the returned (appended) envelope — never on the raw
// input — so a crash after this call loses nothing.
func IngestInput(s *store.EventStore, correlationID, text, source string) (event.Envelope, error) {
	commandID := event.NewCommandID()
	return IngestUserInput(s, correlationID, protocol.UserInput{
		Text: text, Principal: "local-user", Source: source, Trust: "local",
		TurnID: "turn-" + commandID, ItemID: "item-" + commandID,
	}, commandID)
}

// IngestOpeningInput journals an opening prompt WITH pre-materialized
// attachment parts (PLAN 5.5: `new --image/--file`, symmetric with send).
// Binary parts must already be CAS refs (blob-before-event); the text part
// leads the content exactly like journalInput's assembly.
func IngestOpeningInput(s *store.EventStore, correlationID, text, source string,
	content []provider.Part, images, files []event.AttachmentRef) (event.Envelope, error) {
	commandID := event.NewCommandID()
	env, err := event.New(event.TypeInputReceived, &event.InputReceived{
		Text: text, Source: source, TurnID: "turn-" + commandID, ItemID: "item-" + commandID,
		Principal: "local-user", Trust: "local",
		Content: content, Images: images, Files: files,
	})
	if err != nil {
		return event.Envelope{}, err
	}
	env.CausationID = commandID
	env.CorrelationID = correlationID
	env.Sender = source
	appended, err := s.Append(env)
	if err != nil {
		return event.Envelope{}, err
	}
	crash.Point(crash.PointAfterJournalInput)
	return appended, nil
}

// IngestUserInput persists a fully attributed typed input. Binary parts must
// already be CAS refs; the opening-prompt path uses text only, while daemon
// inputs are materialized by agent.journalInput.
func IngestUserInput(s *store.EventStore, correlationID string, in protocol.UserInput, commandID string) (event.Envelope, error) {
	content := in.Content
	if len(content) == 0 {
		content = []protocol.ContentPart{{Kind: provider.PartText, Text: in.Text}}
	}
	parts := make([]provider.Part, 0, len(content))
	for _, part := range content {
		parts = append(parts, provider.Part{Kind: part.Kind, Text: part.Text,
			MediaType: part.MediaType, Ref: part.Ref})
	}
	env, err := event.New(event.TypeInputReceived, &event.InputReceived{
		Text: in.Text, Source: in.Source, TurnID: in.TurnID, ItemID: in.ItemID,
		Principal: in.Principal, Trust: in.Trust, Content: parts,
	})
	if err != nil {
		return event.Envelope{}, err
	}
	// The raw input is a command; the journaled fact is caused by it. The
	// Append assigns the fact's own evt-<seq> id.
	env.CausationID = commandID
	env.CorrelationID = correlationID
	env.Sender = in.Source
	appended, err := s.Append(env)
	if err != nil {
		return event.Envelope{}, err
	}
	crash.Point(crash.PointAfterJournalInput)
	return appended, nil
}
