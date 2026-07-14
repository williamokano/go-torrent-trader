package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// The paginating List methods all clamp their inputs before building the query:
// page <= 0 becomes 1, perPage <= 0 becomes a default, and perPage above 100 is
// capped. The cap is a resource guard — without it an API caller could ask for
// perPage=1000000 and pull the whole table into memory — so it is worth pinning
// rather than trusting.
//
// These tests assert the clamp through observable behaviour: ask for a
// pathological page size, and check you get the capped number of rows back.

// seedUsers creates n users and returns them.
func seedUsers(t *testing.T, db *sql.DB, n int) []*model.User {
	t.Helper()
	users := make([]*model.User, 0, n)
	for i := 0; i < n; i++ {
		users = append(users, newUser(t, db))
	}
	return users
}

func TestUserRepoListClampsPageAndPerPage(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	seedUsers(t, db, 3)

	// page 0 and perPage 0 fall back to page 1 / the 25-row default rather than
	// producing a negative OFFSET, which Postgres would reject.
	users, total, err := repo.List(ctx, repository.ListUsersOptions{Page: 0, PerPage: 0})
	if err != nil {
		t.Fatalf("List(0,0): %v", err)
	}
	if total != 3 || len(users) != 3 {
		t.Errorf("List(0,0) returned %d rows (total %d), want 3/3", len(users), total)
	}

	// A negative page is likewise clamped to the first page.
	users, _, err = repo.List(ctx, repository.ListUsersOptions{Page: -5, PerPage: 10})
	if err != nil {
		t.Fatalf("List(-5,10): %v", err)
	}
	if len(users) != 3 {
		t.Errorf("List(-5,10) returned %d rows, want 3", len(users))
	}

	// An absurd perPage is capped at 100, not honoured.
	users, _, err = repo.List(ctx, repository.ListUsersOptions{Page: 1, PerPage: 1000000})
	if err != nil {
		t.Fatalf("List(1,1000000): %v", err)
	}
	if len(users) != 3 {
		t.Errorf("List(1,1000000) returned %d rows, want 3", len(users))
	}
}

// TestUserRepoListCapsPerPageAt100 proves the cap by seeding more than 100 rows:
// if the cap were missing, the request would return all 105.
func TestUserRepoListCapsPerPageAt100(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	seedUsers(t, db, 105)

	users, total, err := repo.List(ctx, repository.ListUsersOptions{Page: 1, PerPage: 500})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 105 {
		t.Errorf("total = %d, want 105", total)
	}
	if len(users) != 100 {
		t.Errorf("List(perPage=500) returned %d rows, want 100 — the cap is not enforced", len(users))
	}

	// The remainder is on page 2, which proves the cap fed a real LIMIT of 100.
	users, _, err = repo.List(ctx, repository.ListUsersOptions{Page: 2, PerPage: 500})
	if err != nil {
		t.Fatalf("List(page 2): %v", err)
	}
	if len(users) != 5 {
		t.Errorf("page 2 returned %d rows, want the 5 remaining", len(users))
	}
}

