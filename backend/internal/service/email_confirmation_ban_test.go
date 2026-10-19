package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// registerPending registers the bootstrap admin and then a member who still
// has to confirm their email, and returns the member's ID.
func registerPending(t *testing.T, svc *AuthService, repo *mockUserRepo) int64 {
	t.Helper()
	ctx := context.Background()
	for _, u := range []string{"admin", "member"} {
		if _, err := svc.Register(ctx, RegisterRequest{
			Username: u,
			Email:    u + "@example.com",
			Password: "password123",
		}, "127.0.0.1"); err != nil {
			t.Fatalf("register %s: %v", u, err)
		}
	}
	user, err := repo.GetByUsername(ctx, "member")
	if err != nil {
		t.Fatalf("get member: %v", err)
	}
	if user.Enabled || !user.AwaitingConfirmation {
		t.Fatalf("new member enabled=%v awaiting=%v, want a pending registration", user.Enabled, user.AwaitingConfirmation)
	}
	return user.ID
}

// issueToken stores a fresh, valid confirmation token for userID.
func issueToken(t *testing.T, store *mockEmailConfirmationStore, userID int64) string {
	t.Helper()
	raw, err := GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if err := store.Create(context.Background(), userID, hashTokenBytes(raw), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("store token: %v", err)
	}
	return raw
}

func banUser(t *testing.T, repo *mockUserRepo, userID int64) {
	t.Helper()
	user, err := repo.GetByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	user.Disable()
	if err := repo.Update(context.Background(), user); err != nil {
		t.Fatalf("ban: %v", err)
	}
}

// A ban sets enabled=false, the same value a pending registration has. Email
// confirmation must not read that as "still to be confirmed": a banned
// member who asks for a new confirmation and follows it would be enabled.
func TestConfirmEmail_DoesNotReEnableBannedAccount(t *testing.T) {
	cases := map[string]bool{
		"banned after confirming":  true,
		"banned before confirming": false,
	}
	for name, confirmFirst := range cases {
		t.Run(name, func(t *testing.T) {
			svc, repo, _, sender := newAuthServiceWithEmailConfirm()
			ctx := context.Background()
			id := registerPending(t, svc, repo)

			if confirmFirst {
				store := newMockEmailConfirmationStore()
				svc.SetEmailConfirmationStore(store)
				if err := svc.ConfirmEmail(ctx, issueToken(t, store, id)); err != nil {
					t.Fatalf("first confirmation: %v", err)
				}
			}
			banUser(t, repo, id)

			// A clean store, so the resend cooldown cannot be what stops it.
			store := newMockEmailConfirmationStore()
			svc.SetEmailConfirmationStore(store)
			sender.SendCount = 0
			if err := svc.ResendConfirmation(ctx, ResendConfirmationRequest{Email: "member@example.com"}); err != nil {
				t.Fatalf("resend: %v", err)
			}
			if sender.SendCount != 0 || len(store.Items()) != 0 {
				t.Errorf("resend issued %d emails / %d tokens to a banned account", sender.SendCount, len(store.Items()))
			}

			// Even holding a valid token (one issued before the ban), the
			// account stays off.
			if err := svc.ConfirmEmail(ctx, issueToken(t, store, id)); !errors.Is(err, ErrInvalidConfirmToken) {
				t.Errorf("confirm err = %v, want ErrInvalidConfirmToken", err)
			}
			user, _ := repo.GetByID(ctx, id)
			if user.Enabled {
				t.Fatal("confirming an email re-enabled a banned account")
			}

			if _, _, err := svc.Login(ctx, LoginRequest{Username: "member", Password: "password123"}, "127.0.0.1"); !errors.Is(err, ErrInvalidCredentials) {
				t.Errorf("login err = %v, want ErrInvalidCredentials", err)
			}
		})
	}
}

// Confirming ends the pending state, so the account is never treated as
// awaiting confirmation again.
func TestConfirmEmail_ClearsAwaitingConfirmation(t *testing.T) {
	svc, repo, _, _ := newAuthServiceWithEmailConfirm()
	id := registerPending(t, svc, repo)

	store := newMockEmailConfirmationStore()
	svc.SetEmailConfirmationStore(store)
	if err := svc.ConfirmEmail(context.Background(), issueToken(t, store, id)); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	user, _ := repo.GetByID(context.Background(), id)
	if !user.Enabled || user.AwaitingConfirmation {
		t.Errorf("after confirming enabled=%v awaiting=%v, want true/false", user.Enabled, user.AwaitingConfirmation)
	}
}

