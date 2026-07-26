package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// utc is shorthand for a fixed UTC instant. Every date in these tests is explicit
// so nothing depends on the machine's timezone — which is the whole risk being
// tested, since the rollup buckets by UTC month.
func utc(year int, month time.Month, day, hour, min int) time.Time {
	return time.Date(year, month, day, hour, min, 0, 0, time.UTC)
}

func utcDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// setRollupWatermark plants the watermark row. resetTestData truncates it along
// with everything else, so each test states where its rollup starts from rather
// than inheriting whatever the migration seeded.
func setRollupWatermark(t *testing.T, db *sql.DB, date time.Time) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO announce_rollup_state (id, rolled_through) VALUES (true, $1::date)
		 ON CONFLICT (id) DO UPDATE SET rolled_through = EXCLUDED.rolled_through`,
		date.Format("2006-01-02")); err != nil {
		t.Fatalf("seeding rollup watermark: %v", err)
	}
}

// periodTotals reads one member's monthly rows keyed by year_month.
func periodTotals(t *testing.T, db *sql.DB, userID int64) map[string]model.UserPeriodStats {
	t.Helper()
	rows, err := db.Query(
		`SELECT year_month, uploaded, downloaded, counted_downloaded, announces, seed_announces
		 FROM user_period_stats WHERE user_id = $1`, userID)
	if err != nil {
		t.Fatalf("reading user_period_stats: %v", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]model.UserPeriodStats{}
	for rows.Next() {
		var s model.UserPeriodStats
		if err := rows.Scan(&s.YearMonth, &s.Uploaded, &s.Downloaded,
			&s.CountedDownloaded, &s.Announces, &s.SeedAnnounces); err != nil {
			t.Fatalf("scanning user_period_stats: %v", err)
		}
		out[s.YearMonth] = s
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating user_period_stats: %v", err)
	}
	return out
}

type announceSpec struct {
	at                time.Time
	up, down, counted int64
	seeder            bool
}

func createAnnounces(t *testing.T, db *sql.DB, userID, torrentID int64, specs ...announceSpec) {
	t.Helper()
	repo := NewAnnounceEventRepo(db)
	for i, s := range specs {
		if err := repo.Create(context.Background(), &model.AnnounceEvent{
			UserID: userID, TorrentID: &torrentID, PeerID: []byte("peer-aaaaaaaaaaaaaa"),
			IP: "10.0.0.1", Port: 6881, Event: "announce",
			UploadedDelta: s.up, DownloadedDelta: s.down, CountedDownloadedDelta: s.counted,
			Seeder: s.seeder, AnnouncedAt: s.at,
		}); err != nil {
			t.Fatalf("Create(announce %d): %v", i, err)
		}
	}
}

func TestAnnounceRollupRepo_AggregatesByUTCMonth(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)
	setRollupWatermark(t, db, utcDate(2026, time.June, 1))

	createAnnounces(t, db, user.ID, torrent.ID,
		announceSpec{at: utc(2026, time.June, 15, 10, 0), up: 100, down: 50, counted: 50},
		announceSpec{at: utc(2026, time.June, 20, 10, 0), up: 200, seeder: true},
		announceSpec{at: utc(2026, time.July, 5, 10, 0), up: 300, down: 100, counted: 30, seeder: true},
	)

	repo := NewAnnounceRollupRepo(db)

	// Two chunks, so the month straddling the chunk boundary is exercised: a
	// month aggregated across two runs must add, not overwrite.
	first, err := repo.Rollup(ctx, utcDate(2026, time.August, 1), 31)
	if err != nil {
		t.Fatalf("Rollup(first): %v", err)
	}
	if first.CaughtUp {
		t.Fatal("first chunk should not have reached August in 31 days from June 1")
	}
	if !first.From.Equal(utcDate(2026, time.June, 1)) || !first.To.Equal(utcDate(2026, time.July, 2)) {
		t.Fatalf("first chunk covered [%v, %v), want [2026-06-01, 2026-07-02)", first.From, first.To)
	}

	second, err := repo.Rollup(ctx, utcDate(2026, time.August, 1), 31)
	if err != nil {
		t.Fatalf("Rollup(second): %v", err)
	}
	if !second.CaughtUp {
		t.Fatalf("second chunk should have caught up: %+v", second)
	}

	totals := periodTotals(t, db, user.ID)
	if len(totals) != 2 {
		t.Fatalf("expected 2 monthly rows, got %d: %+v", len(totals), totals)
	}

	june := totals["2026-06"]
	if june.Uploaded != 300 || june.Downloaded != 50 || june.CountedDownloaded != 50 {
		t.Errorf("June totals = up %d down %d counted %d, want 300/50/50",
			june.Uploaded, june.Downloaded, june.CountedDownloaded)
	}
	if june.Announces != 2 || june.SeedAnnounces != 1 {
		t.Errorf("June counts = %d announces, %d seeding; want 2 and 1", june.Announces, june.SeedAnnounces)
	}

	july := totals["2026-07"]
	if july.Uploaded != 300 || july.Downloaded != 100 || july.CountedDownloaded != 30 {
		t.Errorf("July totals = up %d down %d counted %d, want 300/100/30",
			july.Uploaded, july.Downloaded, july.CountedDownloaded)
	}

	rolled, err := repo.RolledThrough(ctx)
	if err != nil {
		t.Fatalf("RolledThrough: %v", err)
	}
	if !rolled.Equal(utcDate(2026, time.August, 1)) {
		t.Errorf("watermark = %v, want 2026-08-01", rolled)
	}
}

// The month boundary is UTC, not the server's timezone. Two announces half an hour
// either side of midnight on the first belong to different months, and a database
// session in, say, Europe/Berlin must not move one of them.
func TestAnnounceRollupRepo_MonthBoundaryIsUTC(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)
	setRollupWatermark(t, db, utcDate(2026, time.June, 1))

	if _, err := db.Exec(`SET TIME ZONE 'Europe/Berlin'`); err != nil {
		t.Fatalf("setting session timezone: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`SET TIME ZONE 'UTC'`) })

	createAnnounces(t, db, user.ID, torrent.ID,
		announceSpec{at: utc(2026, time.June, 30, 23, 30), up: 11},
		announceSpec{at: utc(2026, time.July, 1, 0, 30), up: 22},
	)

	if _, err := NewAnnounceRollupRepo(db).Rollup(ctx, utcDate(2026, time.August, 1), 62); err != nil {
		t.Fatalf("Rollup: %v", err)
	}

	totals := periodTotals(t, db, user.ID)
	if totals["2026-06"].Uploaded != 11 {
		t.Errorf("June uploaded = %d, want 11", totals["2026-06"].Uploaded)
	}
	if totals["2026-07"].Uploaded != 22 {
		t.Errorf("July uploaded = %d, want 22", totals["2026-07"].Uploaded)
	}
}

// Today is open: an announce can still land in it. Aggregating it would either
// double-count when the day closes or leave the day partially counted forever.
func TestAnnounceRollupRepo_ExcludesTheOpenDay(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)
	setRollupWatermark(t, db, utcDate(2026, time.July, 1))

	createAnnounces(t, db, user.ID, torrent.ID,
		announceSpec{at: utc(2026, time.July, 9, 23, 59), up: 7},
		announceSpec{at: utc(2026, time.July, 10, 5, 0), up: 99},
	)

	if _, err := NewAnnounceRollupRepo(db).Rollup(ctx, utcDate(2026, time.July, 10), 31); err != nil {
		t.Fatalf("Rollup: %v", err)
	}

	if got := periodTotals(t, db, user.ID)["2026-07"].Uploaded; got != 7 {
		t.Errorf("July uploaded = %d, want 7 — the open day must not be counted", got)
	}
}

// A month is aggregated across as many chunks as it has days, so the conflict
// clause has to add. Written as its own test because the other cases happen to put
// each month inside a single chunk, where "add" and "replace" are indistinguishable
// — a replace would pass every one of them and lose all but the last chunk here.
func TestAnnounceRollupRepo_AddsAcrossChunksWithinOneMonth(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)
	setRollupWatermark(t, db, utcDate(2026, time.July, 1))

	// Ten-day chunks put these three announces in three separate runs, all in July.
	createAnnounces(t, db, user.ID, torrent.ID,
		announceSpec{at: utc(2026, time.July, 5, 10, 0), up: 100, down: 10, counted: 10},
		announceSpec{at: utc(2026, time.July, 15, 10, 0), up: 200, down: 20, counted: 20, seeder: true},
		announceSpec{at: utc(2026, time.July, 25, 10, 0), up: 400, down: 40, counted: 40, seeder: true},
	)

	repo := NewAnnounceRollupRepo(db)
	chunks := 0
	for {
		result, err := repo.Rollup(ctx, utcDate(2026, time.August, 1), 10)
		if err != nil {
			t.Fatalf("Rollup(chunk %d): %v", chunks, err)
		}
		chunks++
		if result.CaughtUp {
			break
		}
		if chunks > 10 {
			t.Fatal("rollup never caught up")
		}
	}
	if chunks < 3 {
		t.Fatalf("only %d chunks — the month was not split, so this proves nothing", chunks)
	}

	july := periodTotals(t, db, user.ID)["2026-07"]
	if july.Uploaded != 700 || july.Downloaded != 70 || july.CountedDownloaded != 70 {
		t.Errorf("July totals = up %d down %d counted %d, want 700/70/70 — chunks overwrote instead of adding",
			july.Uploaded, july.Downloaded, july.CountedDownloaded)
	}
	if july.Announces != 3 || july.SeedAnnounces != 2 {
		t.Errorf("July counts = %d announces, %d seeding; want 3 and 2", july.Announces, july.SeedAnnounces)
	}
}

// Re-running after the watermark has caught up must be a no-op. This is what lets
// the nightly job be retried, or fire twice, without inflating anyone's ratio.
//
// Note what this does NOT prove: the rollup is additive, so it is idempotent only
// because the watermark refuses to revisit a window. Moving `rolled_through`
// backwards by hand would double-count every day it re-covers, silently. That is
// not a bug this test can catch and not one the code can prevent — it is why the
// watermark is a single row nothing but Rollup writes.
func TestAnnounceRollupRepo_RerunAfterCatchUpIsANoOp(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)
	setRollupWatermark(t, db, utcDate(2026, time.July, 1))

	createAnnounces(t, db, user.ID, torrent.ID,
		announceSpec{at: utc(2026, time.July, 5, 10, 0), up: 500, down: 400, counted: 100, seeder: true},
	)

	repo := NewAnnounceRollupRepo(db)
	through := utcDate(2026, time.August, 1)

	for i := 0; i < 3; i++ {
		if _, err := repo.Rollup(ctx, through, 62); err != nil {
			t.Fatalf("Rollup(run %d): %v", i, err)
		}
	}

	july := periodTotals(t, db, user.ID)["2026-07"]
	if july.Uploaded != 500 || july.CountedDownloaded != 100 || july.Announces != 1 {
		t.Errorf("three runs produced up %d counted %d announces %d, want 500/100/1 — the rollup double-counted",
			july.Uploaded, july.CountedDownloaded, july.Announces)
	}
}

// A watermark ahead of `through` means the clock went backwards. Walking it back
// would re-count every day in between.
func TestAnnounceRollupRepo_DoesNotRewindTheWatermark(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	newUser(t, db) // a users row must exist for the FK if anything did aggregate
	setRollupWatermark(t, db, utcDate(2026, time.August, 1))

	repo := NewAnnounceRollupRepo(db)
	result, err := repo.Rollup(ctx, utcDate(2026, time.July, 1), 31)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if !result.CaughtUp || !result.From.Equal(result.To) {
		t.Errorf("expected a no-op, got %+v", result)
	}

	rolled, err := repo.RolledThrough(ctx)
	if err != nil {
		t.Fatalf("RolledThrough: %v", err)
	}
	if !rolled.Equal(utcDate(2026, time.August, 1)) {
		t.Errorf("watermark moved back to %v — days would be counted twice", rolled)
	}
}

// The reason the rollup exists: the monthly totals must still be right after the
// raw rows they came from have been deleted.
func TestAnnounceRollupRepo_TotalsSurvivePruning(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)
	setRollupWatermark(t, db, utcDate(2026, time.June, 1))

	createAnnounces(t, db, user.ID, torrent.ID,
		announceSpec{at: utc(2026, time.June, 10, 10, 0), up: 1000, down: 800, counted: 800},
		announceSpec{at: utc(2026, time.June, 11, 10, 0), up: 2000, seeder: true},
	)

	rollups := NewAnnounceRollupRepo(db)
	if _, err := rollups.Rollup(ctx, utcDate(2026, time.July, 1), 62); err != nil {
		t.Fatalf("Rollup: %v", err)
	}

	events := NewAnnounceEventRepo(db)
	deleted, err := events.DeleteOlderThan(ctx, utcDate(2026, time.July, 1), 1000)
	if err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted %d rows, want 2", deleted)
	}

	// And a later run over the now-empty window must not zero what it already counted.
	if _, err := rollups.Rollup(ctx, utcDate(2026, time.August, 1), 62); err != nil {
		t.Fatalf("Rollup(after prune): %v", err)
	}

	june := periodTotals(t, db, user.ID)["2026-06"]
	if june.Uploaded != 3000 || june.CountedDownloaded != 800 || june.Announces != 2 {
		t.Errorf("June totals after pruning = up %d counted %d announces %d, want 3000/800/2",
			june.Uploaded, june.CountedDownloaded, june.Announces)
	}
}

// Without a watermark there is no safe prune cutoff, so the read must fail rather
// than return a zero time that would read as "nothing has been aggregated" — or
// worse, be mistaken for a valid date.
func TestAnnounceRollupRepo_MissingWatermarkIsAnError(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	if _, err := db.Exec(`DELETE FROM announce_rollup_state`); err != nil {
		t.Fatalf("clearing watermark: %v", err)
	}

	_, err := NewAnnounceRollupRepo(db).RolledThrough(ctx)
	if err == nil {
		t.Fatal("expected an error with no watermark row")
	}
	// Mirrors what the rest of this package returns for an absent row, so callers
	// can tell "not set up" from "database unreachable".
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("error = %v, want it to wrap sql.ErrNoRows", err)
	}
}

func TestAnnounceRollupRepo_ListByUserNewestFirst(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	user := newUser(t, db)
	other := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)
	setRollupWatermark(t, db, utcDate(2026, time.May, 1))

	createAnnounces(t, db, user.ID, torrent.ID,
		announceSpec{at: utc(2026, time.May, 4, 10, 0), up: 1},
		announceSpec{at: utc(2026, time.June, 4, 10, 0), up: 2},
		announceSpec{at: utc(2026, time.July, 4, 10, 0), up: 3},
	)
	createAnnounces(t, db, other.ID, torrent.ID,
		announceSpec{at: utc(2026, time.June, 4, 10, 0), up: 999},
	)

	repo := NewAnnounceRollupRepo(db)
	for i := 0; i < 4; i++ {
		result, err := repo.Rollup(ctx, utcDate(2026, time.August, 1), 31)
		if err != nil {
			t.Fatalf("Rollup(%d): %v", i, err)
		}
		if result.CaughtUp {
			break
		}
	}

	periods, err := repo.ListByUser(ctx, user.ID, 12)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(periods) != 3 {
		t.Fatalf("got %d periods, want 3 (another member's month must not leak)", len(periods))
	}
	want := []string{"2026-07", "2026-06", "2026-05"}
	for i, ym := range want {
		if periods[i].YearMonth != ym {
			t.Errorf("periods[%d] = %q, want %q (newest first)", i, periods[i].YearMonth, ym)
		}
	}

	limited, err := repo.ListByUser(ctx, user.ID, 2)
	if err != nil {
		t.Fatalf("ListByUser(limit 2): %v", err)
	}
	if len(limited) != 2 || limited[0].YearMonth != "2026-07" {
		t.Errorf("limit not applied: %+v", limited)
	}
}

// Deleting an account must take its aggregates with it. The raw log already
// cascades; totals derived from it would otherwise outlive the erasure.
func TestUserPeriodStats_CascadeOnUserDelete(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)
	setRollupWatermark(t, db, utcDate(2026, time.July, 1))

	createAnnounces(t, db, user.ID, torrent.ID,
		announceSpec{at: utc(2026, time.July, 3, 10, 0), up: 1},
	)
	if _, err := NewAnnounceRollupRepo(db).Rollup(ctx, utcDate(2026, time.August, 1), 62); err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if len(periodTotals(t, db, user.ID)) != 1 {
		t.Fatal("expected one monthly row before the delete")
	}

	// The torrent references the uploader, so it goes first — as it would in any
	// real account removal.
	if err := NewTorrentRepo(db).Delete(ctx, torrent.ID); err != nil {
		t.Fatalf("deleting torrent: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("deleting user: %v", err)
	}

	if got := len(periodTotals(t, db, user.ID)); got != 0 {
		t.Errorf("%d monthly rows survived the account deletion", got)
	}
}

// year_month is the one column here that a bug could quietly corrupt — a wrong
// format string would sort wrongly and read wrongly forever. The check constraint
// is what makes that a write failure instead.
func TestUserPeriodStats_RejectsAMalformedMonth(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)

	user := newUser(t, db)
	for _, bad := range []string{"2026-13", "2026-00", "26-07", "2026-7 "} {
		if _, err := db.Exec(
			`INSERT INTO user_period_stats (user_id, year_month) VALUES ($1, $2)`, user.ID, bad); err == nil {
			t.Errorf("year_month %q was accepted", bad)
			if _, derr := db.Exec(`DELETE FROM user_period_stats WHERE user_id = $1`, user.ID); derr != nil {
				t.Fatalf("cleanup: %v", derr)
			}
		}
	}
}

func TestAnnounceEventRepo_DeleteOlderThan(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)

	createAnnounces(t, db, user.ID, torrent.ID,
		announceSpec{at: utc(2026, time.January, 1, 10, 0)},
		announceSpec{at: utc(2026, time.January, 2, 10, 0)},
		announceSpec{at: utc(2026, time.January, 3, 10, 0)},
		announceSpec{at: utc(2026, time.June, 1, 10, 0)},
	)

	repo := NewAnnounceEventRepo(db)

	// The limit bounds the statement: this is what makes the nightly prune chunked
	// rather than one DELETE over a year of rows.
	deleted, err := repo.DeleteOlderThan(ctx, utcDate(2026, time.February, 1), 2)
	if err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted %d rows, want the 2 the limit allows", deleted)
	}

	deleted, err = repo.DeleteOlderThan(ctx, utcDate(2026, time.February, 1), 100)
	if err != nil {
		t.Fatalf("DeleteOlderThan(rest): %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted %d rows, want the 1 remaining January row", deleted)
	}

	// Oldest-first, so the survivor is the June row and not an arbitrary one.
	_, total, err := repo.ListByUser(ctx, user.ID, 1, 25)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if total != 1 {
		t.Fatalf("%d rows left, want 1", total)
	}

	// Nothing older than the cutoff is left, so a further pass is a no-op rather
	// than reaching forward into rows that are still inside the window.
	deleted, err = repo.DeleteOlderThan(ctx, utcDate(2026, time.February, 1), 100)
	if err != nil {
		t.Fatalf("DeleteOlderThan(empty): %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted %d rows past the cutoff, want 0", deleted)
	}

	// A non-positive limit must not be read as "no limit".
	deleted, err = repo.DeleteOlderThan(ctx, utcDate(2027, time.January, 1), 0)
	if err != nil {
		t.Fatalf("DeleteOlderThan(limit 0): %v", err)
	}
	if deleted != 0 {
		t.Errorf("a zero limit deleted %d rows", deleted)
	}
	if _, total, _ = repo.ListByUser(ctx, user.ID, 1, 25); total != 1 {
		t.Errorf("a zero limit removed rows: %d left, want 1", total)
	}
}

func TestAnnounceEventRepo_PageByUser(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	user := newUser(t, db)
	other := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)

	createAnnounces(t, db, user.ID, torrent.ID,
		announceSpec{at: utc(2026, time.July, 1, 10, 0), up: 1},
		announceSpec{at: utc(2026, time.July, 2, 10, 0), up: 2},
		announceSpec{at: utc(2026, time.July, 3, 10, 0), up: 3},
	)
	createAnnounces(t, db, other.ID, torrent.ID,
		announceSpec{at: utc(2026, time.July, 2, 11, 0), up: 999},
	)

	repo := NewAnnounceEventRepo(db)

	first, err := repo.PageByUser(ctx, user.ID, 0, 2)
	if err != nil {
		t.Fatalf("PageByUser(first): %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("got %d rows, want 2", len(first))
	}
	// Ascending, oldest first — an export is a chronological walk.
	if first[0].UploadedDelta != 1 || first[1].UploadedDelta != 2 {
		t.Errorf("wrong order: %d then %d, want 1 then 2", first[0].UploadedDelta, first[1].UploadedDelta)
	}

	second, err := repo.PageByUser(ctx, user.ID, first[1].ID, 2)
	if err != nil {
		t.Fatalf("PageByUser(second): %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("got %d rows on the second page, want 1 (the other member's row must not leak)", len(second))
	}
	if second[0].UploadedDelta != 3 {
		t.Errorf("second page = %d, want 3", second[0].UploadedDelta)
	}
	if second[0].TorrentName != torrent.Name {
		t.Errorf("TorrentName = %q, want %q", second[0].TorrentName, torrent.Name)
	}

	empty, err := repo.PageByUser(ctx, user.ID, second[0].ID, 2)
	if err != nil {
		t.Fatalf("PageByUser(exhausted): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected the walk to end, got %d rows", len(empty))
	}
}

// announce_events is the biggest table on the site, and the listing pages it with
// OFFSET. A page past the end must not become an index walk over everything before
// it — the total is already counted, so the query can be skipped entirely.
func TestAnnounceEventRepo_ListByUserPastTheEnd(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)
	createAnnounces(t, db, user.ID, torrent.ID,
		announceSpec{at: utc(2026, time.July, 1, 10, 0)},
		announceSpec{at: utc(2026, time.July, 2, 10, 0)},
	)

	repo := NewAnnounceEventRepo(db)

	events, total, err := repo.ListByUser(ctx, user.ID, 9_999_999, 100)
	if err != nil {
		t.Fatalf("ListByUser(deep page): %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d rows past the end, want none", len(events))
	}
	// The total still has to be right, or the frontend cannot render pagination.
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}

	// The last real page still works — the short-circuit must not swallow it.
	events, _, err = repo.ListByUser(ctx, user.ID, 2, 1)
	if err != nil {
		t.Fatalf("ListByUser(last page): %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d rows on the last page, want 1", len(events))
	}

	// A user with no announces at all: offset 0 against total 0 is also "past the
	// end", and must be an empty page rather than an error.
	other := newUser(t, db)
	events, total, err = repo.ListByUser(ctx, other.ID, 1, 25)
	if err != nil {
		t.Fatalf("ListByUser(empty log): %v", err)
	}
	if len(events) != 0 || total != 0 {
		t.Errorf("empty log returned %d rows, total %d", len(events), total)
	}
}

// The interface is satisfied by the concrete types, which is what main.go relies
// on. Checked here so a signature drift is a compile error in this package rather
// than in cmd/server, which coverage excludes.
var (
	_ repository.AnnounceEventRepository  = (*AnnounceEventRepo)(nil)
	_ repository.AnnounceRollupRepository = (*AnnounceRollupRepo)(nil)
)
