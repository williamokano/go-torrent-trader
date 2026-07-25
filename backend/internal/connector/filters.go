package connector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// Category filter modes. The empty string means include, so a row written
// before this field existed keeps its original meaning.
const (
	CategoryModeInclude = "include"
	CategoryModeExclude = "exclude"
)

// Filters narrow which announcements an instance receives. Every field is
// additive and an empty one means "no opinion", so the zero value matches
// everything — which is what a freshly created instance gets.
type Filters struct {
	// EventTypes limits delivery to these Announcement.Event values.
	EventTypes []string `json:"event_types"`
	// CategoryIDs is the category list CategoryMode interprets. Empty means no
	// category opinion in either mode.
	//
	// A listed category covers its whole subtree: a torrent in "Adult / 4K" is
	// matched by "Adult". Anything else would make an exclude filter a trap —
	// the feed configured to keep 18+ out would keep out exactly one category
	// and let every sub-category of it through.
	CategoryIDs []int64 `json:"category_ids"`
	// CategoryMode is "include" (deliver only these) or "exclude" (deliver
	// everything but these).
	CategoryMode string `json:"category_mode"`
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
	switch f.CategoryMode {
	case "", CategoryModeInclude, CategoryModeExclude:
	default:
		// A mode nobody recognises must not silently degrade to "include": that
		// would turn a feed meant to exclude a category into one that carries
		// only that category.
		return Filters{}, fmt.Errorf("parse filters: category_mode must be %q or %q", CategoryModeInclude, CategoryModeExclude)
	}
	return f, nil
}

// Matches reports whether the announcement should be delivered to this instance.
func (f Filters) Matches(a Announcement) bool {
	if len(f.EventTypes) > 0 && !slices.Contains(f.EventTypes, a.Event) {
		return false
	}
	if !f.categoryMatches(a) {
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

// categoryMatches applies CategoryIDs in whichever direction CategoryMode asks
// for, testing the announcement's whole ancestor chain so a listed parent
// stands for its descendants too.
func (f Filters) categoryMatches(a Announcement) bool {
	if len(f.CategoryIDs) == 0 {
		return true
	}
	listed := slices.ContainsFunc(a.CategoryChain(), func(id int64) bool {
		return slices.Contains(f.CategoryIDs, id)
	})
	if f.CategoryMode == CategoryModeExclude {
		return !listed
	}
	return listed
}
