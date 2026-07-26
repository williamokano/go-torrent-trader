package postgres

import (
	"bytes"
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// The info-hash predicate is what makes "a takedown notice names this hash" a
// lookup rather than a scan through every name on the tracker.
func TestTorrentRepo_ListByInfoHash(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	user := newUser(t, db)
	wanted := newTorrent(t, db, user.ID)
	other := newTorrent(t, db, user.ID)

	torrents, total, err := repo.List(ctx, repository.ListTorrentsOptions{InfoHash: wanted.InfoHash})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(torrents) != 1 {
		t.Fatalf("total=%d len=%d, want exactly one match", total, len(torrents))
	}
	if torrents[0].ID != wanted.ID {
		t.Errorf("matched torrent %d, want %d", torrents[0].ID, wanted.ID)
	}
	if !bytes.Equal(torrents[0].InfoHash, wanted.InfoHash) {
		t.Errorf("info hash round-tripped as %x, want %x", torrents[0].InfoHash, wanted.InfoHash)
	}
	if torrents[0].ID == other.ID {
		t.Error("the wrong torrent matched")
	}

	// A hash nothing has is an empty result, not an error and not everything.
	_, total, err = repo.List(ctx, repository.ListTorrentsOptions{InfoHash: infoHash("nothing-has-this")})
	if err != nil {
		t.Fatalf("List(unknown hash): %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d for an unknown hash, want 0", total)
	}
}

// A banned torrent is exactly the one the default listing hides, so the filter is
// only reachable through the admin path. Both halves are asserted: a filter that
// silently kept the default `banned = false` predicate would return nothing and
// look like "no banned torrents", which is the answer an operator would believe.
func TestTorrentRepo_ListByBanned(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	user := newUser(t, db)
	clean := newTorrent(t, db, user.ID)
	naughty := newTorrent(t, db, user.ID)

	naughty.Banned = true
	if err := repo.Update(ctx, naughty); err != nil {
		t.Fatalf("banning torrent: %v", err)
	}

	banned := true
	got, total, err := repo.List(ctx, repository.ListTorrentsOptions{
		IncludeHidden: true, Banned: &banned,
	})
	if err != nil {
		t.Fatalf("List(banned): %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].ID != naughty.ID {
		t.Fatalf("banned listing returned %d rows (%+v), want just torrent %d", total, got, naughty.ID)
	}

	notBanned := false
	got, total, err = repo.List(ctx, repository.ListTorrentsOptions{
		IncludeHidden: true, Banned: &notBanned,
	})
	if err != nil {
		t.Fatalf("List(not banned): %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].ID != clean.ID {
		t.Fatalf("un-banned listing returned %d rows (%+v), want just torrent %d", total, got, clean.ID)
	}

	// Omitted means both, which is what the unfiltered admin listing shows.
	_, total, err = repo.List(ctx, repository.ListTorrentsOptions{IncludeHidden: true})
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	if total != 2 {
		t.Errorf("unfiltered admin listing total = %d, want 2", total)
	}
}

// The two predicates have to compose, since a moderator working from a takedown
// notice may well be checking whether that hash is already banned.
func TestTorrentRepo_ListByInfoHashAndBanned(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	user := newUser(t, db)
	tor := newTorrent(t, db, user.ID)

	banned := true
	_, total, err := repo.List(ctx, repository.ListTorrentsOptions{
		IncludeHidden: true, InfoHash: tor.InfoHash, Banned: &banned,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0 — the torrent is not banned yet", total)
	}

	tor.Banned = true
	if err := repo.Update(ctx, tor); err != nil {
		t.Fatalf("banning torrent: %v", err)
	}

	got, total, err := repo.List(ctx, repository.ListTorrentsOptions{
		IncludeHidden: true, InfoHash: tor.InfoHash, Banned: &banned,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].ID != tor.ID {
		t.Errorf("total = %d (%+v), want the now-banned torrent", total, got)
	}
}

// An edit must not roll back what the announce path recorded while it was in
// flight. Update used to write seeders/leechers/times_completed from the struct,
// making every edit a read-modify-write over counters that are maintained with
// relative statements — so a torrent that gained seeders between the read and the
// write was written back at its old count. One admin banning one torrent lost a few
// announces; a bulk ban of a hundred loses a burst across the busiest rows.
func TestTorrentRepo_UpdateDoesNotClobberLiveCounters(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	user := newUser(t, db)
	tor := newTorrent(t, db, user.ID)

	// Stand in for announces landing between the read and the write.
	if err := repo.IncrementSeeders(ctx, tor.ID, 5); err != nil {
		t.Fatalf("IncrementSeeders: %v", err)
	}
	if err := repo.IncrementLeechers(ctx, tor.ID, 3); err != nil {
		t.Fatalf("IncrementLeechers: %v", err)
	}
	if err := repo.IncrementTimesCompleted(ctx, tor.ID); err != nil {
		t.Fatalf("IncrementTimesCompleted: %v", err)
	}

	// `tor` is the pre-increment snapshot, exactly as EditTorrent would be holding
	// it. Banning through it must not carry the stale zeros back to the row.
	tor.Banned = true
	if err := repo.Update(ctx, tor); err != nil {
		t.Fatalf("Update: %v", err)
	}

	after, err := repo.GetByID(ctx, tor.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if after.Seeders != 5 || after.Leechers != 3 || after.TimesCompleted != 1 {
		t.Errorf("counters after the edit = %d/%d/%d, want 5/3/1 — the edit reverted announces",
			after.Seeders, after.Leechers, after.TimesCompleted)
	}
	// And the field the edit was actually for did persist.
	if !after.Banned {
		t.Error("the ban did not persist")
	}
}

// Without IncludeHidden the listing still hides banned torrents, whatever the
// filter says. Asserted so nobody wires the banned filter into a member-facing
// listing and exposes banned content.
func TestTorrentRepo_BannedFilterDoesNotBypassTheDefaultHiding(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	user := newUser(t, db)
	tor := newTorrent(t, db, user.ID)
	tor.Banned = true
	if err := repo.Update(ctx, tor); err != nil {
		t.Fatalf("banning torrent: %v", err)
	}

	banned := true
	_, total, err := repo.List(ctx, repository.ListTorrentsOptions{Banned: &banned})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0 — a public listing must not surface banned torrents", total)
	}
}
