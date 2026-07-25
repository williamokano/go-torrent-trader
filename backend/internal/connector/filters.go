package connector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// Filters narrow which announcements an instance receives. Every field is
// additive and an empty one means "no opinion", so the zero value matches
// everything — which is what a freshly created instance gets.
type Filters struct {
	// EventTypes limits delivery to these Announcement.Event values.
	EventTypes []string `json:"event_types"`
	// CategoryIDs limits delivery to torrents in these categories.
	CategoryIDs []int64 `json:"category_ids"`
	// MinSize drops announcements for torrents smaller than this many bytes.
	// A torrent exactly at the threshold passes.
	MinSize int64 `json:"min_size"`
	// FreeleechOnly drops anything not marked freeleech.
	FreeleechOnly bool `json:"freeleech_only"`
	// ExcludeAnonymous drops anonymous uploads entirely, for destinations where
	// even an "Anonymous" line is unwanted.
	ExcludeAnonymous bool `json:"exclude_anonymous"`
}

// ParseFilters decodes the stored filters JSONB. Unknown keys are rejected so a
// typo like "category_id" fails at save time instead of silently filtering
// nothing forever.
func ParseFilters(raw json.RawMessage) (Filters, error) {
	var f Filters
	if len(raw) == 0 {
		return f, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return Filters{}, fmt.Errorf("parse filters: %w", err)
	}
	if f.MinSize < 0 {
		return Filters{}, fmt.Errorf("parse filters: min_size cannot be negative")
	}
	return f, nil
}

// Matches reports whether the announcement should be delivered to this instance.
func (f Filters) Matches(a Announcement) bool {
	if len(f.EventTypes) > 0 && !slices.Contains(f.EventTypes, a.Event) {
		return false
	}
	if len(f.CategoryIDs) > 0 && !slices.Contains(f.CategoryIDs, a.CategoryID) {
		return false
	}
	if f.MinSize > 0 && a.Size < f.MinSize {
		return false
	}
	if f.FreeleechOnly && !a.Freeleech {
		return false
	}
	if f.ExcludeAnonymous && a.Anonymous {
		return false
	}
	return true
}
