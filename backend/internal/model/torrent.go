package model

import (
	"encoding/json"
	"time"
)

// TorrentFile represents a single file inside a torrent.
type TorrentFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Torrent moderation status values (BE-8.22). A newly uploaded torrent is
// ModerationPending until a staff member (or a self-approving Uploader on their
// own upload) approves it; only then is it public and downloadable.
const (
	ModerationPending  = "pending"
	ModerationApproved = "approved"
	ModerationRejected = "rejected"
)

// ModerationRestricted reports whether the torrent's moderation status subjects it
// to access control (hidden from public lists, undownloadable/unannounceable by
// non-uploader/non-staff). Only explicitly pending or rejected torrents are
// restricted; an approved status — or, defensively, an unset one — is public.
func (t *Torrent) ModerationRestricted() bool {
	return t.ModerationStatus == ModerationPending || t.ModerationStatus == ModerationRejected
}

// Torrent represents a torrent file registered in the tracker.
type Torrent struct {
	ID               int64
	Name             string
	InfoHash         []byte
	Size             int64
	Description      *string
	Nfo              *string
	CategoryID       int64
	CategoryName     string
	CategoryImageURL *string
	UploaderID       int64
	Anonymous        bool
	Seeders          int
	Leechers         int
	TimesCompleted   int
	CommentsCount    int
	Visible          bool
	Banned           bool
	Free             bool
	Silver           bool
	// HnRExempt marks this torrent as never generating hit-and-run
	// obligations, staff-settable in the same shape as Free/Silver. Set
	// after a snatch, it resolves any open hnr_records for this torrent as
	// waived; unsetting it does not resurrect them (see migration 081).
	HnRExempt        bool
	FileCount        int
	Files            *json.RawMessage // JSONB array of TorrentFile, nullable
	Metadata         json.RawMessage  // JSONB object of category-schema field values (defaults to {})
	UploaderName     string           // Resolved via JOIN; "Anonymous" when anonymous=true
	UploaderWarned   bool             // Resolved via JOIN; false when anonymous=true
	// Moderation (BE-8.22). ModerationStatus is one of the Moderation* constants.
	ModerationStatus      string
	AssignedModeratorID   *int64 // staff member who claimed the review; nil when unassigned
	AssignedModeratorName string // Resolved via JOIN; empty when unassigned
	ApprovedBy            *int64 // who approved it; nil while pending/rejected
	ApprovedByName        string // Resolved via JOIN; empty until approved
	ApprovedAt            *time.Time
	// MessageCount is 0 until BE-8.22b, which populates it from the
	// moderation-queue query; earlier code paths always leave it 0.
	MessageCount int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
