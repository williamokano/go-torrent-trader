package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

func bonusBalance(t *testing.T, db *sql.DB, userID int64) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(`SELECT bonus_points FROM users WHERE id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("read bonus balance: %v", err)
	}
	return n
}

func bonusLedger(t *testing.T, db *sql.DB, userID int64) []model.BonusTransaction {
	t.Helper()
	rows, err := db.Query(
		`SELECT id, user_id, delta, reason, ref_id FROM bonus_transactions WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []model.BonusTransaction
	for rows.Next() {
		var tr model.BonusTransaction
		if err := rows.Scan(&tr.ID, &tr.UserID, &tr.Delta, &tr.Reason, &tr.RefID); err != nil {
			t.Fatalf("scan ledger: %v", err)
		}
		out = append(out, tr)
	}
	return out
}

func itemBySlug(t *testing.T, db *sql.DB, slug string) *model.BonusStoreItem {
	t.Helper()
	var it model.BonusStoreItem
	err := db.QueryRow(
		`SELECT id, slug, kind, price, reward_amount, enabled FROM bonus_store_items WHERE slug = $1`, slug,
	).Scan(&it.ID, &it.Slug, &it.Kind, &it.Price, &it.RewardAmount, &it.Enabled)
	if err != nil {
		t.Fatalf("seeded item %q missing: %v", slug, err)
	}
	return &it
}

func TestBonusRepo_AwardPoints(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewBonusRepo(db)

	a := newUser(t, db)
	b := newUser(t, db)

	awards := map[int64]int64{a.ID: 10, b.ID: 3, 999999: 5} // unknown user skipped silently
	if err := repo.AwardPoints(ctx, awards, model.BonusReasonSeeding); err != nil {
		t.Fatalf("AwardPoints: %v", err)
	}

	if got := bonusBalance(t, db, a.ID); got != 10 {
		t.Errorf("user A balance = %d, want 10", got)
	}
	if got := bonusBalance(t, db, b.ID); got != 3 {
		t.Errorf("user B balance = %d, want 3", got)
	}

	ledger := bonusLedger(t, db, a.ID)
	if len(ledger) != 1 || ledger[0].Delta != 10 || ledger[0].Reason != model.BonusReasonSeeding {
		t.Errorf("user A ledger = %+v, want one seeding row of +10", ledger)
	}

	// Second cycle accumulates.
	if err := repo.AwardPoints(ctx, map[int64]int64{a.ID: 5}, model.BonusReasonSeeding); err != nil {
		t.Fatalf("AwardPoints second: %v", err)
	}
	if got := bonusBalance(t, db, a.ID); got != 15 {
		t.Errorf("user A balance after second cycle = %d, want 15", got)
	}
}

func TestBonusRepo_SetPoints(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewBonusRepo(db)

	u := newUser(t, db)
	admin := newUser(t, db)

	if err := repo.SetPoints(ctx, u.ID, 500, admin.ID); err != nil {
		t.Fatalf("SetPoints: %v", err)
	}
	if got := bonusBalance(t, db, u.ID); got != 500 {
		t.Fatalf("balance = %d, want 500", got)
	}
	ledger := bonusLedger(t, db, u.ID)
	if len(ledger) != 1 || ledger[0].Delta != 500 || ledger[0].Reason != model.BonusReasonAdminAdjust {
		t.Fatalf("ledger = %+v, want one admin_adjust +500", ledger)
	}
	if ledger[0].RefID == nil || *ledger[0].RefID != admin.ID {
		t.Errorf("ledger ref = %v, want actor %d", ledger[0].RefID, admin.ID)
	}

	// Lowering the balance records a negative delta.
	if err := repo.SetPoints(ctx, u.ID, 200, admin.ID); err != nil {
		t.Fatalf("SetPoints lower: %v", err)
	}
	ledger = bonusLedger(t, db, u.ID)
	if len(ledger) != 2 || ledger[1].Delta != -300 {
		t.Fatalf("ledger after lowering = %+v, want second row of -300", ledger)
	}

	// Unchanged balance writes no ledger row.
	if err := repo.SetPoints(ctx, u.ID, 200, admin.ID); err != nil {
		t.Fatalf("SetPoints unchanged: %v", err)
	}
	if got := len(bonusLedger(t, db, u.ID)); got != 2 {
		t.Errorf("ledger rows after no-op set = %d, want 2", got)
	}
}

func TestBonusRepo_PurchaseInvite(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewBonusRepo(db)

	u := newUser(t, db)
	item := itemBySlug(t, db, "invite-1")
	if err := repo.SetPoints(ctx, u.ID, item.Price+100, u.ID); err != nil {
		t.Fatalf("fund user: %v", err)
	}

	bought, newBalance, err := repo.PurchaseItem(ctx, u.ID, item.ID)
	if err != nil {
		t.Fatalf("PurchaseItem: %v", err)
	}
	if bought.Slug != "invite-1" || newBalance != 100 {
		t.Errorf("bought=%s newBalance=%d, want invite-1/100", bought.Slug, newBalance)
	}

	var invites int
	if err := db.QueryRow(`SELECT invites FROM users WHERE id=$1`, u.ID).Scan(&invites); err != nil {
		t.Fatalf("read invites: %v", err)
	}
	if invites != int(item.RewardAmount) {
		t.Errorf("invites = %d, want %d", invites, item.RewardAmount)
	}

	ledger := bonusLedger(t, db, u.ID)
	last := ledger[len(ledger)-1]
	if last.Delta != -item.Price || last.Reason != model.BonusReasonPurchase || last.RefID == nil || *last.RefID != item.ID {
		t.Errorf("purchase ledger row = %+v, want -%d purchase ref=%d", last, item.Price, item.ID)
	}
}

