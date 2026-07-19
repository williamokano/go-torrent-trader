package model

import "time"

// SavedMessageKind discriminates a private-message draft (a specific
// in-progress message, optionally tied to a recipient/reply, saved to finish
// later) from a template (a reusable pattern with no fixed recipient, loaded
// to start a new compose). Both share an identical shape, so they live in one
// table (saved_messages) with this column deciding which they are — see
// migrations/060_create_saved_messages.sql for the reasoning.
type SavedMessageKind string

const (
	SavedMessageKindDraft    SavedMessageKind = "draft"
	SavedMessageKindTemplate SavedMessageKind = "template"
)

// SavedMessage represents a saved PM draft or template belonging to a user.
type SavedMessage struct {
	ID               int64
	UserID           int64
	Kind             SavedMessageKind
	ReceiverID       *int64
	ReceiverUsername string // from JOIN; only meaningful for drafts with a receiver set
	Subject          string
	Body             string
	ParentID         *int64
	// Version is a monotonic counter incremented on every successful update
	// (migrations/064_add_version_to_saved_messages.sql). Callers updating a
	// saved message must send back the version they last read; the repo's
	// conditional UPDATE uses it to detect a lost-update race (see
	// SavedMessageRepository.Update and SavedMessageConflictError).
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}