func TestUserRepoListFiltersAndSorts(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	target := newUser(t, db)
	other := newUser(t, db)

	// Disable one so the Enabled filter has something to exclude.
	other.Enabled = false
	if err := repo.Update(ctx, other); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Search matches on username (and email, which embeds the username here).
	users, total, err := repo.List(ctx, repository.ListUsersOptions{Search: target.Username})
	if err != nil {
		t.Fatalf("List(search): %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != target.ID {
		t.Fatalf("search for %q returned %d rows (total %d), want just that user", target.Username, len(users), total)
	}

	// A search that matches nothing is an empty page, not an error.
	users, total, err = repo.List(ctx, repository.ListUsersOptions{Search: "nonexistent-zzz"})
	if err != nil {
		t.Fatalf("List(no match): %v", err)
	}
	if total != 0 || len(users) != 0 {
		t.Errorf("no-match search returned %d rows (total %d), want 0/0", len(users), total)
	}

	// Enabled filter excludes the disabled user.
	users, _, err = repo.List(ctx, repository.ListUsersOptions{Enabled: ptr(true)})
	if err != nil {
		t.Fatalf("List(enabled): %v", err)
	}
	if len(users) != 1 || users[0].ID != target.ID {
		t.Errorf("Enabled=true returned %d rows, want only the enabled user", len(users))
	}
	users, _, err = repo.List(ctx, repository.ListUsersOptions{Enabled: ptr(false)})
	if err != nil {
		t.Fatalf("List(disabled): %v", err)
	}
	if len(users) != 1 || users[0].ID != other.ID {
		t.Errorf("Enabled=false returned %d rows, want only the disabled user", len(users))
	}

	// GroupID filter: both fixtures share the seeded group, so it matches both.
	users, _, err = repo.List(ctx, repository.ListUsersOptions{GroupID: &target.GroupID})
	if err != nil {
		t.Fatalf("List(group): %v", err)
	}
	if len(users) != 2 {
		t.Errorf("GroupID filter returned %d rows, want 2", len(users))
	}
	// A group nobody is in returns nothing.
	users, _, err = repo.List(ctx, repository.ListUsersOptions{GroupID: ptr(int64(999999))})
	if err != nil {
		t.Fatalf("List(unknown group): %v", err)
	}
	if len(users) != 0 {
		t.Errorf("unknown GroupID returned %d rows, want 0", len(users))
	}

	// Sorting: username ascending must order the two deterministically, and the
	// descending sort must be its exact reverse. An unknown SortBy falls back to
	// created_at rather than interpolating the caller's string into the SQL.
	asc, _, err := repo.List(ctx, repository.ListUsersOptions{SortBy: "username", SortOrder: "asc"})
	if err != nil {
		t.Fatalf("List(sort asc): %v", err)
	}
	desc, _, err := repo.List(ctx, repository.ListUsersOptions{SortBy: "username", SortOrder: "desc"})
	if err != nil {
		t.Fatalf("List(sort desc): %v", err)
	}
	if len(asc) != 2 || len(desc) != 2 {
		t.Fatalf("sorted lists have %d/%d rows, want 2/2", len(asc), len(desc))
	}
	if asc[0].Username > asc[1].Username {
		t.Errorf("asc sort is not ascending: %q then %q", asc[0].Username, asc[1].Username)
	}
	if desc[0].ID != asc[1].ID || desc[1].ID != asc[0].ID {
		t.Error("desc sort is not the reverse of asc")
	}

	// An unrecognised sort column must not reach the SQL; it falls back silently.
	if _, _, err := repo.List(ctx, repository.ListUsersOptions{SortBy: "id; DROP TABLE users"}); err != nil {
		t.Errorf("List(bogus SortBy): %v — an unknown column must fall back, not error", err)
	}
	var stillThere int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&stillThere); err != nil {
		t.Fatalf("counting users: %v", err)
	}
	if stillThere != 2 {
		t.Fatalf("users table has %d rows after a bogus SortBy, want 2", stillThere)
	}
}