func TestBonusRepo_PurchaseUploadCredit(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewBonusRepo(db)

	u := newUser(t, db)
	item := itemBySlug(t, db, "upload-5gib")
	if err := repo.SetPoints(ctx, u.ID, item.Price, u.ID); err != nil {
		t.Fatalf("fund user: %v", err)
	}

	_, newBalance, err := repo.PurchaseItem(ctx, u.ID, item.ID)
	if err != nil {
		t.Fatalf("PurchaseItem: %v", err)
	}
	if newBalance != 0 {
		t.Errorf("newBalance = %d, want 0", newBalance)
	}

	var uploaded int64
	if err := db.QueryRow(`SELECT uploaded FROM users WHERE id=$1`, u.ID).Scan(&uploaded); err != nil {
		t.Fatalf("read uploaded: %v", err)
	}
	if uploaded != item.RewardAmount {
		t.Errorf("uploaded = %d, want %d", uploaded, item.RewardAmount)
	}
}

// TestBonusRepo_PurchaseInsufficientBalance is the mandated race-safety test:
// a purchase the balance cannot cover must fail and change nothing.
func TestBonusRepo_PurchaseInsufficientBalance(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewBonusRepo(db)

	u := newUser(t, db)
	item := itemBySlug(t, db, "invite-1")
	if err := repo.SetPoints(ctx, u.ID, item.Price-1, u.ID); err != nil {
		t.Fatalf("fund user: %v", err)
	}
	ledgerBefore := len(bonusLedger(t, db, u.ID))

	_, _, err := repo.PurchaseItem(ctx, u.ID, item.ID)
	if !errors.Is(err, repository.ErrInsufficientBonusPoints) {
		t.Fatalf("err = %v, want ErrInsufficientBonusPoints", err)
	}

	if got := bonusBalance(t, db, u.ID); got != item.Price-1 {
		t.Errorf("balance changed on failed purchase: %d", got)
	}
	var invites int
	_ = db.QueryRow(`SELECT invites FROM users WHERE id=$1`, u.ID).Scan(&invites)
	if invites != 0 {
		t.Errorf("invites granted on failed purchase: %d", invites)
	}
	if got := len(bonusLedger(t, db, u.ID)); got != ledgerBefore {
		t.Errorf("ledger rows written on failed purchase: %d -> %d", ledgerBefore, got)
	}
}

func TestBonusRepo_PurchaseDisabledAndUnknownItems(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewBonusRepo(db)

	u := newUser(t, db)
	if err := repo.SetPoints(ctx, u.ID, 100000, u.ID); err != nil {
		t.Fatalf("fund user: %v", err)
	}

	// The freeleech ticket is seeded disabled (catalogue-only kind).
	disabled := itemBySlug(t, db, "freeleech-ticket")
	if _, _, err := repo.PurchaseItem(ctx, u.ID, disabled.ID); !errors.Is(err, repository.ErrBonusKindNotAvailable) {
		t.Errorf("disabled item: err = %v, want ErrBonusKindNotAvailable", err)
	}

	if _, _, err := repo.PurchaseItem(ctx, u.ID, 999999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("unknown item: err = %v, want sql.ErrNoRows", err)
	}

	if got := bonusBalance(t, db, u.ID); got != 100000 {
		t.Errorf("balance changed by rejected purchases: %d", got)
	}
}

func TestBonusRepo_ListEnabledItems(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	repo := NewBonusRepo(db)

	items, err := repo.ListEnabledItems(context.Background())
	if err != nil {
		t.Fatalf("ListEnabledItems: %v", err)
	}
	// Seeded: invite-1 and upload-5gib enabled; freeleech/double-upload disabled.
	slugs := map[string]bool{}
	for _, it := range items {
		slugs[it.Slug] = true
		if !it.Enabled {
			t.Errorf("disabled item %q returned", it.Slug)
		}
	}
	if !slugs["invite-1"] || !slugs["upload-5gib"] {
		t.Errorf("expected seeded enabled items, got %v", slugs)
	}
	if slugs["freeleech-ticket"] || slugs["double-upload"] {
		t.Errorf("disabled seeds leaked into enabled list: %v", slugs)
	}
}

func TestBonusRepo_SeedingCounts(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewBonusRepo(db)

	seeder := newUser(t, db)
	leecher := newUser(t, db)
	t1 := newTorrent(t, db, seeder.ID)
	t2 := newTorrent(t, db, seeder.ID)

	insert := func(userID, torrentID int64, peerID string, isSeeder bool) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `INSERT INTO peers
			(torrent_id, user_id, peer_id, ip, port, seeder) VALUES ($1,$2,$3,'10.0.0.1',6881,$4)`,
			torrentID, userID, []byte(peerID), isSeeder); err != nil {
			t.Fatalf("insert peer: %v", err)
		}
	}

	insert(seeder.ID, t1.ID, "peer-a-000000000001", true)
	insert(seeder.ID, t2.ID, "peer-a-000000000002", true)
	// Same torrent from a second client: still one distinct torrent.
	insert(seeder.ID, t2.ID, "peer-a-000000000003", true)
	insert(leecher.ID, t1.ID, "peer-b-000000000001", false)

	counts, err := repo.SeedingCounts(ctx)
	if err != nil {
		t.Fatalf("SeedingCounts: %v", err)
	}
	if counts[seeder.ID] != 2 {
		t.Errorf("seeder count = %d, want 2 (distinct torrents)", counts[seeder.ID])
	}
	if _, ok := counts[leecher.ID]; ok {
		t.Errorf("leecher must not appear in seeding counts")
	}
}
