package service

import (
	"context"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

func newWarningServiceForExtras(t *testing.T) (*WarningService, *mockWarningRepo, *mockUserRepoForWarnings) {
	t.Helper()

	warnRepo := newMockWarningRepo()
	userRepo := newMockUserRepoForWarnings()
	svc := NewWarningService(warnRepo, userRepo, newMockMessageRepoForWarnings(), event.NewInMemoryBus())
	return svc, warnRepo, userRepo
}

// GetActiveWarnings is the owner's view (active only); GetAllWarnings is the
// staff view (everything). The distinction is the whole point — a resolved
// warning must be invisible to the user but visible to staff.
func TestWarningServiceActiveVsAllViews(t *testing.T) {
	svc, warnRepo, _ := newWarningServiceForExtras(t)
	ctx := context.Background()

	warnRepo.warnings = []*model.Warning{
		{ID: 1, UserID: 2, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
		{ID: 2, UserID: 2, Type: model.WarningTypeManual, Status: model.WarningStatusResolved},
	}

	active, err := svc.GetActiveWarnings(ctx, 2)
	if err != nil {
		t.Fatalf("GetActiveWarnings: %v", err)
	}
	if len(active) != 1 || active[0].ID != 1 {
		t.Errorf("active view = %d warnings, want only the active one", len(active))
	}

	all, err := svc.GetAllWarnings(ctx, 2)
	if err != nil {
		t.Fatalf("GetAllWarnings: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("staff view = %d warnings, want both", len(all))
	}
}

func TestWarningServiceGetActiveRatioWarning(t *testing.T) {
	svc, warnRepo, _ := newWarningServiceForExtras(t)
	ctx := context.Background()

	// No ratio warning: the service maps sql.ErrNoRows to (nil, nil) so callers
	// can treat "none" without special-casing the error.
	got, err := svc.GetActiveRatioWarning(ctx, 2)
	if err != nil {
		t.Fatalf("GetActiveRatioWarning: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil when the user has no ratio warning", got)
	}

	warnRepo.warnings = []*model.Warning{
		{ID: 5, UserID: 2, Type: model.WarningTypeRatioSoft, Status: model.WarningStatusActive},
	}
	got, err = svc.GetActiveRatioWarning(ctx, 2)
	if err != nil {
		t.Fatalf("GetActiveRatioWarning: %v", err)
	}
	if got == nil || got.ID != 5 {
		t.Errorf("got %v, want the active ratio_soft warning", got)
	}
}

func TestWarningServiceGetUsersWithLowRatioDelegates(t *testing.T) {
	svc, _, _ := newWarningServiceForExtras(t)

	// The mock returns nil; the point is that the call path works and does not
	// error — the real filtering is exercised at the repository layer.
	users, err := svc.GetUsersWithLowRatio(context.Background(), 0.3, 5<<30)
	if err != nil {
		t.Fatalf("GetUsersWithLowRatio: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("got %d users from the stub, want 0", len(users))
	}
}

// The maintenance job resolves expired manual warnings and, for any user left
// with no active warnings, clears the warned flag. A user with another active
// warning must stay flagged.
func TestWarningServiceResolveExpiredClearsFlagOnlyWhenNoneRemain(t *testing.T) {
	svc, warnRepo, userRepo := newWarningServiceForExtras(t)
	ctx := context.Background()

	past := time.Now().Add(-time.Hour)

	// cleared: one expired manual warning, nothing else.
	cleared := &model.User{ID: 2, Username: "cleared", Enabled: true, Warned: true}
	// stillWarned: an expired manual warning AND a separate active one.
	stillWarned := &model.User{ID: 3, Username: "still", Enabled: true, Warned: true}
	userRepo.addUser(cleared)
	userRepo.addUser(stillWarned)

	warnRepo.warnings = []*model.Warning{
		{ID: 1, UserID: 2, Type: model.WarningTypeManual, Status: model.WarningStatusActive, ExpiresAt: &past},
		{ID: 2, UserID: 3, Type: model.WarningTypeManual, Status: model.WarningStatusActive, ExpiresAt: &past},
		{ID: 3, UserID: 3, Type: model.WarningTypeManual, Status: model.WarningStatusActive}, // no expiry: stays active
	}

	n, err := svc.ResolveExpiredManualWarnings(ctx)
	if err != nil {
		t.Fatalf("ResolveExpiredManualWarnings: %v", err)
	}
	if n != 2 {
		t.Errorf("resolved %d users, want 2", n)
	}

	if got, _ := userRepo.GetByID(ctx, 2); got.Warned {
		t.Error("user 2 still flagged — their only warning expired, so the flag should clear")
	}
	if got, _ := userRepo.GetByID(ctx, 3); !got.Warned {
		t.Error("user 3 was un-flagged despite still having an active warning")
	}
}

func TestWarningServiceListWarningsDelegates(t *testing.T) {
	svc, warnRepo, _ := newWarningServiceForExtras(t)

	warnRepo.warnings = []*model.Warning{
		{ID: 1, UserID: 2, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
	}
	list, total, err := svc.ListWarnings(context.Background(), repository.ListWarningsOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("ListWarnings: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("got %d (total %d), want 1", len(list), total)
	}
}
