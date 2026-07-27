package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// indexInfo is what the catalogue says about one index.
//
// relfilenode is the load-bearing field. "Reindex returned nil" is satisfied by
// a Reindex that did nothing at all, and on a table whose indexes are already
// valid, so is every assertion about validity. A real rebuild writes a new
// physical file and the relfilenode changes; that is the only observation here
// that a no-op cannot produce.
type indexInfo struct {
	valid       bool
	relfilenode int64
}

// announceIndexes returns every index on announce_events, so the assertions
// below read the catalogue rather than the absence of an error.
func announceIndexes(t *testing.T) map[string]indexInfo {
	t.Helper()
	db := requireDB(t)

	result, err := db.QueryContext(context.Background(), `
		SELECT c.relname, i.indisvalid, c.relfilenode
		  FROM pg_class c
		  JOIN pg_index i ON i.indexrelid = c.oid
		  JOIN pg_class tbl ON tbl.oid = i.indrelid
		 WHERE tbl.relname = 'announce_events'`)
	if err != nil {
		t.Fatalf("listing indexes: %v", err)
	}
	defer func() { _ = result.Close() }()

	out := map[string]indexInfo{}
	for result.Next() {
		var name string
		var info indexInfo
		if err := result.Scan(&name, &info.valid, &info.relfilenode); err != nil {
			t.Fatalf("scanning index row: %v", err)
		}
		out[name] = info
	}
	if err := result.Err(); err != nil {
		t.Fatalf("iterating indexes: %v", err)
	}
	return out
}

// TestAnnounceEventRepo_ReindexRebuildsEveryIndex drives the real statement
// against a real server. The interesting part is not that it returns nil — a
// REINDEX that silently did nothing would too — but that all three indexes come
// back valid and the table is still queryable through them afterwards.
func TestAnnounceEventRepo_ReindexRebuildsEveryIndex(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewAnnounceEventRepo(db)
	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)

	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	for i := 0; i < 50; i++ {
		if err := repo.Create(ctx, &model.AnnounceEvent{
			UserID: user.ID, TorrentID: &torrent.ID, PeerID: []byte("peer-aaaaaaaaaaaaaa"),
			Port: 6881, Event: "announce", AnnouncedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("Create(%d): %v", i, err)
		}
	}

	before := announceIndexes(t)
	// Guards the guard: if the schema ever stops having these, this test would
	// otherwise pass by asserting nothing.
	for _, want := range []string{
		"announce_events_pkey",
		"idx_announce_events_user",
		"idx_announce_events_announced_at",
	} {
		if _, ok := before[want]; !ok {
			t.Fatalf("expected index %q to exist before the rebuild; found %v", want, before)
		}
	}

	result, err := repo.Reindex(ctx)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if result.BytesBefore <= 0 || result.BytesAfter <= 0 {
		t.Errorf("Reindex reported before=%d after=%d, want both positive",
			result.BytesBefore, result.BytesAfter)
	}
	if result.LeftoversDropped != 0 {
		t.Errorf("LeftoversDropped = %d on a clean table, want 0", result.LeftoversDropped)
	}

	after := announceIndexes(t)
	for name, info := range after {
		if !info.valid {
			t.Errorf("index %q is invalid after the rebuild", name)
		}
		// The assertion that a no-op cannot satisfy: every index must sit on a new
		// physical file. Without this the test passes against a Reindex whose
		// REINDEX statement was never issued, since the indexes were already valid.
		if was, ok := before[name]; ok && was.relfilenode == info.relfilenode {
			t.Errorf("index %q kept relfilenode %d — it was not actually rebuilt",
				name, info.relfilenode)
		}
	}
	if len(after) != len(before) {
		t.Errorf("index count changed across the rebuild: %d -> %d (%v -> %v)",
			len(before), len(after), before, after)
	}

	// And the data is still reachable — a rebuild that produced valid but wrong
	// indexes would satisfy everything above.
	events, total, err := repo.ListByUser(ctx, user.ID, 1, 100)
	if err != nil {
		t.Fatalf("ListByUser after reindex: %v", err)
	}
	if total != 50 || len(events) != 50 {
		t.Errorf("after reindex: total=%d len=%d, want 50 and 50", total, len(events))
	}
}

