// Package messagefork implements authoritative message-scoped continuation:
// resolve a canonical timeline item to its anchored CheckpointBarrier, then
// publish a dormant, independent child session exactly once per request id.
package messagefork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/ralphite/agentrunner/internal/event"
	"github.com/ralphite/agentrunner/internal/fork"
	"github.com/ralphite/agentrunner/internal/protocol"
	"github.com/ralphite/agentrunner/internal/provider"
	"github.com/ralphite/agentrunner/internal/runtime"
	"github.com/ralphite/agentrunner/internal/snapshot"
	"github.com/ralphite/agentrunner/internal/state"
	"github.com/ralphite/agentrunner/internal/store"
)

const (
	SideBeforeUser     = "before_user"
	SideAfterAssistant = "after_assistant"
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

var (
	ErrNotFound = errors.New("message continuation target not found")
	ErrConflict = errors.New("message continuation request conflict")
	ErrInvalid  = errors.New("message continuation request invalid")
)

type Request struct {
	ParentSession string `json:"parent_session"`
	ItemID        string `json:"item_id"`
	RequestID     string `json:"request_id"`
}

type Result struct {
	SessionID    string           `json:"session_id"`
	SourceItemID string           `json:"source_item_id"`
	SourceSide   string           `json:"source_side"`
	Draft        *event.ForkDraft `json:"draft,omitempty"`
	Created      bool             `json:"-"`
}

// PendingDraft returns the durable composer seed while the child remains in
// its dedicated fork park. Once a human input clears the park, ok is false.
func PendingDraft(sessionID string) (draft *event.ForkDraft, ok bool, err error) {
	data, err := runtime.DataDir()
	if err != nil {
		return nil, false, err
	}
	dir, err := ResolveSessionDir(filepath.Join(data, "sessions"), sessionID)
	if err != nil {
		return nil, false, err
	}
	events, err := store.ReadEvents(dir)
	if err != nil {
		return nil, false, err
	}
	folded, err := state.Fold(events)
	if err != nil {
		return nil, false, err
	}
	if folded.ForkPark == nil || folded.Session.ForkedFrom == nil || folded.Session.ForkedFrom.Draft == nil {
		return nil, false, nil
	}
	if folded.ForkPark.DraftID != folded.Session.ForkedFrom.Draft.DraftID {
		return nil, false, fmt.Errorf("message fork: draft/park mismatch")
	}
	return folded.Session.ForkedFrom.Draft, true, nil
}

type DraftPartRequest struct {
	Kind provider.PartKind `json:"kind"`
	Ref  string            `json:"ref"`
	// Ordinal is the zero-based position among source draft attachments. It
	// disambiguates duplicate kind/ref entries with different metadata.
	Ordinal int `json:"ordinal"`
}

type AuthorizedDraftPart struct {
	Kind      provider.PartKind
	Text      string
	Path      string
	MediaType string
	Name      string
	PartID    string
}

// AuthorizeDraftParts proves kind/ref/multiplicity/order against this child's
// pending draft and returns authoritative metadata plus child-local paths.
// exact=true replays the recorded Content wholesale; edited drafts receive a
// canonical text part in the Web layer followed by the selected attachments.
func AuthorizeDraftParts(sessionID, draftID, text string, requested []DraftPartRequest,
	exact bool) ([]AuthorizedDraftPart, string, error) {
	draft, ok, err := PendingDraft(sessionID)
	if err != nil {
		return nil, "", err
	}
	if !ok || draft.DraftID != draftID {
		return nil, "", fmt.Errorf("%w: draft is not pending", ErrConflict)
	}
	source := append([]provider.Part(nil), draft.Content...)
	if len(source) == 0 {
		if draft.Text != "" {
			source = append(source, provider.Part{Kind: provider.PartText, Text: draft.Text})
		}
		for _, p := range draft.Images {
			source = append(source, provider.Part{Kind: provider.PartImage, Ref: p.Ref,
				MediaType: p.MediaType, Name: p.Name, PartID: p.PartID})
		}
		for _, p := range draft.Files {
			source = append(source, provider.Part{Kind: provider.PartFile, Ref: p.Ref,
				MediaType: p.MediaType, Name: p.Name, PartID: p.PartID})
		}
	}
	attachments := make([]provider.Part, 0, len(source))
	for _, p := range source {
		if p.Kind == provider.PartImage || p.Kind == provider.PartFile {
			attachments = append(attachments, p)
		}
	}
	if exact {
		if text != draft.Text || len(requested) != len(attachments) {
			return nil, "", fmt.Errorf("%w: exact draft replay does not match recorded content", ErrInvalid)
		}
		for i, req := range requested {
			if req.Ordinal != i || req.Kind != attachments[i].Kind || req.Ref != attachments[i].Ref {
				return nil, "", fmt.Errorf("%w: exact draft attachment order changed", ErrInvalid)
			}
		}
	}
	data, err := runtime.DataDir()
	if err != nil {
		return nil, "", err
	}
	dir, err := ResolveSessionDir(filepath.Join(data, "sessions"), sessionID)
	if err != nil {
		return nil, "", err
	}
	events, err := store.ReadEventPrefix(dir, 1)
	if err != nil || len(events) != 1 {
		return nil, "", fmt.Errorf("message fork: cannot read child genesis")
	}
	decoded, err := event.DecodePayload(events[0])
	if err != nil {
		return nil, "", err
	}
	origin := decoded.(*event.ForkedFrom)
	toAuthorized := func(p provider.Part) (AuthorizedDraftPart, error) {
		out := AuthorizedDraftPart{Kind: p.Kind, Text: p.Text, MediaType: p.MediaType,
			Name: p.Name, PartID: p.PartID}
		if p.Ref != "" {
			out.Path = filepath.Join(dir, "artifacts", "blobs", p.Ref)
			if _, err := os.Stat(out.Path); err != nil {
				return AuthorizedDraftPart{}, err
			}
		}
		return out, nil
	}
	var selected []provider.Part
	if exact {
		selected = source
	} else {
		used := make([]bool, len(attachments))
		for _, req := range requested {
			if req.Ordinal < 0 || req.Ordinal >= len(attachments) {
				return nil, "", fmt.Errorf("%w: attachment ordinal is outside the pending draft", ErrInvalid)
			}
			p := attachments[req.Ordinal]
			if p.Kind != req.Kind || p.Ref != req.Ref {
				return nil, "", fmt.Errorf("%w: attachment identity does not match the pending draft", ErrInvalid)
			}
			if used[req.Ordinal] {
				return nil, "", fmt.Errorf("%w: duplicate attachment ordinal", ErrInvalid)
			}
			used[req.Ordinal] = true
			selected = append(selected, p)
		}
	}
	out := make([]AuthorizedDraftPart, 0, len(selected))
	for _, p := range selected {
		authorized, err := toAuthorized(p)
		if err != nil {
			return nil, "", err
		}
		out = append(out, authorized)
	}
	return out, origin.SourceItemID, nil
}

type target struct {
	barrier state.Barrier
	side    string
	draft   *event.ForkDraft
}

type registry struct {
	RequestID   string `json:"request_id"`
	PayloadHash string `json:"payload_hash"`
	ChildID     string `json:"child_id"`
	Workspace   string `json:"workspace"`
	State       string `json:"state"`
}

// Continue resolves parent exactly (top-level or nested full session id) and
// atomically publishes one dormant top-level child.
func Continue(ctx context.Context, req Request) (Result, error) {
	if !requestIDPattern.MatchString(req.RequestID) || req.ItemID == "" ||
		!runtime.ValidSessionID(req.ParentSession) {
		return Result{}, fmt.Errorf("%w: invalid parent, item_id, or request_id", ErrInvalid)
	}
	data, err := runtime.DataDir()
	if err != nil {
		return Result{}, err
	}
	parentDir, err := ResolveSessionDir(filepath.Join(data, "sessions"), req.ParentSession)
	if err != nil {
		return Result{}, err
	}
	return continueLocked(ctx, data, parentDir, req)
}

func continueLocked(ctx context.Context, data, parentDir string, req Request) (Result, error) {
	key := sha256.Sum256([]byte(req.RequestID))
	keyText := hex.EncodeToString(key[:])
	registryDir := filepath.Join(data, "continue-requests")
	if err := os.MkdirAll(registryDir, 0o700); err != nil {
		return Result{}, err
	}
	lock, err := os.OpenFile(filepath.Join(registryDir, keyText+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Result{}, err
	}

	payload := sha256.Sum256([]byte(req.ParentSession + "\x00" + req.ItemID))
	payloadHash := hex.EncodeToString(payload[:])
	registryPath := filepath.Join(registryDir, keyText+".json")
	reg, exists, err := readRegistry(registryPath)
	if err != nil {
		return Result{}, err
	}
	if exists && (reg.RequestID != req.RequestID || reg.PayloadHash != payloadHash) {
		return Result{}, fmt.Errorf("%w: request_id already belongs to another target", ErrConflict)
	}

	events, err := store.ReadEvents(parentDir)
	if err != nil {
		return Result{}, err
	}
	t, err := resolve(events, req.ItemID)
	if err != nil {
		return Result{}, err
	}
	if exists && reg.State == "session_published" {
		if err := validatePublished(data, reg, req); err != nil {
			return Result{}, err
		}
		return Result{SessionID: reg.ChildID, SourceItemID: req.ItemID,
			SourceSide: t.side, Draft: t.draft}, nil
	}

	workspaceRoot, err := sessionWorkspace(events)
	if err != nil {
		return Result{}, err
	}
	if !exists {
		childID := runtime.NewSessionID(time.Now(), "continue "+req.ItemID)
		reg = registry{RequestID: req.RequestID, PayloadHash: payloadHash,
			ChildID: childID, Workspace: workspaceRoot + "-continue-" + childID[len(childID)-4:],
			State: "reserved"}
		if err := writeRegistry(registryPath, reg); err != nil {
			return Result{}, err
		}
	}

	sessionsRoot := filepath.Join(data, "sessions")
	finalDir := filepath.Join(sessionsRoot, reg.ChildID)
	if validPublished(finalDir, req) {
		reg.State = "session_published"
		if err := writeRegistry(registryPath, reg); err != nil {
			return Result{}, err
		}
		return Result{SessionID: reg.ChildID, SourceItemID: req.ItemID,
			SourceSide: t.side, Draft: t.draft}, nil
	}

	parentShadow, err := openShadow(data, workspaceRoot)
	if err != nil {
		return Result{}, fmt.Errorf("parent snapshot store: %w", err)
	}
	if _, err := os.Stat(reg.Workspace); os.IsNotExist(err) {
		workspaceStage := reg.Workspace + ".staging-" + keyText[:12]
		_ = os.RemoveAll(workspaceStage) // this request's private, unpublished temp
		if err := parentShadow.Materialize(ctx, t.barrier.SnapshotRef, workspaceStage); err != nil {
			return Result{}, err
		}
		if err := os.Rename(workspaceStage, reg.Workspace); err != nil {
			return Result{}, err
		}
	} else if err != nil {
		return Result{}, err
	}
	reg.State = "workspace_ready"
	if err := writeRegistry(registryPath, reg); err != nil {
		return Result{}, err
	}

	stageRoot := filepath.Join(sessionsRoot, ".staging")
	stageDir := filepath.Join(stageRoot, keyText)
	if err := os.MkdirAll(stageRoot, 0o700); err != nil {
		return Result{}, err
	}
	_ = os.RemoveAll(stageDir) // only this locked request's hidden staging dir
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return Result{}, err
	}
	draftID := ""
	if t.draft != nil {
		draftID = t.draft.DraftID
	}
	refs, err := fork.Cut(fork.Options{
		ParentDir: parentDir, ParentSession: req.ParentSession,
		NewDir: stageDir, NewSession: reg.ChildID, Barrier: t.barrier,
		WorkspaceRoot: reg.Workspace, Now: time.Now(),
		GenesisMeta: &event.ForkedFrom{RequestID: req.RequestID,
			SourceItemID: req.ItemID, SourceSide: t.side, Draft: t.draft},
	})
	if err != nil {
		return Result{}, err
	}
	if childShadow, serr := openShadow(data, reg.Workspace); serr != nil {
		return Result{}, serr
	} else if serr := parentShadow.PushRefs(ctx, childShadow.GitDir(), refs); serr != nil {
		return Result{}, serr
	}
	if err := normalizeDormant(stageDir, reg.ChildID, req.RequestID, draftID); err != nil {
		return Result{}, err
	}
	if err := verifyDraftRefs(stageDir, t.draft); err != nil {
		return Result{}, err
	}
	if err := syncDir(stageDir); err != nil {
		return Result{}, err
	}
	if _, err := os.Stat(finalDir); err == nil {
		if !validPublished(finalDir, req) {
			return Result{}, fmt.Errorf("%w: child id already exists", ErrConflict)
		}
	} else if !os.IsNotExist(err) {
		return Result{}, err
	} else if err := os.Rename(stageDir, finalDir); err != nil {
		return Result{}, err
	}
	if err := syncDir(sessionsRoot); err != nil {
		return Result{}, err
	}
	reg.State = "session_published"
	if err := writeRegistry(registryPath, reg); err != nil {
		return Result{}, err
	}
	return Result{SessionID: reg.ChildID, SourceItemID: req.ItemID,
		SourceSide: t.side, Draft: t.draft, Created: true}, nil
}

func resolve(events []event.Envelope, itemID string) (target, error) {
	var (
		message event.Envelope
		side    string
		draft   *event.ForkDraft
		found   int
	)
	for _, env := range events {
		switch env.Type {
		case event.TypeInputReceived:
			decoded, err := event.DecodePayload(env)
			if err != nil {
				return target{}, err
			}
			p := decoded.(*event.InputReceived)
			if p.ItemID != itemID {
				continue
			}
			found++
			if !protocol.UserClassSource(p.Source) {
				return target{}, fmt.Errorf("%w: input is not human", ErrInvalid)
			}
			message, side = env, SideBeforeUser
			draft = draftFromInput(p)
		case event.TypeAssistantMessage:
			decoded, err := event.DecodePayload(env)
			if err != nil {
				return target{}, err
			}
			p := decoded.(*event.AssistantMessage)
			if p.ItemID != itemID {
				continue
			}
			found++
			if p.ContinuationCheckpoint == nil || !visibleAssistant(p.Message) {
				return target{}, fmt.Errorf("%w: assistant message is not loop-final", ErrInvalid)
			}
			message, side = env, SideAfterAssistant
		}
	}
	if found == 0 {
		return target{}, ErrNotFound
	}
	if found != 1 {
		return target{}, fmt.Errorf("%w: duplicate item_id", ErrInvalid)
	}
	folded, err := state.Fold(events)
	if err != nil {
		return target{}, err
	}
	var matches []state.Barrier
	for _, b := range folded.Barriers {
		if b.MessageAnchor != nil && b.MessageAnchor.Side == side && b.MessageAnchor.ItemID == itemID {
			matches = append(matches, b)
		}
	}
	if len(matches) != 1 || matches[0].SnapshotRef == "" {
		return target{}, fmt.Errorf("%w: no unique materializable message anchor", ErrInvalid)
	}
	b := matches[0]
	if side == SideBeforeUser && b.Seq >= message.Seq {
		return target{}, fmt.Errorf("%w: before-user anchor ordering", ErrInvalid)
	}
	if side == SideAfterAssistant && b.Seq != message.Seq+1 {
		return target{}, fmt.Errorf("%w: after-assistant anchor ordering", ErrInvalid)
	}
	return target{barrier: b, side: side, draft: draft}, nil
}

func visibleAssistant(msg provider.Message) bool {
	for _, p := range msg.Parts {
		if p.Kind == provider.PartToolCall {
			return false
		}
		if p.Kind == provider.PartText && strings.TrimSpace(p.Text) != "" {
			return true
		}
	}
	return false
}

func draftFromInput(in *event.InputReceived) *event.ForkDraft {
	d := &event.ForkDraft{DraftID: "draft-" + in.ItemID, Text: in.Text,
		Content: append([]provider.Part(nil), in.Content...),
		Images:  append([]event.AttachmentRef(nil), in.Images...),
		Files:   append([]event.AttachmentRef(nil), in.Files...)}
	for i := range d.Content {
		d.Content[i].Data = nil
		d.Content[i].Name = cleanName(d.Content[i].Name)
	}
	for i := range d.Images {
		d.Images[i].Name = cleanName(d.Images[i].Name)
	}
	for i := range d.Files {
		d.Files[i].Name = cleanName(d.Files[i].Name)
	}
	return d
}

func cleanName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if len(name) > 160 {
		name = name[:160]
		for len(name) > 0 && !utf8.ValidString(name) {
			name = name[:len(name)-1]
		}
	}
	return name
}

