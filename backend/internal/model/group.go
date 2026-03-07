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
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
