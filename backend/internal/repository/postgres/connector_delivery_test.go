package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func pendingDelivery(instanceID int64, eventKey string) *model.ConnectorDelivery {
	return &model.ConnectorDelivery{
		InstanceID: instanceID,
		EventKey:   eventKey,
		EventType:  "torrent.published",
		Payload:    json.RawMessage(fmt.Sprintf(`{"event":"torrent.published","name":%q}`, eventKey)),
		Status:     model.DeliveryPending,
	}
}

// Dedupe is enforced by the unique index, not by a check-then-insert: concurrent
// dispatches race, and a read-first guard would let both through.
func TestConnectorDeliveryInsertPendingIsIdempotent(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	inst := newConnector(t, db, "webhook", "Hook", true)

	first := pendingDelivery(inst.ID, "torrent.published:1")
	inserted, err := repo.InsertPending(ctx, first)
	if err != nil || !inserted {
		t.Fatalf("first InsertPending = %v, %v; want true, nil", inserted, err)
	}
	if first.ID == 0 {
		t.Fatal("InsertPending did not populate the ID")
	}

	second := pendingDelivery(inst.ID, "torrent.published:1")
	inserted, err = repo.InsertPending(ctx, second)
	if err != nil {
		t.Fatalf("duplicate InsertPending errored: %v", err)
	}
	if inserted {
		t.Fatal("a duplicate (instance, event_key) must report inserted=false, not create a second row")
	}

	// A different instance is a different destination, so it gets its own row.
	other := newConnector(t, db, "webhook", "Other Hook", true)
	inserted, err = repo.InsertPending(ctx, pendingDelivery(other.ID, "torrent.published:1"))
	if err != nil || !inserted {
		t.Fatalf("other instance InsertPending = %v, %v; want true, nil", inserted, err)
	}
}

func TestConnectorDeliveryListDueRespectsStatusAndSchedule(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	inst := newConnector(t, db, "webhook", "Hook", true)

	due := pendingDelivery(inst.ID, "torrent.published:1")
	mustInsert(t, repo, due)

	sent := pendingDelivery(inst.ID, "torrent.published:2")
	mustInsert(t, repo, sent)
	if err := repo.MarkSent(ctx, sent.ID, model.DeliverySent); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	future := pendingDelivery(inst.ID, "torrent.published:3")
	later := time.Now().Add(time.Hour)
	future.NextAttemptAt = &later
	mustInsert(t, repo, future)

	rows, err := repo.ListDue(ctx, inst.ID, time.Now(), 100)
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != due.ID {
		t.Fatalf("ListDue returned %+v, want only the row that is actually due", rows)
	}
}

func TestConnectorDeliveryListDueIsOrderedAndLimited(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	inst := newConnector(t, db, "webhook", "Hook", true)
	var ids []int64
	for i := 1; i <= 5; i++ {
		d := pendingDelivery(inst.ID, fmt.Sprintf("torrent.published:%d", i))
		mustInsert(t, repo, d)
		ids = append(ids, d.ID)
	}

	rows, err := repo.ListDue(ctx, inst.ID, time.Now(), 3)
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("ListDue returned %d rows, want the limit of 3", len(rows))
	}
	// Oldest first, so announcements go out in the order they happened.
	for i, row := range rows {
		if row.ID != ids[i] {
			t.Fatalf("row %d has ID %d, want %d (insertion order)", i, row.ID, ids[i])
		}
	}
}

func TestConnectorDeliveryCountSentSince(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	inst := newConnector(t, db, "webhook", "Hook", true)

	sent := pendingDelivery(inst.ID, "torrent.published:1")
	mustInsert(t, repo, sent)
	if err := repo.MarkSent(ctx, sent.ID, model.DeliverySent); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	// Coalesced rows must NOT count: a whole batch of them was one "+N more"
	// message on the wire, and charging each row would stall the instance for
	// minutes after every burst.
	for _, key := range []string{"torrent.published:2", "torrent.published:3", "torrent.published:4"} {
		coalesced := pendingDelivery(inst.ID, key)
		mustInsert(t, repo, coalesced)
		if err := repo.MarkSent(ctx, coalesced.ID, model.DeliveryCoalesced); err != nil {
			t.Fatalf("MarkSent(coalesced): %v", err)
		}
	}

	// Still pending: not sent, so it must not count against the budget either.
	mustInsert(t, repo, pendingDelivery(inst.ID, "torrent.published:5"))

	n, err := repo.CountSentSince(ctx, inst.ID, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("CountSentSince: %v", err)
	}
	if n != 1 {
		t.Fatalf("CountSentSince = %d, want 1 (only the individually sent row)", n)
	}

	// Outside the window.
	n, err = repo.CountSentSince(ctx, inst.ID, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("CountSentSince: %v", err)
	}
	if n != 0 {
		t.Fatalf("CountSentSince (future window) = %d, want 0", n)
	}
}

