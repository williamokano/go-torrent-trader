package model

import "time"

// Group represents a user permission group from the groups table.
type Group struct {
	ID          int64
	Name        string
	Slug        string
	Level       int
	Color       *string
	CanUpload   bool
	CanDownload bool
	CanInvite   bool
	CanComment  bool
	CanForum    bool
	IsAdmin     bool
	IsModerator bool
	IsImmune    bool
	// CanSelfApprove marks the Uploader class (BE-8.22c): members may approve
	// their own torrent submissions.
	CanSelfApprove bool
	// CanFeed lets members of this class watch the live release feeds.
	CanFeed   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
