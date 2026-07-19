package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrSavedMessageNotFound = errors.New("saved message not found")
	ErrInvalidSavedMessage  = errors.New("invalid saved message")
)

// SavedMessageConflictError is returned by Update when the caller's
// expectedVersion no longer matches what's stored — another save (a
// different tab, a different device) landed first since the caller last
// read this draft/template. Current is the row as it stands right now
// (including its up-to-date Version), so a handler can hand it straight
// back to the client in a 409 response without a second lookup.
type SavedMessageConflictError struct {
	Current *model.SavedMessage
}

func (e *SavedMessageConflictError) Error() string {
	return fmt.Sprintf("saved message %d: version conflict, current version is %d", e.Current.ID, e.Current.Version)
}

// SaveMessageRequest holds the data needed to create or update a draft or
// template. ReceiverID/ParentID only apply to drafts — a template is never
// addressed to anyone, so build() rejects them for that kind.
type SaveMessageRequest struct {
	ReceiverID *int64 `json:"receiver_id,omitempty"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	ParentID   *int64 `json:"parent_id,omitempty"`
}

// SavedMessageService handles business logic for PM drafts and templates.
// Both kinds share the same repository/table (model.SavedMessage) and the
// same validation entry point (build); only the rules inside build differ.
type SavedMessageService struct {
	saved    repository.SavedMessageRepository
	users    repository.UserRepository
	messages repository.MessageRepository
}

// NewSavedMessageService creates a new SavedMessageService.
func NewSavedMessageService(
	saved repository.SavedMessageRepository,
	users repository.UserRepository,
	messages repository.MessageRepository,
) *SavedMessageService {
	return &SavedMessageService{saved: saved, users: users, messages: messages}
}

// Save creates a new draft or template for the given user.
func (s *SavedMessageService) Save(ctx context.Context, userID int64, kind model.SavedMessageKind, req SaveMessageRequest) (*model.SavedMessage, error) {
	sm, err := s.build(ctx, userID, kind, req)
	if err != nil {
		return nil, err
	}
	if err := s.saved.Create(ctx, sm); err != nil {
		return nil, fmt.Errorf("create saved message: %w", err)
	}
	return s.getOwned(ctx, sm.ID, userID)
}

// Update overwrites an existing draft or template. The caller must own it and
// it must already be of the given kind (a draft cannot silently become a
// template through the templates endpoint, and vice versa).
//
// expectedVersion must be the Version the caller last read (e.g. from the
// response of a prior Save/Update/Get/List call). It's compared against the
// row's current version by a conditional UPDATE in the repository — see
// SavedMessageConflictError — so two overlapping edits of the same
// draft/template (two tabs, two devices) can't silently overwrite each
// other: whichever save lands second gets a conflict instead of clobbering
// the first.
func (s *SavedMessageService) Update(ctx context.Context, id, userID int64, kind model.SavedMessageKind, expectedVersion int64, req SaveMessageRequest) (*model.SavedMessage, error) {
	existing, err := s.getOwnedOfKind(ctx, id, userID, kind)
	if err != nil {
		return nil, err
	}

	built, err := s.build(ctx, userID, kind, req)
	if err != nil {
		return nil, err
	}
	built.ID = existing.ID
	built.Version = expectedVersion

	if err := s.saved.Update(ctx, built); err != nil {
		var conflict *repository.SavedMessageConflictError
		if errors.As(err, &conflict) {
			return nil, &SavedMessageConflictError{Current: conflict.Current}
		}
		// getOwnedOfKind just confirmed this row exists and is owned by
		// userID, but if it was deleted in between (a narrow TOCTOU window),
		// Update's own ownership-scoped WHERE clause (see SavedMessageRepo.
		// Update) means it now matches zero rows too — surface that as a 404,
		// not a 500, the same way Delete already does below.
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSavedMessageNotFound
		}
		return nil, fmt.Errorf("update saved message: %w", err)
	}
	return s.getOwned(ctx, id, userID)
}

// Get retrieves a single draft or template. The caller must own it.
func (s *SavedMessageService) Get(ctx context.Context, id, userID int64, kind model.SavedMessageKind) (*model.SavedMessage, error) {
	return s.getOwnedOfKind(ctx, id, userID, kind)
}

// List returns paginated drafts or templates for a user.
func (s *SavedMessageService) List(ctx context.Context, userID int64, kind model.SavedMessageKind, page, perPage int) ([]model.SavedMessage, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	return s.saved.ListByUser(ctx, userID, kind, page, perPage)
}

// Delete removes a draft or template. The caller must own it.
func (s *SavedMessageService) Delete(ctx context.Context, id, userID int64, kind model.SavedMessageKind) error {
	if _, err := s.getOwnedOfKind(ctx, id, userID, kind); err != nil {
		return err
	}
	if err := s.saved.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSavedMessageNotFound
		}
		return fmt.Errorf("delete saved message: %w", err)
	}
	return nil
}

// build validates a request and turns it into a model.SavedMessage, applying
// kind-specific rules:
//   - template: reusable and recipient-less by definition, so receiver_id and
//     parent_id are rejected, and body must be non-empty (an empty template
//     isn't a "pattern" worth saving).
//   - draft: deliberately incomplete, so subject/body may be blank, but a
//     wholly empty draft (no subject, body, or receiver) is a no-op, not a
//     save. If a receiver or parent is given, it's validated the same way
//     SendMessage validates them.
func (s *SavedMessageService) build(ctx context.Context, userID int64, kind model.SavedMessageKind, req SaveMessageRequest) (*model.SavedMessage, error) {
	subject := strings.TrimSpace(req.Subject)
	body := strings.TrimSpace(req.Body)

	switch kind {
	case model.SavedMessageKindTemplate:
		if req.ReceiverID != nil {
			return nil, fmt.Errorf("%w: templates cannot have a recipient", ErrInvalidSavedMessage)
		}
		if req.ParentID != nil {
			return nil, fmt.Errorf("%w: templates cannot be a reply", ErrInvalidSavedMessage)
		}
		if body == "" {
			return nil, fmt.Errorf("%w: template body cannot be empty", ErrInvalidSavedMessage)
		}
	case model.SavedMessageKindDraft:
		if subject == "" && body == "" && req.ReceiverID == nil {
			return nil, fmt.Errorf("%w: draft cannot be entirely empty", ErrInvalidSavedMessage)
		}
		if req.ReceiverID != nil {
			if _, err := s.users.GetByID(ctx, *req.ReceiverID); err != nil {
				return nil, fmt.Errorf("%w: receiver not found", ErrInvalidSavedMessage)
			}
		}
		if req.ParentID != nil {
			parent, err := s.messages.GetByID(ctx, *req.ParentID)
			if err != nil {
				return nil, fmt.Errorf("%w: parent message not found", ErrInvalidSavedMessage)
			}
			if parent.SenderID != userID && parent.ReceiverID != userID {
				return nil, fmt.Errorf("%w: you are not a participant in the parent conversation", ErrInvalidSavedMessage)
			}
		}
	default:
		return nil, fmt.Errorf("%w: unknown kind %q", ErrInvalidSavedMessage, kind)
	}

	return &model.SavedMessage{
		UserID:     userID,
		Kind:       kind,
		ReceiverID: req.ReceiverID,
		Subject:    subject,
		Body:       body,
		ParentID:   req.ParentID,
	}, nil
}

func (s *SavedMessageService) getOwned(ctx context.Context, id, userID int64) (*model.SavedMessage, error) {
	sm, err := s.saved.GetByID(ctx, id)
	if err != nil {
		return nil, ErrSavedMessageNotFound
	}
	if sm.UserID != userID {
		return nil, ErrSavedMessageNotFound
	}
	return sm, nil
}

func (s *SavedMessageService) getOwnedOfKind(ctx context.Context, id, userID int64, kind model.SavedMessageKind) (*model.SavedMessage, error) {
	sm, err := s.getOwned(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if sm.Kind != kind {
		return nil, ErrSavedMessageNotFound
	}
	return sm, nil
}
