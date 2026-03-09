package model

import "time"

// Category represents a torrent category from the categories table.
type Category struct {
	ID        int64
	Name      string
	Slug      string
	ParentID  *int64
	ImageURL  *string
	SortOrder int
	CreatedAt time.Time
	UpdatedAt time.Time
}
