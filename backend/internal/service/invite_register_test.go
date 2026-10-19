package service

import (
	"context"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// staleValidation reports every invite as unspent, the way a check made just
// before a concurrent registration spent it would.
type staleValidation struct{ *mockInviteRepo }

func (s staleValidation) GetByToken(ctx context.Context, token string) (*model.Invite, error) {
	inv, err := s.mockInviteRepo.GetByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	fresh := *inv
	fresh.InviteeID, fresh.RedeemedAt, fresh.Redeemed = nil, nil, false
	return &fresh, nil
}

// Invite-only registration, through Register: the invite is spent with the
// account, and a registration whose invite was spent after it was validated
// gets no account.
func TestRegister_InviteRegistersOneAccount(t *testing.T) {
	ctx := context.Background()
	users := newMockInviteUserRepo()
	_ = users.Create(ctx, &model.User{Username: "inviter", Email: "inviter@example.com", Invites: 1, Enabled: true, CanInvite: true})

	invites := newMockInviteRepo()
	invites.users = users
	bus := event.NewInMemoryBus()
	inviteSvc := NewInviteService(staleValidation{invites}, users, bus)

	settings := newMockSiteSettingsRepo()
	settings.settings[SettingRegistrationMode] = &model.SiteSetting{Key: SettingRegistrationMode, Value: RegistrationModeInviteOnly}
	auth := NewAuthService(users, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost", bus)
	auth.SetSiteSettings(NewSiteSettingsService(settings, bus))
	auth.SetInviteService(inviteSvc)

	invite, err := inviteSvc.CreateInvite(ctx, 1)
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	if _, err := auth.Register(ctx, RegisterRequest{Username: "first", Email: "first@example.com", Password: "password123", InviteCode: invite.Token}, "127.0.0.1"); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	_, err = auth.Register(ctx, RegisterRequest{Username: "second", Email: "second@example.com", Password: "password123", InviteCode: invite.Token}, "127.0.0.1")
	if !errors.Is(err, ErrInvalidInviteCode) {
		t.Fatalf("second registration err = %v, want ErrInvalidInviteCode", err)
	}
	if u, err := users.GetByUsername(ctx, "second"); err == nil && u != nil {
		t.Error("a second account was created on one invite")
	}
}

// A negative invite balance is refused before anything is written.
func TestAdminUpdateUser_RejectsNegativeInvites(t *testing.T) {
	svc, userRepo, _, _, _ := newAdminServiceWithDeps(t)
	target := createTestUserForAdmin(t, userRepo, 5)
	title := "kept"
	neg := -1

	_, err := svc.UpdateUser(context.Background(), 99, target.ID, AdminUpdateUserRequest{Title: &title, Invites: &neg})
	if !errors.Is(err, ErrAdminInvalidInvites) {
		t.Fatalf("err = %v, want ErrAdminInvalidInvites", err)
	}
	if got, _ := userRepo.GetByID(context.Background(), target.ID); got.Title != nil {
		t.Error("a rejected request still wrote another field")
	}
}
