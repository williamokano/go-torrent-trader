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
	after := announceIndexes(t)
	if _, still := after[leftover]; still {
		t.Errorf("%q survived the rebuild, want it dropped", leftover)
	}
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

// What a real cancellation actually leaves. The two single-leftover tests above
// each cover one arm; a genuine failure mid-REINDEX TABLE strands a marker for
// every index at once, and the primary key's is the one worth pinning — its
// constraint dependency moves to the swapped-in index, so the leftover is
// free-standing and can be dropped, but that is exactly the kind of claim that
// should be demonstrated rather than reasoned about.
func TestAnnounceEventRepo_ReindexClearsAWholeFailedRunsWreckage(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewAnnounceEventRepo(db)

	leftovers := map[string]string{
		"announce_events_pkey_ccold":             "(id)",
		"idx_announce_events_user_ccnew":         "(user_id, announced_at DESC)",
		"idx_announce_events_announced_at_ccold": "(announced_at)",
		// The counter form PostgreSQL uses when the name is already taken, which
		// is what a second consecutive failure produces.
		"idx_announce_events_announced_at_ccnew1": "(announced_at)",
	}
	for name, cols := range leftovers {
		if _, err := db.ExecContext(ctx, `CREATE INDEX `+name+` ON announce_events `+cols); err != nil {
			t.Fatalf("creating leftover %s: %v", name, err)
		}
		if _, err := db.ExecContext(ctx,
			`UPDATE pg_index SET indisvalid = false WHERE indexrelid = $1::regclass`, name); err != nil {
			t.Fatalf("marking %s invalid: %v", name, err)
		}
		t.Cleanup(func() {
			_, _ = db.ExecContext(context.Background(), `DROP INDEX IF EXISTS `+name)
		})
	}

	result, err := repo.Reindex(ctx)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if result.LeftoversDropped != len(leftovers) {
		t.Errorf("LeftoversDropped = %d, want %d", result.LeftoversDropped, len(leftovers))
	}

	after := announceIndexes(t)
	for name := range leftovers {
		if _, still := after[name]; still {
			t.Errorf("%q survived the rebuild", name)
		}
	}
	for _, want := range []string{
		"announce_events_pkey",
		"idx_announce_events_user",
		"idx_announce_events_announced_at",
	} {
		if info, ok := after[want]; !ok {
			t.Errorf("the cleanup removed the real index %q", want)
		} else if !info.valid {
			t.Errorf("index %q is invalid after the rebuild", want)
		}
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

// The advisory lock is what makes this job safe to schedule rather than merely
// well-timed. Two rebuilds on one table do not simply queue: the cleanup goes
// straight for the "_ccnew" indexes a live rebuild is currently building, and
// PostgreSQL resolves that as a deadlock. Which side loses is not deterministic,
// so an unguarded second run can take the real rebuild down instead of itself.
//
// The lock is session-scoped, which is why Reindex pins a connection. Holding it
// from a separate connection here is exactly the state a concurrent run sees.
func TestAnnounceEventRepo_ReindexSkipsWhenAnotherRebuildHoldsTheLock(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	holder, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("opening the holding connection: %v", err)
	}
	defer func() { _ = holder.Close() }()

	var held bool
	if err := holder.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock($1)`, announceReindexLockKey).Scan(&held); err != nil {
		t.Fatalf("taking the lock: %v", err)
	}
	if !held {
		t.Fatal("could not take the lock, so the contended path would not be exercised")
	}
	defer func() {
		if _, err := holder.ExecContext(context.Background(),
			`SELECT pg_advisory_unlock($1)`, announceReindexLockKey); err != nil {
			t.Errorf("releasing the lock: %v", err)
		}
	}()

	before := announceIndexes(t)

	result, err := NewAnnounceEventRepo(db).Reindex(ctx)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if !result.Skipped {
		t.Error("Skipped = false, want true — the rebuild ran while another held the lock")
	}

	// Skipping has to mean skipping: nothing rebuilt, nothing dropped.
	if result.LeftoversDropped != 0 {
		t.Errorf("LeftoversDropped = %d while skipping, want 0", result.LeftoversDropped)
	}
	for name, info := range announceIndexes(t) {
		if was, ok := before[name]; ok && was.relfilenode != info.relfilenode {
			t.Errorf("index %q was rebuilt despite the lock being held", name)
		}
	}
}

// advisoryLocksHeld counts sessions holding the rebuild lock, read from pg_locks
// rather than by trying to take it.
//
// That distinction is the whole test. PostgreSQL advisory locks are re-entrant
// per session, and database/sql hands the same pooled connection back, so a
// second Reindex re-acquires a leaked lock happily and reports success. Asking
// "can I take it?" therefore cannot detect a leak — only looking at who holds it
// can. This is the trap tasks/lessons.md records against the leader-election
// test, and the first cut of this test walked straight into it: removing the
// unlock entirely left it green.
func advisoryLocksHeld(t *testing.T) int {
	t.Helper()
	db := requireDB(t)

	var n int
	if err := db.QueryRowContext(context.Background(), `
		SELECT count(*) FROM pg_locks
		 WHERE locktype = 'advisory'
		   AND ((classid::bigint << 32) | objid::bigint) = $1`,
		announceReindexLockKey).Scan(&n); err != nil {
		t.Fatalf("reading pg_locks: %v", err)
	}
	return n
}

// The lock must not outlive the call. A leaked one is not merely untidy: a
// second worker process, or the operator's own psql session, would then skip
// every rebuild forever while the job kept reporting that it had run.
func TestAnnounceEventRepo_ReindexReleasesTheLock(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	if held := advisoryLocksHeld(t); held != 0 {
		t.Fatalf("%d sessions already hold the rebuild lock before the test", held)
	}

	result, err := NewAnnounceEventRepo(db).Reindex(ctx)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if result.Skipped {
		t.Fatal("the rebuild skipped; nothing should have held the lock")
	}

	if held := advisoryLocksHeld(t); held != 0 {
		t.Errorf("%d sessions still hold the rebuild lock after Reindex returned", held)
	}
}

// The case the release matters most in: cancelled *after* the lock was taken.
// That is what the task timeout does, and it is why the unlock runs on
// context.WithoutCancel — on the original context the unlock statement is itself
// cancelled, so the lock leaks precisely on the failure path nobody watches.
//
// Cancelling before the call proves nothing (the lock is never taken, so there
// is nothing to leak), so the rebuild is deliberately parked: another session
// holds ACCESS EXCLUSIVE on the table, which REINDEX must wait for. That gives a
// deterministic window in which the advisory lock is held and the rebuild is
// not yet finished.
func TestAnnounceEventRepo_ReindexReleasesTheLockAfterCancellation(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)

	if held := advisoryLocksHeld(t); held != 0 {
		t.Fatalf("%d sessions already hold the rebuild lock before the test", held)
	}

	// Park the rebuild behind a conflicting lock.
	blocker, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("opening the blocking connection: %v", err)
	}
	defer func() { _ = blocker.Close() }()
	if _, err := blocker.ExecContext(context.Background(), `BEGIN`); err != nil {
		t.Fatalf("beginning the blocking transaction: %v", err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			_, _ = blocker.ExecContext(context.Background(), `ROLLBACK`)
		}
	}
	defer release()
	if _, err := blocker.ExecContext(context.Background(),
		`LOCK TABLE announce_events IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("taking the blocking lock: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Bounded, because the subject can block: a regression here must report as a
	// named failure rather than a stalled package.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = NewAnnounceEventRepo(db).Reindex(ctx)
	}()

	// Wait until the advisory lock is actually held, so the cancellation lands
	// inside the window this test exists to cover.
	deadline := time.Now().Add(10 * time.Second)
	for advisoryLocksHeld(t) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the rebuild never took the advisory lock")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	release()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Reindex did not return after cancellation")
	}

	// Polled, not asserted immediately. A cancellation that lands mid-statement
	// makes database/sql close the pinned connection, so the explicit unlock
	// fails and the release happens when the session ends instead — which is
	// reliable but not synchronous. What must hold is that the lock does not
	// *stay* held, since that would block every future rebuild.
	deadline = time.Now().Add(15 * time.Second)
	for {
		held := advisoryLocksHeld(t)
		if held == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Errorf("%d sessions still hold the rebuild lock 15s after a cancelled rebuild", held)
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
}
