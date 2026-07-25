package connector

import (
	"encoding/json"
	"testing"
)

func publishedAnnouncement() Announcement {
	return Announcement{
		Event:      EventTorrentPublished,
		TorrentID:  42,
		Name:       "Some.Release-GROUP",
		CategoryID: 3,
		Category:   "Movies",
		Size:       2 * 1024 * 1024 * 1024,
		Uploader:   "alice",
		FileCount:  1,
	}
}

func TestEmptyFiltersMatchEverything(t *testing.T) {
	f, err := ParseFilters(nil)
	if err != nil {
		t.Fatalf("ParseFilters(nil): %v", err)
	}
	if !f.Matches(publishedAnnouncement()) {
		t.Fatal("zero-value filters must match every announcement")
	}

	f, err = ParseFilters(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("ParseFilters({}): %v", err)
	}
	if !f.Matches(publishedAnnouncement()) {
		t.Fatal("empty filters must match every announcement")
	}
}

func TestFiltersCategoryInclusion(t *testing.T) {
	f, err := ParseFilters(json.RawMessage(`{"category_ids":[3,9]}`))
	if err != nil {
		t.Fatalf("ParseFilters: %v", err)
	}

	a := publishedAnnouncement()
	if !f.Matches(a) {
		t.Fatal("category 3 is listed and must match")
	}

	a.CategoryID = 7
	if f.Matches(a) {
		t.Fatal("category 7 is not listed and must not match")
	}
}

func TestFiltersEventTypes(t *testing.T) {
	f, err := ParseFilters(json.RawMessage(`{"event_types":["torrent.published"]}`))
	if err != nil {
		t.Fatalf("ParseFilters: %v", err)
	}

	a := publishedAnnouncement()
	if !f.Matches(a) {
		t.Fatal("listed event type must match")
	}

	a.Event = EventTest
	if f.Matches(a) {
		t.Fatal("unlisted event type must not match")
	}
}

// MinSize is inclusive: a torrent exactly at the threshold is delivered. The
// boundary is pinned here because "at least this big" is what an admin means.
func TestFiltersMinSizeIsInclusive(t *testing.T) {
	f, err := ParseFilters(json.RawMessage(`{"min_size":1024}`))
	if err != nil {
		t.Fatalf("ParseFilters: %v", err)
	}

	a := publishedAnnouncement()
	a.Size = 1024
	if !f.Matches(a) {
		t.Fatal("size exactly at min_size must match")
	}

	a.Size = 1023
	if f.Matches(a) {
		t.Fatal("size below min_size must not match")
	}
}

func TestFiltersFreeleechOnly(t *testing.T) {
	f, err := ParseFilters(json.RawMessage(`{"freeleech_only":true}`))
	if err != nil {
		t.Fatalf("ParseFilters: %v", err)
	}

	a := publishedAnnouncement()
	if f.Matches(a) {
		t.Fatal("non-freeleech announcement must not match freeleech_only")
	}

	a.Freeleech = true
	if !f.Matches(a) {
		t.Fatal("freeleech announcement must match freeleech_only")
	}
}

func TestFiltersExcludeAnonymous(t *testing.T) {
	f, err := ParseFilters(json.RawMessage(`{"exclude_anonymous":true}`))
	if err != nil {
		t.Fatalf("ParseFilters: %v", err)
	}

	a := publishedAnnouncement()
	if !f.Matches(a) {
		t.Fatal("named upload must match")
	}

	a.Anonymous = true
	a.Uploader = AnonymousUploader
	if f.Matches(a) {
		t.Fatal("anonymous upload must be excluded")
	}
}

// A typo in a filter key would otherwise create a filter that silently filters
// nothing, forever. It has to fail at save time.
func TestParseFiltersRejectsUnknownKey(t *testing.T) {
	if _, err := ParseFilters(json.RawMessage(`{"category_id":[3]}`)); err == nil {
		t.Fatal("expected unknown filter key to be rejected")
	}
}

func TestParseFiltersRejectsNegativeMinSize(t *testing.T) {
	if _, err := ParseFilters(json.RawMessage(`{"min_size":-1}`)); err == nil {
		t.Fatal("expected negative min_size to be rejected")
	}
}

func TestParseFiltersRejectsMalformedJSON(t *testing.T) {
	if _, err := ParseFilters(json.RawMessage(`{`)); err == nil {
		t.Fatal("expected malformed filter JSON to be rejected")
	}
}