func TestWarningRepoListAllClampsPagination(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewWarningRepo(db)
	u := newUser(t, db)
	for i := 0; i < 3; i++ {
		newWarning(t, db, u.ID, "manual", "active", nil)
	}

	// Zero page/perPage fall back to the defaults.
	warnings, total, err := repo.ListAll(ctx, repository.ListWarningsOptions{})
	if err != nil {
		t.Fatalf("ListAll(defaults): %v", err)
	}
	if total != 3 || len(warnings) != 3 {
		t.Errorf("ListAll(defaults) returned %d (total %d), want 3/3", len(warnings), total)
	}

	// perPage above the cap is clamped, and a page past the end is empty.
	warnings, _, err = repo.ListAll(ctx, repository.ListWarningsOptions{Page: 1, PerPage: 5000})
	if err != nil {
		t.Fatalf("ListAll(perPage 5000): %v", err)
	}
	if len(warnings) != 3 {
		t.Errorf("ListAll(perPage 5000) returned %d, want 3", len(warnings))
	}

	warnings, total, err = repo.ListAll(ctx, repository.ListWarningsOptions{Page: 99, PerPage: 10})
	if err != nil {
		t.Fatalf("ListAll(page 99): %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("page 99 returned %d rows, want 0", len(warnings))
	}
	if total != 3 {
		t.Errorf("total past the end = %d, want 3 (the count is independent of the page)", total)
	}

	// "all" is a sentinel meaning "do not filter on status", distinct from a
	// literal status value: it must not be pushed into the WHERE clause.
	warnings, _, err = repo.ListAll(ctx, repository.ListWarningsOptions{Status: ptr("all")})
	if err != nil {
		t.Fatalf("ListAll(status=all): %v", err)
	}
	if len(warnings) != 3 {
		t.Errorf("status=all returned %d, want all 3", len(warnings))
	}

	// The username search goes through the LEFT JOIN on users.
	warnings, _, err = repo.ListAll(ctx, repository.ListWarningsOptions{Search: u.Username})
	if err != nil {
		t.Fatalf("ListAll(search): %v", err)
	}
	if len(warnings) != 3 {
		t.Errorf("search for %q returned %d, want 3", u.Username, len(warnings))
	}
	warnings, _, err = repo.ListAll(ctx, repository.ListWarningsOptions{Search: "nobody-zzz"})
	if err != nil {
		t.Fatalf("ListAll(search miss): %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("a non-matching search returned %d, want 0", len(warnings))
	}
}

func TestCheatFlagRepoListFiltersAndClamps(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewCheatFlagRepo(db)
	u1 := newUser(t, db)
	u2 := newUser(t, db)
	mod := newUser(t, db)
	tor := newTorrent(t, db, u1.ID)

	mk := func(userID int64, flagType string) *model.CheatFlag {
		f := &model.CheatFlag{UserID: userID, TorrentID: &tor.ID, FlagType: flagType, Details: `{}`}
		if err := repo.Create(ctx, f); err != nil {
			t.Fatalf("Create: %v", err)
		}
		return f
	}
	speed := mk(u1.ID, "impossible_speed")
	mk(u1.ID, "multi_ip")
	mk(u2.ID, "impossible_speed")

	if err := repo.Dismiss(ctx, speed.ID, mod.ID); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}

	// Defaults: no filters, everything comes back.
	flags, total, err := repo.List(ctx, repository.ListCheatFlagsOptions{})
	if err != nil {
		t.Fatalf("List(defaults): %v", err)
	}
	if total != 3 || len(flags) != 3 {
		t.Errorf("List(defaults) returned %d (total %d), want 3/3", len(flags), total)
	}

	// UserID filter.
	flags, total, err = repo.List(ctx, repository.ListCheatFlagsOptions{UserID: &u2.ID})
	if err != nil {
		t.Fatalf("List(user): %v", err)
	}
	if total != 1 || len(flags) != 1 || flags[0].UserID != u2.ID {
		t.Errorf("UserID filter returned %d (total %d), want only u2's flag", len(flags), total)
	}

	// FlagType filter.
	flags, total, err = repo.List(ctx, repository.ListCheatFlagsOptions{FlagType: ptr("multi_ip")})
	if err != nil {
		t.Fatalf("List(type): %v", err)
	}
	if total != 1 || len(flags) != 1 || flags[0].FlagType != "multi_ip" {
		t.Errorf("FlagType filter returned %d (total %d), want the one multi_ip flag", len(flags), total)
	}

	// Dismissed filter, both polarities. A *bool of false must actually filter
	// rather than being treated as "unset" — the classic nil-vs-false bug.
	flags, total, err = repo.List(ctx, repository.ListCheatFlagsOptions{Dismissed: ptr(true)})
	if err != nil {
		t.Fatalf("List(dismissed): %v", err)
	}
	if total != 1 || len(flags) != 1 || flags[0].ID != speed.ID {
		t.Errorf("Dismissed=true returned %d (total %d), want the one dismissed flag", len(flags), total)
	}
	flags, total, err = repo.List(ctx, repository.ListCheatFlagsOptions{Dismissed: ptr(false)})
	if err != nil {
		t.Fatalf("List(undismissed): %v", err)
	}
	if total != 2 || len(flags) != 2 {
		t.Errorf("Dismissed=false returned %d (total %d), want the 2 open flags", len(flags), total)
	}

	// Filters compose.
	flags, total, err = repo.List(ctx, repository.ListCheatFlagsOptions{
		UserID:    &u1.ID,
		FlagType:  ptr("impossible_speed"),
		Dismissed: ptr(true),
	})
	if err != nil {
		t.Fatalf("List(combined): %v", err)
	}
	if total != 1 || len(flags) != 1 || flags[0].ID != speed.ID {
		t.Errorf("combined filters returned %d (total %d), want exactly the dismissed speed flag", len(flags), total)
	}

	// Pagination clamps.
	flags, _, err = repo.List(ctx, repository.ListCheatFlagsOptions{Page: -1, PerPage: 9999})
	if err != nil {
		t.Fatalf("List(clamped): %v", err)
	}
	if len(flags) != 3 {
		t.Errorf("clamped List returned %d, want 3", len(flags))
	}
}

func TestNewsRepoListPublishedClampsPagination(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNewsRepo(db)
	author := newUser(t, db)

	for i := 0; i < 3; i++ {
		a := &model.NewsArticle{Title: uniq("news"), Body: "body", AuthorID: &author.ID, Published: true}
		if err := repo.Create(ctx, a); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	// A draft must never appear in the published listing.
	draft := &model.NewsArticle{Title: "draft", Body: "body", AuthorID: &author.ID, Published: false}
	if err := repo.Create(ctx, draft); err != nil {
		t.Fatalf("Create(draft): %v", err)
	}

	articles, total, err := repo.ListPublished(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ListPublished(0,0): %v", err)
	}
	if total != 3 || len(articles) != 3 {
		t.Errorf("ListPublished(0,0) returned %d (total %d), want 3/3 — the draft must be excluded", len(articles), total)
	}

	articles, _, err = repo.ListPublished(ctx, 1, 5000)
	if err != nil {
		t.Fatalf("ListPublished(perPage 5000): %v", err)
	}
	if len(articles) != 3 {
		t.Errorf("ListPublished(perPage 5000) returned %d, want 3", len(articles))
	}

	articles, total, err = repo.ListPublished(ctx, 50, 10)
	if err != nil {
		t.Fatalf("ListPublished(page 50): %v", err)
	}
	if len(articles) != 0 {
		t.Errorf("page 50 returned %d rows, want 0", len(articles))
	}
	if total != 3 {
		t.Errorf("total past the end = %d, want 3", total)
	}

	// The admin List with an explicit Published filter is the counterpart.
	drafts, total, err := repo.List(ctx, repository.ListNewsOptions{Published: ptr(false)})
	if err != nil {
		t.Fatalf("List(published=false): %v", err)
	}
	if total != 1 || len(drafts) != 1 || drafts[0].ID != draft.ID {
		t.Errorf("Published=false returned %d (total %d), want the single draft", len(drafts), total)
	}
	// Unfiltered, the admin sees everything.
	all, total, err := repo.List(ctx, repository.ListNewsOptions{Page: -1, PerPage: 9999})
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	if total != 4 || len(all) != 4 {
		t.Errorf("unfiltered List returned %d (total %d), want 4/4", len(all), total)
	}
}

func TestInviteRepoListByInviterClampsPagination(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewInviteRepo(db)
	inviter := newUser(t, db)
	stranger := newUser(t, db)

	for i := 0; i < 3; i++ {
		inv := &model.Invite{
			InviterID: inviter.ID,
			Token:     uniq("tok"),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		if err := repo.Create(ctx, inv); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	invites, total, err := repo.ListByInviter(ctx, inviter.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListByInviter(0,0): %v", err)
	}
	if total != 3 || len(invites) != 3 {
		t.Errorf("ListByInviter(0,0) returned %d (total %d), want 3/3", len(invites), total)
	}

	invites, _, err = repo.ListByInviter(ctx, inviter.ID, 1, 9999)
	if err != nil {
		t.Fatalf("ListByInviter(perPage 9999): %v", err)
	}
	if len(invites) != 3 {
		t.Errorf("ListByInviter(perPage 9999) returned %d, want 3", len(invites))
	}

	invites, total, err = repo.ListByInviter(ctx, inviter.ID, 99, 10)
	if err != nil {
		t.Fatalf("ListByInviter(page 99): %v", err)
	}
	if len(invites) != 0 || total != 3 {
		t.Errorf("page 99 returned %d rows (total %d), want 0 rows and total 3", len(invites), total)
	}

	// The listing is scoped to the inviter: another user sees none of them.
	invites, total, err = repo.ListByInviter(ctx, stranger.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByInviter(stranger): %v", err)
	}
	if len(invites) != 0 || total != 0 {
		t.Errorf("a stranger saw %d of the inviter's invites (total %d), want 0", len(invites), total)
	}
}

func TestPeerRepoListByUserClampsPaginationAndSplitsBySeeder(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewPeerRepo(db)
	u := newUser(t, db)
	uploader := newUser(t, db)

	var seedingTorrents []*model.Torrent
	for i := 0; i < 3; i++ {
		tor := newTorrent(t, db, uploader.ID)
		newPeer(t, db, tor.ID, u.ID, true)
		seedingTorrents = append(seedingTorrents, tor)
	}
	leechTorrent := newTorrent(t, db, uploader.ID)
	newPeer(t, db, leechTorrent.ID, u.ID, false)

	// Zero page/perPage clamp to the defaults.
	seeding, total, err := repo.ListByUserSeeding(ctx, u.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListByUserSeeding(0,0): %v", err)
	}
	if total != 3 || len(seeding) != 3 {
		t.Errorf("ListByUserSeeding(0,0) returned %d (total %d), want 3/3", len(seeding), total)
	}
	// The torrent name comes from the JOIN, so it must be populated.
	if seeding[0].TorrentName == "" {
		t.Error("TorrentName is empty; the torrents JOIN did not resolve")
	}

	// perPage above the cap clamps.
	seeding, _, err = repo.ListByUserSeeding(ctx, u.ID, 1, 9999)
	if err != nil {
		t.Fatalf("ListByUserSeeding(perPage 9999): %v", err)
	}
	if len(seeding) != 3 {
		t.Errorf("ListByUserSeeding(perPage 9999) returned %d, want 3", len(seeding))
	}

	// Leeching is the complement, not a copy of the seeding list.
	leeching, total, err := repo.ListByUserLeeching(ctx, u.ID, -1, -1)
	if err != nil {
		t.Fatalf("ListByUserLeeching(-1,-1): %v", err)
	}
	if total != 1 || len(leeching) != 1 {
		t.Fatalf("ListByUserLeeching returned %d (total %d), want 1/1", len(leeching), total)
	}
	if leeching[0].TorrentID != leechTorrent.ID {
		t.Errorf("leeching torrent = %d, want %d", leeching[0].TorrentID, leechTorrent.ID)
	}
	for _, s := range seedingTorrents {
		if leeching[0].TorrentID == s.ID {
			t.Error("a seeding torrent leaked into the leeching list")
		}
	}

	// Past the end is empty.
	seeding, total, err = repo.ListByUserSeeding(ctx, u.ID, 99, 10)
	if err != nil {
		t.Fatalf("ListByUserSeeding(page 99): %v", err)
	}
	if len(seeding) != 0 || total != 3 {
		t.Errorf("page 99 returned %d rows (total %d), want 0 rows and total 3", len(seeding), total)
	}
}

func TestTransferHistoryListByUserClampsAndNamesDeletedTorrents(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTransferHistoryRepo(db)
	u := newUser(t, db)
	uploader := newUser(t, db)
	tor := newTorrent(t, db, uploader.ID)

	th := &model.TransferHistory{
		UserID:       u.ID,
		TorrentID:    &tor.ID,
		Uploaded:     100,
		Downloaded:   50,
		Seeder:       true,
		CompletedAt:  time.Now(),
		LastAnnounce: time.Now(),
	}
	if err := repo.Upsert(ctx, th); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Clamped pagination still returns the row, with the torrent name joined in.
	items, total, err := repo.ListByUser(ctx, u.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListByUser(0,0): %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("ListByUser(0,0) returned %d (total %d), want 1/1", len(items), total)
	}
	if items[0].TorrentName != tor.Name {
		t.Errorf("TorrentName = %q, want %q", items[0].TorrentName, tor.Name)
	}

	items, _, err = repo.ListByUser(ctx, u.ID, 1, 9999)
	if err != nil {
		t.Fatalf("ListByUser(perPage 9999): %v", err)
	}
	if len(items) != 1 {
		t.Errorf("ListByUser(perPage 9999) returned %d, want 1", len(items))
	}

	// Deleting the torrent must not orphan the history row: the LEFT JOIN plus
	// COALESCE has to keep it listable under a placeholder name. If the join were
	// an inner one the user's history would silently lose entries.
	if _, err := db.Exec(`DELETE FROM torrents WHERE id = $1`, tor.ID); err != nil {
		t.Fatalf("deleting torrent: %v", err)
	}

	items, total, err = repo.ListByUser(ctx, u.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByUser after torrent delete: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("history lost its row when the torrent was deleted: %d rows (total %d)", len(items), total)
	}
	if items[0].TorrentName != "Deleted Torrent" {
		t.Errorf("TorrentName = %q, want %q", items[0].TorrentName, "Deleted Torrent")
	}

	// Past the end is empty.
	items, _, err = repo.ListByUser(ctx, u.ID, 99, 10)
	if err != nil {
		t.Fatalf("ListByUser(page 99): %v", err)
	}
	if len(items) != 0 {
		t.Errorf("page 99 returned %d rows, want 0", len(items))
	}
}

func TestChatMuteListActiveClampsPagination(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewChatMuteRepo(db)
	mod := newUser(t, db)

	for i := 0; i < 3; i++ {
		u := newUser(t, db)
		m := &model.ChatMute{
			UserID:    u.ID,
			MutedBy:   &mod.ID,
			Reason:    "spam",
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if err := repo.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	// An already-expired mute must not appear in the active list.
	expiredUser := newUser(t, db)
	expired := &model.ChatMute{
		UserID:    expiredUser.ID,
		MutedBy:   &mod.ID,
		Reason:    "old",
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	if err := repo.Create(ctx, expired); err != nil {
		t.Fatalf("Create(expired): %v", err)
	}

	mutes, total, err := repo.ListActive(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ListActive(0,0): %v", err)
	}
	if total != 3 || len(mutes) != 3 {
		t.Errorf("ListActive(0,0) returned %d (total %d), want 3/3 — the expired mute must be excluded", len(mutes), total)
	}
	// The JOINs resolve both the muted user and the staffer who muted them.
	if mutes[0].Username == "" {
		t.Error("Username is empty; the users JOIN did not resolve")
	}
	if mutes[0].MutedByName == nil || *mutes[0].MutedByName != mod.Username {
		t.Errorf("MutedByName = %v, want %q", mutes[0].MutedByName, mod.Username)
	}

	mutes, _, err = repo.ListActive(ctx, 2, 2)
	if err != nil {
		t.Fatalf("ListActive(page 2): %v", err)
	}
	if len(mutes) != 1 {
		t.Errorf("page 2 of 2 returned %d, want the 1 remainder", len(mutes))
	}

	mutes, total, err = repo.ListActive(ctx, 99, 10)
	if err != nil {
		t.Fatalf("ListActive(page 99): %v", err)
	}
	if len(mutes) != 0 || total != 3 {
		t.Errorf("page 99 returned %d rows (total %d), want 0 rows and total 3", len(mutes), total)
	}
}

func TestChatMessageDeleteByUserIDReportsCount(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewChatMessageRepo(db)
	spammer := newUser(t, db)
	innocent := newUser(t, db)

	for i := 0; i < 3; i++ {
		if err := repo.Create(ctx, &model.ChatMessage{UserID: spammer.ID, Message: uniq("spam")}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	if err := repo.Create(ctx, &model.ChatMessage{UserID: innocent.ID, Message: "hello"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The purge reports how many it removed, and touches only the spammer's.
	n, err := repo.DeleteByUserID(ctx, spammer.ID)
	if err != nil {
		t.Fatalf("DeleteByUserID: %v", err)
	}
	if n != 3 {
		t.Errorf("DeleteByUserID = %d, want 3", n)
	}

	remaining, err := repo.ListRecent(ctx, 50)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(remaining) != 1 || remaining[0].UserID != innocent.ID {
		t.Errorf("purge removed %d other messages; only the spammer's should go", 1-len(remaining))
	}

	// Purging a user with nothing to purge is zero, not an error.
	n, err = repo.DeleteByUserID(ctx, spammer.ID)
	if err != nil {
		t.Fatalf("second DeleteByUserID: %v", err)
	}
	if n != 0 {
		t.Errorf("second DeleteByUserID = %d, want 0", n)
	}
}

func TestNotificationDeleteOldReportsCount(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationRepo(db)
	u := newUser(t, db)

	// DeleteOld reaps only notifications that are BOTH old and read.
	mk := func(read bool, createdAt time.Time) int64 {
		n := &model.Notification{UserID: u.ID, Type: "pm", Data: []byte(`{}`)}
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, err := db.Exec(
			`UPDATE notifications SET created_at = $1, is_read = $2 WHERE id = $3`,
			createdAt, read, n.ID,
		); err != nil {
			t.Fatalf("backdating notification: %v", err)
		}
		return n.ID
	}

	old := time.Now().Add(-48 * time.Hour)
	oldRead := mk(true, old)
	oldUnread := mk(false, old)
	freshRead := mk(true, time.Now())

	cutoff := time.Now().Add(-24 * time.Hour)
	n, err := repo.DeleteOld(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeleteOld: %v", err)
	}
	if n != 1 {
		t.Errorf("DeleteOld = %d, want 1 (only the old AND read one)", n)
	}

	if _, err := repo.GetByID(ctx, oldRead); err == nil {
		t.Error("the old read notification survived DeleteOld")
	}
	// An old but unread notification must be kept: the user has not seen it yet.
	if _, err := repo.GetByID(ctx, oldUnread); err != nil {
		t.Errorf("DeleteOld reaped an unread notification: %v", err)
	}
	if _, err := repo.GetByID(ctx, freshRead); err != nil {
		t.Errorf("DeleteOld reaped a fresh notification: %v", err)
	}
}