func makePending(t *testing.T, repo *mockUserRepo, id int64) {
	t.Helper()
	u, err := repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	u.Enabled = false
	u.AwaitingConfirmation = true
	if err := repo.Update(context.Background(), u); err != nil {
		t.Fatalf("update: %v", err)
	}
}

// Every staff disable must end a pending confirmation; otherwise a member
// banned before confirming could confirm their way back in. Staff enabling
// an unconfirmed account stands in for the confirmation.
func TestAdminPathsEndPendingConfirmation(t *testing.T) {
	ctx := context.Background()

	for _, enable := range []bool{false, true} {
		t.Run(fmt.Sprintf("update user enabled=%v", enable), func(t *testing.T) {
			svc, userRepo, _, _, _ := newAdminServiceWithDeps(t)
			target := createTestUserForAdmin(t, userRepo, 5)
			makePending(t, userRepo, target.ID)

			if _, err := svc.UpdateUser(ctx, 99, target.ID, AdminUpdateUserRequest{Enabled: &enable}); err != nil {
				t.Fatalf("update: %v", err)
			}
			u, _ := userRepo.GetByID(ctx, target.ID)
			if u.Enabled != enable || u.AwaitingConfirmation {
				t.Errorf("enabled=%v awaiting=%v, want %v/false", u.Enabled, u.AwaitingConfirmation, enable)
			}
		})
	}

	t.Run("quick ban", func(t *testing.T) {
		svc, userRepo, _, _, _, _ := newAdminServiceForBanTests(t)
		admin := createTestUserForAdmin(t, userRepo, 1)
		target := createTestUserForAdmin(t, userRepo, 5)
		makePending(t, userRepo, target.ID)

		if _, err := svc.QuickBanUser(ctx, admin.ID, target.ID, QuickBanRequest{Reason: "spam"}); err != nil {
			t.Fatalf("quick ban: %v", err)
		}
		if u, _ := userRepo.GetByID(ctx, target.ID); u.Enabled || u.AwaitingConfirmation {
			t.Errorf("enabled=%v awaiting=%v after quick ban, want false/false", u.Enabled, u.AwaitingConfirmation)
		}
	})
}

// The automatic bans end a pending confirmation too: a member still waiting
// to confirm who is banned must not be able to confirm their way back in.
func TestWarningBansEndPendingConfirmation(t *testing.T) {
	ctx := context.Background()
	bans := map[string]func(*WarningService) error{
		"hnr ban": func(svc *WarningService) error { return svc.IssueHnRBan(ctx, 1, "hit-and-runs") },
		"ratio ban": func(svc *WarningService) error {
			w, err := svc.IssueRatioWarning(ctx, 1, "low ratio")
			if err != nil {
				return err
			}
			return svc.EscalateRatioWarning(ctx, w.ID, "Account disabled")
		},
	}
	for name, ban := range bans {
		t.Run(name, func(t *testing.T) {
			userRepo := newMockUserRepoForWarnings()
			userRepo.addUser(&model.User{ID: 1, Username: "pending", AwaitingConfirmation: true})
			svc := NewWarningService(newMockWarningRepo(), userRepo, newMockMessageRepoForWarnings(), event.NewInMemoryBus())

			if err := ban(svc); err != nil {
				t.Fatalf("ban: %v", err)
			}
			if u, _ := userRepo.GetByID(ctx, 1); u.Enabled || u.AwaitingConfirmation {
				t.Errorf("enabled=%v awaiting=%v after %s, want false/false", u.Enabled, u.AwaitingConfirmation, name)
			}
		})
	}
}

// A timed ban that expires turns the account on through Enable, which counts
// as activation: an account banned before it ever confirmed comes back
// activated, not pending and not purgeable.
func TestReEnableExpiredBansActivatesAccount(t *testing.T) {
	svc, userRepo, _, _, _, _ := newAdminServiceForBanTests(t)
	target := createTestUserForAdmin(t, userRepo, 5)
	target.Disable()
	target.ActivatedAt = nil
	past := time.Now().Add(-time.Hour)
	target.DisabledUntil = &past
	_ = userRepo.Update(context.Background(), target)

	if _, err := svc.ReEnableExpiredBans(context.Background()); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	u, _ := userRepo.GetByID(context.Background(), target.ID)
	if !u.Enabled || u.AwaitingConfirmation || u.ActivatedAt == nil {
		t.Errorf("enabled=%v awaiting=%v activated=%v, want an enabled, activated account", u.Enabled, u.AwaitingConfirmation, u.ActivatedAt)
	}
}
