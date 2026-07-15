package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func groupIDBySlug(t *testing.T, db *sql.DB, slug string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM groups WHERE slug = $1`, slug).Scan(&id); err != nil {
		t.Fatalf("group %q: %v", slug, err)
	}
	return id
}

func insertAnnounce(t *testing.T, db *sql.DB, userID, torrentID int64, seeder bool, at time.Time) {
	t.Helper()
	repo := NewAnnounceEventRepo(db)
	if err := repo.Create(context.Background(), &model.AnnounceEvent{
		UserID: userID, TorrentID: &torrentID, PeerID: []byte("peer-one-1234567890"),
		IP: "10.0.0.1", Port: 6881, Event: "announce", Seeder: seeder, AnnouncedAt: at,
	}); err != nil {
		t.Fatalf("insert announce: %v", err)
	}
}

func TestPromotionRepo_RulesCRUD(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewPromotionRepo(db)
	gid := groupIDBySlug(t, db, "power-user")

	rule := &model.PromotionRule{GroupID: gid, MinRatio: 1.0, MinUploaded: 1000, MinAgeDays: 30, MinTorrents: 5, MinSeedHours: 20}
	if err := repo.UpsertRule(ctx, rule); err != nil {
		t.Fatalf("UpsertRule insert: %v", err)
	}
	if rule.CreatedAt.IsZero() {
		t.Fatal("UpsertRule did not populate timestamps")
	}

	// Update via the same conflict path.
	rule.MinRatio = 1.5
	if err := repo.UpsertRule(ctx, rule); err != nil {
		t.Fatalf("UpsertRule update: %v", err)
	}

	rules, err := repo.ListRules(ctx)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 1 || rules[0].MinRatio != 1.5 || rules[0].MinTorrents != 5 {
		t.Fatalf("unexpected rules after update: %+v", rules)
	}

	if err := repo.DeleteRule(ctx, gid); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	if err := repo.DeleteRule(ctx, gid); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DeleteRule(missing) = %v, want sql.ErrNoRows", err)
	}
}

func TestPromotionRepo_LadderMetrics(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewPromotionRepo(db)

	userGroup := groupIDBySlug(t, db, "user")
	powerGroup := groupIDBySlug(t, db, "power-user")

	u := newUser(t, db)
	if _, err := db.ExecContext(ctx, `UPDATE users SET group_id=$1, uploaded=$2, downloaded=$3 WHERE id=$4`,
		userGroup, int64(5000), int64(1000), u.ID); err != nil {
		t.Fatalf("set user stats: %v", err)
	}
	// Two uploaded torrents for the user.
	newTorrent(t, db, u.ID)
	newTorrent(t, db, u.ID)

	// A user in a non-ladder group must be excluded when we ask only for userGroup.
	other := newUser(t, db)
	if _, err := db.ExecContext(ctx, `UPDATE users SET group_id=$1 WHERE id=$2`, powerGroup, other.ID); err != nil {
		t.Fatalf("set other group: %v", err)
	}

	metrics, err := repo.LadderMetrics(ctx, []int64{userGroup})
	if err != nil {
		t.Fatalf("LadderMetrics: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("got %d metrics, want 1 (only the userGroup member)", len(metrics))
	}
	m := metrics[0]
	if m.UserID != u.ID || m.Uploaded != 5000 || m.Downloaded != 1000 || m.Torrents != 2 {
		t.Errorf("metrics mismatch: %+v", m)
	}
}

func TestPromotionRepo_SeedHoursByUser(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewPromotionRepo(db)

	tor := newTorrent(t, db, newUser(t, db).ID)

	// User A: 6 announces 30 min apart, first downloading then seeding. The gap
	// after the downloading announce is not counted (its predecessor was not
	// seeding); the four gaps that follow a seeding announce are → 4 * 30 min = 2h.
	userA := newUser(t, db)
	base := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Second)
	seedStates := []bool{false, true, true, true, true, true}
	for i, s := range seedStates {
		insertAnnounce(t, db, userA.ID, tor.ID, s, base.Add(time.Duration(i)*30*time.Minute))
	}

	// User B: two seeding announces 5 hours apart. The gap follows a seeding
	// announce but must be CAPPED at 2 intervals (1h), not counted as 5h.
	userB := newUser(t, db)
	insertAnnounce(t, db, userB.ID, tor.ID, true, base)
	insertAnnounce(t, db, userB.ID, tor.ID, true, base.Add(5*time.Hour))

	// User C: only downloading — contributes no seed time.
	userC := newUser(t, db)
	insertAnnounce(t, db, userC.ID, tor.ID, false, base)
	insertAnnounce(t, db, userC.ID, tor.ID, false, base.Add(30*time.Minute))

	hours, err := repo.SeedHoursByUser(ctx, base.Add(-time.Hour), 2*1800)
	if err != nil {
		t.Fatalf("SeedHoursByUser: %v", err)
	}
	if hours[userA.ID] != 2 {
		t.Errorf("user A seed hours = %d, want 2", hours[userA.ID])
	}
	if hours[userB.ID] != 1 {
		t.Errorf("user B seed hours = %d, want 1 (gap capped, not 5)", hours[userB.ID])
	}
	if _, ok := hours[userC.ID]; ok {
		t.Errorf("user C (download only) should have no seed hours, got %d", hours[userC.ID])
	}
}

func TestPromotionRepo_SetUserGroupAndRuns(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewPromotionRepo(db)

	u := newUser(t, db)
	powerGroup := groupIDBySlug(t, db, "power-user")
	if err := repo.SetUserGroup(ctx, u.ID, powerGroup); err != nil {
		t.Fatalf("SetUserGroup: %v", err)
	}
	var gid int64
	if err := db.QueryRowContext(ctx, `SELECT group_id FROM users WHERE id=$1`, u.ID).Scan(&gid); err != nil {
		t.Fatalf("read group: %v", err)
	}
	if gid != powerGroup {
		t.Errorf("group_id = %d, want %d", gid, powerGroup)
	}
	if err := repo.SetUserGroup(ctx, 999999, powerGroup); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("SetUserGroup(missing) = %v, want sql.ErrNoRows", err)
	}

	// No runs yet.
	if _, ok, err := repo.LastRunAt(ctx); err != nil || ok {
		t.Fatalf("LastRunAt before any run: ok=%v err=%v", ok, err)
	}
	if err := repo.RecordRun(ctx, 3, 1); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
	at, ok, err := repo.LastRunAt(ctx)
	if err != nil || !ok || at.IsZero() {
		t.Errorf("LastRunAt after run: at=%v ok=%v err=%v", at, ok, err)
	}
}