// TestAnnounceEventRepo_ReindexClearsAFailedRunsWreckage is the case that makes
// this job safe to schedule unattended.
//
// PostgreSQL leaves an invalid "_ccnew" index behind whenever REINDEX
// CONCURRENTLY fails. It is dead weight the planner will not use but every
// insert still maintains, and nothing ever removes it. A monthly job that failed
// every month would accumulate one every month, so a run has to clear them
// before it starts.
//
// The invalid index is fabricated by marking a real one invalid in the
// catalogue, because provoking a genuine mid-rebuild failure is not something a
// test can do reliably. What matters is the state the cleanup encounters, and
// this reproduces it exactly.
func TestAnnounceEventRepo_ReindexClearsAFailedRunsWreckage(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewAnnounceEventRepo(db)
	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)
	if err := repo.Create(ctx, &model.AnnounceEvent{
		UserID: user.ID, TorrentID: &torrent.ID, PeerID: []byte("peer-aaaaaaaaaaaaaa"),
		Port: 6881, Event: "announce", AnnouncedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const leftover = "idx_announce_events_announced_at_ccnew"
	if _, err := db.ExecContext(ctx,
		`CREATE INDEX `+leftover+` ON announce_events (announced_at)`); err != nil {
		t.Fatalf("creating the leftover index: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE pg_index SET indisvalid = false WHERE indexrelid = $1::regclass`, leftover); err != nil {
		t.Fatalf("marking the leftover index invalid: %v", err)
	}
	// The Reindex under test is what normally removes this. If it fails, the
	// invalid index would otherwise survive in the package-wide database —
	// resetTestData truncates rows, not indexes — and the next test would fail on
	// an index count it never created, burying the real failure under a second one.
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP INDEX IF EXISTS `+leftover)
	})

	if got := announceIndexes(t); got[leftover].valid {
		t.Fatalf("setup failed: %q is still valid, so the cleanup path would not be exercised", leftover)
	}

	result, err := repo.Reindex(ctx)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if result.LeftoversDropped != 1 {
		t.Errorf("LeftoversDropped = %d, want 1", result.LeftoversDropped)
	}

	after := announceIndexes(t)
	if _, still := after[leftover]; still {
		t.Errorf("%q survived the rebuild, want it dropped", leftover)
	}
	// The real indexes must not have been caught by the same net.
	for _, want := range []string{
		"announce_events_pkey",
		"idx_announce_events_user",
		"idx_announce_events_announced_at",
	} {
		if info, ok := after[want]; !ok {
			t.Errorf("the cleanup removed %q, which it had no business touching", want)
		} else if !info.valid {
			t.Errorf("index %q is invalid after the rebuild", want)
		}
	}
}

// The other half of the same failure mode, and the expensive one. When a
// concurrent rebuild dies just after the swap rather than during the build, what
// it leaves is the *original* index, renamed with a "_ccold" marker and left
// invalid — a full-size copy of a live index rather than a partial stub, roughly
// doubling pg_indexes_size until something removes it. Nothing in PostgreSQL
// does, and a later rebuild skips it with a server notice the driver discards,
// so the job would report success while the dead bytes accumulated.
func TestAnnounceEventRepo_ReindexClearsACcoldLeftover(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewAnnounceEventRepo(db)

	const leftover = "idx_announce_events_user_ccold"
	if _, err := db.ExecContext(ctx,
		`CREATE INDEX `+leftover+` ON announce_events (user_id, announced_at DESC)`); err != nil {
		t.Fatalf("creating the leftover index: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE pg_index SET indisvalid = false WHERE indexrelid = $1::regclass`, leftover); err != nil {
		t.Fatalf("marking the leftover index invalid: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP INDEX IF EXISTS `+leftover)
	})

	result, err := repo.Reindex(ctx)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if result.LeftoversDropped != 1 {
		t.Errorf("LeftoversDropped = %d, want 1", result.LeftoversDropped)
	}
	if _, still := announceIndexes(t)[leftover]; still {
		t.Errorf("%q survived the rebuild, want it dropped", leftover)
	}
}

// A valid index whose name happens to carry one of those markers must survive.
// Invalidity is the predicate that makes the cleanup safe — an invalid index
// cannot be serving any query — and the name match only keeps it to PostgreSQL's
// own generated names. An index someone is building by hand is not this job's to
// delete, and the
// cleanup keys on both.
func TestAnnounceEventRepo_ReindexLeavesAValidLookalikeAlone(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewAnnounceEventRepo(db)

	const lookalike = "idx_announce_events_port_ccnew"
	if _, err := db.ExecContext(ctx,
		`CREATE INDEX `+lookalike+` ON announce_events (port)`); err != nil {
		t.Fatalf("creating the lookalike index: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP INDEX IF EXISTS `+lookalike)
	})

	result, err := repo.Reindex(ctx)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if result.LeftoversDropped != 0 {
		t.Errorf("LeftoversDropped = %d, want 0 — a valid index was dropped", result.LeftoversDropped)
	}
	if info, ok := announceIndexes(t)[lookalike]; !ok {
		t.Errorf("%q was dropped despite being valid", lookalike)
	} else if !info.valid {
		t.Errorf("%q is invalid after the rebuild", lookalike)
	}
}
