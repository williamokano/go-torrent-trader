package service

import (
	"context"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// A member's own password change stores the argon2id scheme with the hash,
// so a member who arrived on a legacy scheme can log in with the new one.
func TestChangePasswordStoresArgon2idScheme(t *testing.T) {
	repo := newMockUserRepo()
	hash, err := HashPassword("old-password-1")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := &model.User{Username: "member", Email: "m@example.com", PasswordHash: hash, PasswordScheme: SchemeLegacyMD5, Enabled: true}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("create: %v", err)
	}
	svc := NewUserService(repo, newTestSessionStore(), nil, nil, nil)

	if err := svc.ChangePassword(context.Background(), u.ID, "", ChangePasswordRequest{
		CurrentPassword: "old-password-1",
		NewPassword:     "new-password-2",
	}); err != nil {
		t.Fatalf("change password: %v", err)
	}
	got, _ := repo.GetByID(context.Background(), u.ID)
	if got.PasswordScheme != SchemeArgon2id {
		t.Errorf("scheme = %q, want %q", got.PasswordScheme, SchemeArgon2id)
	}
}

// racingWarnings answers the first count as if no warning were active, then
// shows one when asked again — a warning issued while the flag was cleared.
type racingWarnings struct {
	*mockWarningRepo
	late model.Warning
}

func (r *racingWarnings) CountActiveByUser(context.Context, int64) (int, error) { return 0, nil }
func (r *racingWarnings) ListByUser(context.Context, int64, bool) ([]model.Warning, error) {
	return []model.Warning{r.late}, nil
}

// Clearing the warned flag looks again afterwards: a warning that arrived in
// between puts the flag back, with its expiry, so it still lapses on time.
func TestClearWarnedIfNoneRestoresARacingWarning(t *testing.T) {
	expiry := time.Now().Add(48 * time.Hour).Truncate(time.Second)
	users := newMockUserRepoForWarnings()
	users.addUser(&model.User{ID: 1, Username: "member", Warned: true})
	svc := NewWarningService(&racingWarnings{mockWarningRepo: newMockWarningRepo(), late: model.Warning{UserID: 1, ExpiresAt: &expiry}},
		users, newMockMessageRepoForWarnings(), event.NewInMemoryBus())

	if err := svc.clearWarnedIfNone(context.Background(), 1); err != nil {
		t.Fatalf("clear: %v", err)
	}
	u, _ := users.GetByID(context.Background(), 1)
	if !u.Warned || u.WarnUntil == nil || !u.WarnUntil.Equal(expiry) {
		t.Errorf("warned=%v warn_until=%v, want the racing warning restored with its expiry %v", u.Warned, u.WarnUntil, expiry)
	}
}

func TestLatestExpiry(t *testing.T) {
	soon, later := time.Now().Add(time.Hour), time.Now().Add(2*time.Hour)
	cases := map[string]struct {
		in   []model.Warning
		want *time.Time
	}{
		"latest of timed warnings":  {[]model.Warning{{ExpiresAt: &soon}, {ExpiresAt: &later}}, &later},
		"any open-ended one wins":   {[]model.Warning{{ExpiresAt: &later}, {}}, nil},
		"no warnings has no expiry": {nil, nil},
	}
	for name, tc := range cases {
		got := latestExpiry(tc.in)
		if (got == nil) != (tc.want == nil) || (got != nil && !got.Equal(*tc.want)) {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

// A timed quick ban never shortens a permanent one already in force.
func TestQuickBanKeepsAnExistingPermanentBan(t *testing.T) {
	svc, userRepo, _, _, _, _ := newAdminServiceForBanTests(t)
	admin := createTestUserForAdmin(t, userRepo, 1)
	target := createTestUserForAdmin(t, userRepo, 5)
	if err := userRepo.UpdateFields(context.Background(), target.ID, model.DisablePatch(nil)); err != nil {
		t.Fatalf("permanent ban: %v", err)
	}

	days := 3
	if _, err := svc.QuickBanUser(context.Background(), admin.ID, target.ID, QuickBanRequest{Reason: "again", DurationDays: &days}); err != nil {
		t.Fatalf("quick ban: %v", err)
	}
	got, _ := userRepo.GetByID(context.Background(), target.ID)
	if got.Enabled || got.DisabledUntil != nil {
		t.Errorf("enabled=%v disabled_until=%v, want the permanent ban kept", got.Enabled, got.DisabledUntil)
	}
}