func normalizeDormant(dir, sessionID, requestID, draftID string) error {
	events, err := store.ReadEvents(dir)
	if err != nil {
		return err
	}
	folded, err := state.Fold(events)
	if err != nil {
		return err
	}
	es, err := store.OpenEventStore(dir)
	if err != nil {
		return err
	}
	defer func() { _ = es.Close() }()
	appendFact := func(typ string, payload any) error {
		env, err := event.New(typ, payload)
		if err != nil {
			return err
		}
		env.CorrelationID, env.CausationID, env.Sender, env.Target = sessionID, event.EventID(es.LastSeq()), "message_fork", "session"
		_, err = es.Append(env)
		return err
	}
	for id := range folded.Timers {
		if err := appendFact(event.TypeTimerCancelled, &event.TimerCancelled{TimerID: id}); err != nil {
			return err
		}
	}
	if folded.Goal != nil && !folded.Goal.Paused {
		if err := appendFact(event.TypeGoalPaused, &event.GoalPaused{GoalID: folded.Goal.GoalID, Source: "message_fork"}); err != nil {
			return err
		}
	}
	if folded.Schedule != nil && !folded.Schedule.Paused {
		if err := appendFact(event.TypeSchedulePaused, &event.SchedulePaused{ScheduleID: folded.Schedule.ScheduleID, Source: "message_fork"}); err != nil {
			return err
		}
	}
	if err := appendFact(event.TypeForkAwaitingInput, &event.ForkAwaitingInput{RequestID: requestID, DraftID: draftID}); err != nil {
		return err
	}
	finalEvents, err := store.ReadEvents(dir)
	if err != nil {
		return err
	}
	final, err := state.Fold(finalEvents)
	if err != nil {
		return err
	}
	if len(final.Handles) != 0 || len(final.Timers) != 0 || final.ForkPark == nil {
		return fmt.Errorf("message fork: dormant normalization failed")
	}
	return nil
}

