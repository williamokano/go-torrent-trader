package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func TestTorrentRepoListMissingRequiredMetadata(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewTorrentRepo(db)

	u1 := newUser(t, db)
	u2 := newUser(t, db)
	cat := newCategoryID(t, db)

	// mk places a torrent in `cat` with the given metadata.
	mk := func(uploaderID int64, meta string) *model.Torrent {
		tor := newTorrent(t, db, uploaderID)
		tor.CategoryID = cat
		tor.Metadata = json.RawMessage(meta)
		if err := repo.Update(ctx, tor); err != nil {
			t.Fatalf("Update: %v", err)
		}
		return tor
	}

	complete := mk(u1.ID, `{"year":2020}`) // has year
	missingU1 := mk(u1.ID, `{}`)           // missing year
	missingU2 := mk(u2.ID, `{"codec":"x264"}`)

	ids := func(ts []model.Torrent) map[int64]bool {
		out := map[int64]bool{}
		for _, tor := range ts {
			out[tor.ID] = true
		}
		return out
	}

	t.Run("finds all torrents missing a required key", func(t *testing.T) {
		got, err := repo.ListMissingRequiredMetadata(ctx, cat, []string{"year"}, nil)
		if err != nil {
			t.Fatalf("ListMissingRequiredMetadata: %v", err)
		}
		set := ids(got)
		if set[complete.ID] {
			t.Error("torrent with year should not be reported")
		}
		if !set[missingU1.ID] || !set[missingU2.ID] {
			t.Errorf("missing torrents = %v, want both %d and %d", set, missingU1.ID, missingU2.ID)
		}
	})

	t.Run("scopes to a single uploader", func(t *testing.T) {
		got, err := repo.ListMissingRequiredMetadata(ctx, cat, []string{"year"}, &u1.ID)
		if err != nil {
			t.Fatalf("ListMissingRequiredMetadata: %v", err)
		}
		set := ids(got)
		if !set[missingU1.ID] {
			t.Errorf("want u1's missing torrent %d in %v", missingU1.ID, set)
		}
		if set[missingU2.ID] {
			t.Error("u2's torrent must not appear when scoped to u1")
		}
	})

	t.Run("no required keys is a no-op", func(t *testing.T) {
		got, err := repo.ListMissingRequiredMetadata(ctx, cat, nil, nil)
		if err != nil {
			t.Fatalf("ListMissingRequiredMetadata: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d rows, want 0", len(got))
		}
	})
}
