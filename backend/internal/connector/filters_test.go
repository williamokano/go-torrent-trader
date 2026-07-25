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
		// The dispatcher always fills this in, so the default fixture carries it
		// too: a filter test against a bare leaf would be exercising the
		// fallback rather than the shape production sends.
		CategoryPath: []int64{1, 3},
		Category:     "Movies",
		Size:         2 * 1024 * 1024 * 1024,
		Uploader:     "alice",
		FileCount:    1,
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
	a.CategoryPath = []int64{6, 7}
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

// The exclude mode's whole point is the "everything except 18+" feed, so these
// cover the case that mode exists for.

func TestFiltersCategoryExclusion(t *testing.T) {
	f, err := ParseFilters(json.RawMessage(`{"category_ids":[3,9],"category_mode":"exclude"}`))
	if err != nil {
		t.Fatalf("ParseFilters: %v", err)
	}

	a := publishedAnnouncement()
	if f.Matches(a) {
		t.Fatal("category 3 is excluded and must not match")
	}

	a.CategoryID = 7
	a.CategoryPath = []int64{6, 7}
	if !f.Matches(a) {
		t.Fatal("category 7 is not excluded and must match")
	}
}

func TestFiltersExplicitIncludeModeMatchesTheDefault(t *testing.T) {
	explicit, err := ParseFilters(json.RawMessage(`{"category_ids":[3],"category_mode":"include"}`))
	if err != nil {
		t.Fatalf("ParseFilters(explicit): %v", err)
	}
	omitted, err := ParseFilters(json.RawMessage(`{"category_ids":[3]}`))
	if err != nil {
		t.Fatalf("ParseFilters(omitted): %v", err)
	}

	// Rows written before the mode existed must keep meaning what they meant.
	for _, categoryID := range []int64{3, 7} {
		a := publishedAnnouncement()
		a.CategoryID = categoryID
		a.CategoryPath = []int64{categoryID}
		if explicit.Matches(a) != omitted.Matches(a) {
			t.Fatalf("category %d: explicit include = %v, omitted = %v",
				categoryID, explicit.Matches(a), omitted.Matches(a))
		}
	}
}

func TestFiltersEmptyCategoryListMatchesEverythingInBothModes(t *testing.T) {
	for _, mode := range []string{"include", "exclude"} {
		f, err := ParseFilters(json.RawMessage(`{"category_ids":[],"category_mode":"` + mode + `"}`))
		if err != nil {
			t.Fatalf("ParseFilters(%s): %v", mode, err)
		}
		// An exclude filter naming nothing excludes nothing. Reading it as
		// "exclude everything" would silence an instance that looks unfiltered.
		if !f.Matches(publishedAnnouncement()) {
			t.Fatalf("%s mode with an empty list must match everything", mode)
		}
	}
}

func TestFiltersExcludeCoversTheWholeSubtree(t *testing.T) {
	// The case the feature exists for: "Adult" (id 50) is excluded, and the
	// torrent sits in "Adult / 4K" (id 51), so only the ancestor chain can catch
	// it. Matching on the leaf id alone would leak it into the clean feed.
	f, err := ParseFilters(json.RawMessage(`{"category_ids":[50],"category_mode":"exclude"}`))
	if err != nil {
		t.Fatalf("ParseFilters: %v", err)
	}

	a := publishedAnnouncement()
	a.CategoryID = 51
	a.CategoryPath = []int64{50, 51}
	if f.Matches(a) {
		t.Fatal("a sub-category of an excluded category must not match")
	}
}

func TestFiltersIncludeCoversTheWholeSubtree(t *testing.T) {
	f, err := ParseFilters(json.RawMessage(`{"category_ids":[1]}`))
	if err != nil {
		t.Fatalf("ParseFilters: %v", err)
	}

	a := publishedAnnouncement()
	a.CategoryID = 51
	a.CategoryPath = []int64{1, 51}
	if !f.Matches(a) {
		t.Fatal("a sub-category of an included category must match")
	}

	a.CategoryPath = []int64{2, 51}
	if f.Matches(a) {
		t.Fatal("a category under a different parent must not match")
	}
}

func TestFiltersFallBackToTheLeafWhenNoPathIsKnown(t *testing.T) {
	// Payloads stored before CategoryPath existed, and test-sends, carry only
	// the leaf id. They must still filter on it rather than matching nothing.
	f, err := ParseFilters(json.RawMessage(`{"category_ids":[3]}`))
	if err != nil {
		t.Fatalf("ParseFilters: %v", err)
	}
	a := publishedAnnouncement()
	a.CategoryPath = nil
	if !f.Matches(a) {
		t.Fatal("with no path the leaf category must still match")
	}
}

func TestParseFiltersRejectsUnknownCategoryMode(t *testing.T) {
	// Degrading an unrecognised mode to "include" would inverse the filter: an
	// instance meant to carry everything but 18+ would carry only 18+.
	if _, err := ParseFilters(json.RawMessage(`{"category_ids":[3],"category_mode":"only"}`)); err == nil {
		t.Fatal("expected an unknown category_mode to be rejected")
	}
}