func verifyDraftRefs(dir string, draft *event.ForkDraft) error {
	if draft == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, p := range draft.Content {
		if p.Ref != "" {
			seen[p.Ref] = true
		}
	}
	for _, p := range draft.Images {
		seen[p.Ref] = true
	}
	for _, p := range draft.Files {
		seen[p.Ref] = true
	}
	for ref := range seen {
		if !regexp.MustCompile(`^sha256-[0-9a-f]{64}$`).MatchString(ref) {
			return fmt.Errorf("message fork: invalid draft ref")
		}
		raw, err := os.ReadFile(filepath.Join(dir, "artifacts", "blobs", ref))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		if "sha256-"+hex.EncodeToString(sum[:]) != ref {
			return fmt.Errorf("message fork: draft blob hash mismatch for %s", ref)
		}
	}
	return nil
}

func sessionWorkspace(events []event.Envelope) (string, error) {
	var root string
	for _, env := range events {
		switch env.Type {
		case event.TypeForkedFrom:
			decoded, err := event.DecodePayload(env)
			if err != nil {
				return "", err
			}
			if v := decoded.(*event.ForkedFrom).WorkspaceRoot; v != "" {
				root = v
			}
		case event.TypeSessionStarted:
			decoded, err := event.DecodePayload(env)
			if err != nil {
				return "", err
			}
			if root == "" {
				root = decoded.(*event.SessionStarted).WorkspaceRoot
			}
		}
	}
	if root == "" {
		return "", fmt.Errorf("%w: session has no resumable workspace", ErrInvalid)
	}
	return root, nil
}

