package model

import (
	"encoding/json"
	"time"
)

// Category represents a torrent category from the categories table.
type Category struct {
	ID        int64
	Name      string
	Slug      string
	ParentID  *int64
	ImageURL  *string
	SortOrder int
	// MetadataSchema is the category's own metadata field definitions, stored as
	// a JSONB array. See internal/metadata for the field shape and inheritance.
	MetadataSchema json.RawMessage
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
