package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func TestInviteDistributionRepo_RulesCRUD(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewInviteDistributionRepo(db)
	gid := groupIDBySlug(t, db, "power-user")

	rule := &model.InviteDistributionRule{GroupID: gid, MinRatio: 1.0, MinDownloadedBytes: 1000, MaxDownloadedBytes: 5000, MaxInvites: 3}
	if err := repo.UpsertRule(ctx, rule); err != nil {
		t.Fatalf("UpsertRule insert: %v", err)
	}
	if rule.CreatedAt.IsZero() {
		t.Fatal("UpsertRule did not populate timestamps")
	}

	// Update via the same conflict path.
	rule.MaxInvites = 5
	if err := repo.UpsertRule(ctx, rule); err != nil {
		t.Fatalf("UpsertRule update: %v", err)
	}

	rules, err := repo.ListRules(ctx)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 1 || rules[0].MaxInvites != 5 || rules[0].MinDownloadedBytes != 1000 {
		t.Fatalf("unexpected rules after update: %+v", rules)
	}

	if err := repo.DeleteRule(ctx, gid); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	if err := repo.DeleteRule(ctx, gid); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DeleteRule(missing) = %v, want sql.ErrNoRows", err)
	}
}

func TestInviteDistributionRepo_GroupMetrics(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewInviteDistributionRepo(db)

	userGroup := groupIDBySlug(t, db, "user")
	powerGroup := groupIDBySlug(t, db, "power-user")

	u := newUser(t, db)
	if _, err := db.ExecContext(ctx, `UPDATE users SET group_id=$1, uploaded=$2, downloaded=$3 WHERE id=$4`,
		userGroup, int64(5000), int64(1000), u.ID); err != nil {
		t.Fatalf("set user stats: %v", err)
	}

	// A user in a different group must be excluded when we ask only for userGroup.
	other := newUser(t, db)
	if _, err := db.ExecContext(ctx, `UPDATE users SET group_id=$1 WHERE id=$2`, powerGroup, other.ID); err != nil {
		t.Fatalf("set other group: %v", err)
	}

	metrics, err := repo.GroupMetrics(ctx, []int64{userGroup})
	if err != nil {
		t.Fatalf("GroupMetrics: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("got %d metrics, want 1 (only the userGroup member)", len(metrics))
	}
	if metrics[0].UserID != u.ID || metrics[0].Uploaded != 5000 || metrics[0].Downloaded != 1000 {
		t.Errorf("metrics mismatch: %+v", metrics[0])
	}
}

func TestInviteDistributionRepo_UserInviteStates(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewInviteDistributionRepo(db)

	u := newUser(t, db)
	if _, err := db.ExecContext(ctx, `UPDATE users SET invites=$1, can_invite=$2 WHERE id=$3`, 4, false, u.ID); err != nil {
		t.Fatalf("set invite state: %v", err)
	}

	states, err := repo.UserInviteStates(ctx, []int64{u.ID, 999999})
	if err != nil {
		t.Fatalf("UserInviteStates: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected exactly one resolved state (missing user skipped), got %d", len(states))
	}
	got, ok := states[u.ID]
	if !ok {
		t.Fatalf("missing state for user %d", u.ID)
	}
	if got.Invites != 4 || got.CanInvite || got.Username != u.Username {
		t.Errorf("unexpected state: %+v", got)
	}

	empty, err := repo.UserInviteStates(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("empty input should return an empty map, got %v err=%v", empty, err)
	}
}

func TestInviteDistributionRepo_Runs(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewInviteDistributionRepo(db)

	if _, ok, err := repo.LastRunAt(ctx); err != nil || ok {
		t.Fatalf("LastRunAt before any run: ok=%v err=%v", ok, err)
	}
	if err := repo.RecordRun(ctx, 3); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
	at, ok, err := repo.LastRunAt(ctx)
	if err != nil || !ok || at.IsZero() {
		t.Errorf("LastRunAt after run: at=%v ok=%v err=%v", at, ok, err)
	}
}