func shadowDir(data, root string) string {
	h := sha256.Sum256([]byte(root))
	return filepath.Join(data, "shadow", hex.EncodeToString(h[:6]))
}

func openShadow(data, root string) (*snapshot.ShadowRepo, error) {
	st, err := snapshot.Open(shadowDir(data, root), root)
	if err != nil {
		return nil, err
	}
	repo, ok := st.(*snapshot.ShadowRepo)
	if !ok {
		return nil, fmt.Errorf("snapshot backend unavailable")
	}
	return repo, nil
}

// ResolveSessionDir resolves an exact top-level or nested full session id.
func ResolveSessionDir(sessionsRoot, id string) (string, error) {
	if !runtime.ValidSessionID(id) {
		return "", ErrNotFound
	}
	if validSession(filepath.Join(sessionsRoot, id)) {
		return filepath.Join(sessionsRoot, id), nil
	}
	for i := strings.LastIndex(id, "-sub-"); i >= 0; i = strings.LastIndex(id[:i], "-sub-") {
		parent, rest := id[:i], id[i+len("-sub-"):]
		dir := filepath.Join(sessionsRoot, parent, "sub", strings.ReplaceAll(rest, "-sub-", "/sub/"))
		if validSession(dir) {
			return dir, nil
		}
	}
	return "", ErrNotFound
}

