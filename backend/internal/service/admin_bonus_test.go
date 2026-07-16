package service

import (
	"context"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
)

func newAdminUserForBonus(t *testing.T, userRepo *mockUserRepo) int64 {
	t.Helper()
	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, err := authSvc.Register(context.Background(), RegisterRequest{
		Username: "bonustarget",
		Email:    "bonustarget@example.com",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}
	return result.User.ID
}

func TestAdminUpdateUser_SetsBonusPointsViaBonusRepo(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())
	bonus := &fakeBonusRepo{}
	svc.SetBonusRepo(bonus)

	userID := newAdminUserForBonus(t, userRepo)

	pts := int64(750)
	view, err := svc.UpdateUser(context.Background(), 99, userID, AdminUpdateUserRequest{BonusPoints: &pts})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if len(bonus.setCalls) != 1 || bonus.setCalls[0] != [3]int64{userID, 750, 99} {
		t.Errorf("SetPoints calls = %v, want [user 750 actor99]", bonus.setCalls)
	}
	if view.BonusPoints != 750 {
		t.Errorf("view.BonusPoints = %d, want 750", view.BonusPoints)
	}
}

func TestAdminUpdateUser_NoBonusFieldNoBonusCall(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())
	bonus := &fakeBonusRepo{}
	svc.SetBonusRepo(bonus)

	userID := newAdminUserForBonus(t, userRepo)

	enabled := true
	if _, err := svc.UpdateUser(context.Background(), 99, userID, AdminUpdateUserRequest{Enabled: &enabled}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if len(bonus.setCalls) != 0 {
		t.Errorf("SetPoints called without bonus field: %v", bonus.setCalls)
	}
}

func TestAdminUpdateUser_RejectsNegativeBonusPoints(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())
	svc.SetBonusRepo(&fakeBonusRepo{})

	userID := newAdminUserForBonus(t, userRepo)

	pts := int64(-5)
	_, err := svc.UpdateUser(context.Background(), 99, userID, AdminUpdateUserRequest{BonusPoints: &pts})
	if !errors.Is(err, ErrAdminNegativeBonusPoints) {
		t.Errorf("err = %v, want ErrAdminNegativeBonusPoints", err)
	}
}