func TestConnectorDeliveryMarkFailedAttempt(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	inst := newConnector(t, db, "webhook", "Hook", true)

	retrying := pendingDelivery(inst.ID, "torrent.published:1")
	mustInsert(t, repo, retrying)
	next := time.Now().Add(30 * time.Second)
	if err := repo.MarkFailedAttempt(ctx, retrying.ID, 1, "endpoint down", &next); err != nil {
		t.Fatalf("MarkFailedAttempt: %v", err)
	}

	rows, _, err := repo.ListByInstance(ctx, inst.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Status != model.DeliveryPending {
		t.Fatalf("status = %q, want it to stay pending while a retry is scheduled", rows[0].Status)
	}
	if rows[0].Attempts != 1 || rows[0].LastError == nil || *rows[0].LastError != "endpoint down" {
		t.Fatalf("row = %+v, want the attempt and error recorded", rows[0])
	}
	if rows[0].NextAttemptAt == nil {
		t.Fatal("next_attempt_at not persisted")
	}

	// A nil next attempt is the dead-letter path.
	if err := repo.MarkFailedAttempt(ctx, retrying.ID, 5, "gave up", nil); err != nil {
		t.Fatalf("MarkFailedAttempt(dead-letter): %v", err)
	}
	rows, _, err = repo.ListByInstance(ctx, inst.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if rows[0].Status != model.DeliveryFailed || rows[0].NextAttemptAt != nil {
		t.Fatalf("row = %+v, want a dead-lettered failed row with no next attempt", rows[0])
	}
}

func TestConnectorDeliveryMarkOnMissingRow(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	if err := repo.MarkSent(ctx, 9999, model.DeliverySent); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("MarkSent(missing) = %v, want sql.ErrNoRows", err)
	}
	if err := repo.MarkFailedAttempt(ctx, 9999, 1, "x", nil); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("MarkFailedAttempt(missing) = %v, want sql.ErrNoRows", err)
	}
}

func TestConnectorDeliveryListByInstancePaginates(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	inst := newConnector(t, db, "webhook", "Hook", true)
	for i := 1; i <= 5; i++ {
		mustInsert(t, repo, pendingDelivery(inst.ID, fmt.Sprintf("torrent.published:%d", i)))
	}

	rows, total, err := repo.ListByInstance(ctx, inst.ID, 1, 2)
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if total != 5 || len(rows) != 2 {
		t.Fatalf("page 1: %d rows of %d total, want 2 of 5", len(rows), total)
	}

	page3, total, err := repo.ListByInstance(ctx, inst.ID, 3, 2)
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if total != 5 || len(page3) != 1 {
		t.Fatalf("page 3: %d rows of %d total, want 1 of 5", len(page3), total)
	}
}

