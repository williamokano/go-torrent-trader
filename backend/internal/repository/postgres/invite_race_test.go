package postgres

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// Ten registrations racing on one invite: exactly one gets an account, and
// the others leave no account behind.
func TestCreateUserRedeemingRegistersOnce(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	inviter := newUser(t, db)
	invites := NewInviteRepo(db)
	inv := &model.Invite{InviterID: inviter.ID, Token: uniq("tok"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := invites.Create(ctx, inv); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	const n = 10
	var wg sync.WaitGroup
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := uniq("racer")
			results <- invites.CreateUserRedeeming(ctx, inv.Token, &model.User{
				Username: name, Email: name + "@example.test", PasswordHash: "h", PasswordScheme: "argon2id",
				GroupID: anyGroupID(t, db),
			})
		}(i)
	}
	wg.Wait()
	close(results)

	won := 0
	for err := range results {
		switch {
		case err == nil:
			won++
		case errors.Is(err, sql.ErrNoRows):
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	var accounts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE username LIKE 'racer%'`).Scan(&accounts); err != nil {
		t.Fatalf("count: %v", err)
	}
	if won != 1 || accounts != 1 {
		t.Errorf("registrations won=%d accounts=%d, want exactly one of each", won, accounts)
	}
}

// An expired invite creates no account: the insert is rolled back with it.
func TestCreateUserRedeemingRollsBackOnUnusableInvite(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	inviter := newUser(t, db)
	invites := NewInviteRepo(db)
	inv := &model.Invite{InviterID: inviter.ID, Token: uniq("tok"), ExpiresAt: time.Now().Add(-time.Hour)}
	if err := invites.Create(ctx, inv); err != nil {
		t.Fatalf("create invite: %v", err)
	}
	name := uniq("late")
	err := invites.CreateUserRedeeming(ctx, inv.Token, &model.User{
		Username: name, Email: name + "@example.test", PasswordHash: "h", PasswordScheme: "argon2id", GroupID: anyGroupID(t, db),
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
	if _, err := NewUserRepo(db).GetByUsername(ctx, name); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("an account was left behind for an expired invite (err = %v)", err)
	}
}

// Ten concurrent spends of a single invite: exactly one succeeds, and the
// balance ends at zero, never below.
func TestAdjustInvitesNeverGoesNegative(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewUserRepo(db)
	u := newUser(t, db)
	if err := repo.SetInvites(ctx, u.ID, 1, nil); err != nil {
		t.Fatalf("set invites: %v", err)
	}

	const n = 10
	var wg sync.WaitGroup
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- repo.AdjustInvites(ctx, u.ID, -1)
		}()
	}
	wg.Wait()
	close(results)

	spent := 0
	for err := range results {
		switch {
		case err == nil:
			spent++
		case errors.Is(err, repository.ErrInsufficientInvites):
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	got, _ := repo.GetByID(ctx, u.ID)
	if spent != 1 || got.Invites != 0 {
		t.Errorf("spent=%d balance=%d, want 1 and 0", spent, got.Invites)
	}

	if err := repo.AdjustInvites(ctx, 1<<40, -1); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("unknown user: err = %v, want sql.ErrNoRows", err)
	}
	if _, err := db.Exec(`UPDATE users SET invites = -1 WHERE id = $1`, u.ID); err == nil {
		t.Error("the schema accepted a negative invite balance")
	}
}

// Ten concurrent invites from an inviter with one left: exactly one invite
// exists afterwards and the balance is zero.
func TestCreateInviteSpendingSpendsTheLastInviteOnce(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	inviter := newUser(t, db)
	if err := NewUserRepo(db).SetInvites(ctx, inviter.ID, 1, nil); err != nil {
		t.Fatalf("set invites: %v", err)
	}
	invites := NewInviteRepo(db)

	const n = 10
	var wg sync.WaitGroup
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- invites.CreateInviteSpending(ctx, &model.Invite{InviterID: inviter.ID, Token: uniq("tok"), ExpiresAt: time.Now().Add(time.Hour)})
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && !errors.Is(err, repository.ErrInsufficientInvites) {
			t.Errorf("unexpected error: %v", err)
		}
	}

	var created, balance int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM invites WHERE inviter_id = $1), (SELECT invites FROM users WHERE id = $1)`, inviter.ID).Scan(&created, &balance); err != nil {
		t.Fatalf("read: %v", err)
	}
	if created != 1 || balance != 0 {
		t.Errorf("invites created=%d balance=%d, want 1 and 0", created, balance)
	}
}

// A restricted inviter's outstanding invite registers nobody, even when the
// restriction lands after the invite was checked.
func TestCreateUserRedeemingRefusesAVoidedInvite(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	inviter := newUser(t, db)
	invites := NewInviteRepo(db)
	inv := &model.Invite{InviterID: inviter.ID, Token: uniq("tok"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := invites.Create(ctx, inv); err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if err := NewUserRepo(db).SetPrivilegeFlag(ctx, inviter.ID, model.RestrictionTypeInvite, false); err != nil {
		t.Fatalf("restrict inviter: %v", err)
	}

	name := uniq("voided")
	err := invites.CreateUserRedeeming(ctx, inv.Token, &model.User{
		Username: name, Email: name + "@example.test", PasswordHash: "h", PasswordScheme: "argon2id", GroupID: anyGroupID(t, db),
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
	if _, err := NewUserRepo(db).GetByUsername(ctx, name); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("an account was created on a voided invite (err = %v)", err)
	}
}

// A registration that fails on a taken username leaves the invite unspent,
// so the invitee can try again with another name.
func TestCreateUserRedeemingLeavesInviteOnUniqueViolation(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	inviter := newUser(t, db)
	taken := newUser(t, db)
	invites := NewInviteRepo(db)
	inv := &model.Invite{InviterID: inviter.ID, Token: uniq("tok"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := invites.Create(ctx, inv); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	err := invites.CreateUserRedeeming(ctx, inv.Token, &model.User{
		Username: taken.Username, Email: uniq("other") + "@example.test", PasswordHash: "h", PasswordScheme: "argon2id", GroupID: anyGroupID(t, db),
	})
	if err == nil {
		t.Fatal("registration with a taken username succeeded")
	}
	got, err := invites.GetByToken(ctx, inv.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got.Redeemed {
		t.Error("the invite was spent by a registration that failed")
	}
}