func validSession(dir string) bool {
	events, err := store.ReadEventPrefix(dir, 2)
	if err != nil || len(events) == 0 {
		return false
	}
	return events[0].Type == event.TypeSessionStarted || events[0].Type == event.TypeForkedFrom
}

func validPublished(dir string, req Request) bool {
	events, err := store.ReadEventPrefix(dir, 1)
	if err != nil || len(events) != 1 || events[0].Type != event.TypeForkedFrom {
		return false
	}
	decoded, err := event.DecodePayload(events[0])
	if err != nil {
		return false
	}
	p := decoded.(*event.ForkedFrom)
	return p.RequestID == req.RequestID && p.ParentSession == req.ParentSession && p.SourceItemID == req.ItemID
}

func validatePublished(data string, reg registry, req Request) error {
	if !validPublished(filepath.Join(data, "sessions", reg.ChildID), req) {
		return fmt.Errorf("message fork: registry points at invalid child")
	}
	return nil
}

func readRegistry(path string) (registry, bool, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return registry{}, false, nil
	}
	if err != nil {
		return registry{}, false, err
	}
	var reg registry
	if err := json.Unmarshal(raw, &reg); err != nil {
		return registry{}, false, fmt.Errorf("message fork registry corrupt: %w", err)
	}
	if reg.RequestID == "" || reg.PayloadHash == "" || reg.ChildID == "" {
		return registry{}, false, fmt.Errorf("message fork registry corrupt")
	}
	return reg, true, nil
}

func writeRegistry(path string, reg registry) error {
	raw, err := json.Marshal(reg)
	if err != nil {
		return err
	}
	tmp := path + ".tmp-" + fmt.Sprint(os.Getpid())
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	f, err := os.Open(tmp)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}