func TestConnectorDeliveryLatestStatusByInstance(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	a := newConnector(t, db, "webhook", "Hook A", true)
	b := newConnector(t, db, "chat", "Shoutbox", true)

	older := pendingDelivery(a.ID, "torrent.published:1")
	mustInsert(t, repo, older)
	newer := pendingDelivery(a.ID, "torrent.published:2")
	mustInsert(t, repo, newer)
	if err := repo.MarkSent(ctx, newer.ID, model.DeliverySent); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	mustInsert(t, repo, pendingDelivery(b.ID, "torrent.published:1"))

	latest, err := repo.LatestStatusByInstance(ctx)
	if err != nil {
		t.Fatalf("LatestStatusByInstance: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("got %d instances, want 2", len(latest))
	}
	if latest[a.ID].ID != newer.ID {
		t.Fatalf("instance A latest = %d, want the newest row %d", latest[a.ID].ID, newer.ID)
	}
}

func TestConnectorDeliveryInstancesWithDue(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	withDue := newConnector(t, db, "webhook", "Has Work", true)
	allSent := newConnector(t, db, "chat", "Nothing Due", true)

	mustInsert(t, repo, pendingDelivery(withDue.ID, "torrent.published:1"))
	mustInsert(t, repo, pendingDelivery(withDue.ID, "torrent.published:2"))

	done := pendingDelivery(allSent.ID, "torrent.published:1")
	mustInsert(t, repo, done)
	if err := repo.MarkSent(ctx, done.ID, model.DeliverySent); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	ids, err := repo.InstancesWithDue(ctx, time.Now())
	if err != nil {
		t.Fatalf("InstancesWithDue: %v", err)
	}
	if len(ids) != 1 || ids[0] != withDue.ID {
		t.Fatalf("InstancesWithDue = %v, want [%d] and no duplicates", ids, withDue.ID)
	}
}

// Retention must never be the reason an announcement went missing, so pending
// rows survive pruning regardless of age.
func TestConnectorDeliveryDeleteOldSparesPendingRows(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	inst := newConnector(t, db, "webhook", "Hook", true)

	oldSent := pendingDelivery(inst.ID, "torrent.published:1")
	mustInsert(t, repo, oldSent)
	if err := repo.MarkSent(ctx, oldSent.ID, model.DeliverySent); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	oldPending := pendingDelivery(inst.ID, "torrent.published:2")
	mustInsert(t, repo, oldPending)
	recentSent := pendingDelivery(inst.ID, "torrent.published:3")
	mustInsert(t, repo, recentSent)
	if err := repo.MarkSent(ctx, recentSent.ID, model.DeliverySent); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	if _, err := db.ExecContext(ctx,
		`UPDATE connector_deliveries SET created_at = NOW() - INTERVAL '60 days' WHERE id = ANY($1)`,
		fmt.Sprintf("{%d,%d}", oldSent.ID, oldPending.ID),
	); err != nil {
		t.Fatalf("backdating rows: %v", err)
	}

	deleted, err := repo.DeleteOld(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("DeleteOld: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted %d rows, want 1 (the old sent one)", deleted)
	}

	rows, total, err := repo.ListByInstance(ctx, inst.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByInstance: %v", err)
	}
	if total != 2 {
		t.Fatalf("%d rows left, want 2", total)
	}
	for _, row := range rows {
		if row.ID == oldSent.ID {
			t.Fatal("the old sent row should have been pruned")
		}
	}
}

// Deleting an instance takes its whole delivery history with it, so the worker
// never finds orphaned work.
func TestConnectorDeliveryCascadesOnInstanceDelete(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	inst := newConnector(t, db, "webhook", "Hook", true)
	mustInsert(t, repo, pendingDelivery(inst.ID, "torrent.published:1"))

	if err := NewConnectorRepo(db).Delete(ctx, inst.ID); err != nil {
		t.Fatalf("Delete connector: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM connector_deliveries WHERE instance_id = $1`, inst.ID,
	).Scan(&count); err != nil {
		t.Fatalf("counting deliveries: %v", err)
	}
	if count != 0 {
		t.Fatalf("%d delivery rows survived the instance delete, want 0", count)
	}
}

func mustInsert(t *testing.T, repo interface {
	InsertPending(context.Context, *model.ConnectorDelivery) (bool, error)
}, d *model.ConnectorDelivery) {
	t.Helper()
	inserted, err := repo.InsertPending(context.Background(), d)
	if err != nil {
		t.Fatalf("InsertPending: %v", err)
	}
	if !inserted {
		t.Fatalf("InsertPending(%s) reported a duplicate", d.EventKey)
	}
}

// The claim is atomic: of two racing drains, exactly one gets the row.
func TestConnectorDeliveryClaimIsExclusive(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	inst := newConnector(t, db, "webhook", "Hook", true)
	row := pendingDelivery(inst.ID, "torrent.published:1")
	mustInsert(t, repo, row)

	now := time.Now()
	lease := now.Add(30 * time.Second)

	claimed, err := repo.ClaimForDelivery(ctx, row.ID, lease, now)
	if err != nil || !claimed {
		t.Fatalf("first claim = %v, %v; want true, nil", claimed, err)
	}

	claimed, err = repo.ClaimForDelivery(ctx, row.ID, lease, now)
	if err != nil {
		t.Fatalf("second claim errored: %v", err)
	}
	if claimed {
		t.Fatal("a second drain must not be able to claim a leased row")
	}

	// A leased row is hidden from ListDue for the duration of the lease.
	rows, err := repo.ListDue(ctx, inst.ID, now, 10)
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("ListDue returned %d leased rows, want 0", len(rows))
	}

	// Once the lease expires the row becomes due again, which is how a worker
	// that died mid-delivery self-heals.
	afterLease := lease.Add(time.Second)
	rows, err = repo.ListDue(ctx, inst.ID, afterLease, 10)
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListDue after lease expiry returned %d rows, want 1", len(rows))
	}
}

func TestConnectorDeliveryClaimRejectsClosedRow(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorDeliveryRepo(db)

	inst := newConnector(t, db, "webhook", "Hook", true)
	row := pendingDelivery(inst.ID, "torrent.published:1")
	mustInsert(t, repo, row)
	if err := repo.MarkSent(ctx, row.ID, model.DeliverySent); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	now := time.Now()
	claimed, err := repo.ClaimForDelivery(ctx, row.ID, now.Add(time.Minute), now)
	if err != nil {
		t.Fatalf("ClaimForDelivery: %v", err)
	}
	if claimed {
		t.Fatal("an already-sent row must not be claimable")
	}
}
