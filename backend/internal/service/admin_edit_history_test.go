package service

import (
	"context"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

type fakeEditHistoryRepo struct {
	recorded  []model.UserEditHistory
	recordErr error
}

func (f *fakeEditHistoryRepo) Record(_ context.Context, entries []model.UserEditHistory) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	f.recorded = append(f.recorded, entries...)
	return nil
}

func (f *fakeEditHistoryRepo) ListByUser(_ context.Context, userID int64, limit, offset int) ([]model.UserEditHistory, int64, error) {
	var out []model.UserEditHistory
	for _, e := range f.recorded {
		if e.UserID == userID {
			out = append(out, e)
		}
	}
	total := int64(len(out))
	if offset > len(out) {
		offset = len(out)
	}
	out = out[offset:]
	if len(out) > limit {
		out = out[:limit]
	}
	return out, total, nil
}

// newEditHistoryFixture returns the service, the fake history repo, the
// target user's ID and the acting admin's ID (a real registered user, so the
// username snapshot can be asserted).
func newEditHistoryFixture(t *testing.T) (*AdminService, *fakeEditHistoryRepo, int64, int64) {
	t.Helper()
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())
	hist := &fakeEditHistoryRepo{}
	svc.SetEditHistoryRepo(hist)

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, err := authSvc.Register(context.Background(), RegisterRequest{
		Username: "audittarget",
		Email:    "audittarget@example.com",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}
	actor, err := authSvc.Register(context.Background(), RegisterRequest{
		Username: "auditactor",
		Email:    "auditactor@example.com",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("register fixture actor: %v", err)
	}
	return svc, hist, result.User.ID, actor.User.ID
}

func findEntry(entries []model.UserEditHistory, field string) *model.UserEditHistory {
	for i := range entries {
		if entries[i].Field == field {
			return &entries[i]
		}
	}
	return nil
}

func TestAdminUpdateUser_RecordsEditHistory(t *testing.T) {
	svc, hist, userID, actorID := newEditHistoryFixture(t)

	uploaded := int64(1099511627776) // 1 TB
	invites := 7
	enabled := false
	if _, err := svc.UpdateUser(context.Background(), actorID, userID, AdminUpdateUserRequest{
		Uploaded: &uploaded,
		Invites:  &invites,
		Enabled:  &enabled,
	}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	up := findEntry(hist.recorded, "uploaded")
	if up == nil {
		t.Fatalf("no uploaded entry recorded; got %+v", hist.recorded)
	}
	if up.OldValue != "0" || up.NewValue != "1099511627776" {
		t.Errorf("uploaded entry old=%q new=%q, want 0 -> 1099511627776", up.OldValue, up.NewValue)
	}
	if up.ChangedBy == nil || *up.ChangedBy != actorID {
		t.Errorf("uploaded entry changed_by = %v, want %d", up.ChangedBy, actorID)
	}
	if up.ChangedByUsername != "auditactor" {
		t.Errorf("uploaded entry changed_by_username = %q, want auditactor (write-time snapshot)", up.ChangedByUsername)
	}
	if up.UserID != userID {
		t.Errorf("uploaded entry user_id = %d, want %d", up.UserID, userID)
	}

	if inv := findEntry(hist.recorded, "invites"); inv == nil || inv.NewValue != "7" {
		t.Errorf("invites entry = %+v, want new value 7", inv)
	}
	if en := findEntry(hist.recorded, "enabled"); en == nil || en.OldValue != "true" || en.NewValue != "false" {
		t.Errorf("enabled entry = %+v, want true -> false", en)
	}
}

func TestAdminUpdateUser_UnchangedFieldsNotRecorded(t *testing.T) {
	svc, hist, userID, actorID := newEditHistoryFixture(t)

	// Same values the user already has: no audit rows.
	uploaded := int64(0)
	enabled := true
	username := "audittarget"
	if _, err := svc.UpdateUser(context.Background(), actorID, userID, AdminUpdateUserRequest{
		Uploaded: &uploaded,
		Enabled:  &enabled,
		Username: &username,
	}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	if len(hist.recorded) != 0 {
		t.Errorf("recorded %d entries for no-op update: %+v", len(hist.recorded), hist.recorded)
	}
}

func TestAdminUpdateUser_GroupChangeRecordsNames(t *testing.T) {
	svc, hist, userID, actorID := newEditHistoryFixture(t)

	// The first registered user in the mock repo becomes Administrator (group 1);
	// move them to the User group (5) and expect names, not IDs, in the trail.
	gid := int64(5)
	if _, err := svc.UpdateUser(context.Background(), actorID, userID, AdminUpdateUserRequest{GroupID: &gid}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	g := findEntry(hist.recorded, "group")
	if g == nil {
		t.Fatalf("no group entry recorded; got %+v", hist.recorded)
	}
	if g.OldValue != "Administrator" || g.NewValue != "User" {
		t.Errorf("group entry old=%q new=%q, want Administrator -> User", g.OldValue, g.NewValue)
	}
}

func TestAdminUpdateUser_RecordFailureDoesNotFailUpdate(t *testing.T) {
	svc, hist, userID, actorID := newEditHistoryFixture(t)
	hist.recordErr = errors.New("db down")

	uploaded := int64(42)
	view, err := svc.UpdateUser(context.Background(), actorID, userID, AdminUpdateUserRequest{Uploaded: &uploaded})
	if err != nil {
		t.Fatalf("UpdateUser should not fail when audit recording fails: %v", err)
	}
	if view.Uploaded != 42 {
		t.Errorf("view.Uploaded = %d, want 42", view.Uploaded)
	}
}

func TestAdminUpdateUser_NoHistoryRepoIsFine(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, err := authSvc.Register(context.Background(), RegisterRequest{
		Username: "norepo",
		Email:    "norepo@example.com",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}

	uploaded := int64(1)
	if _, err := svc.UpdateUser(context.Background(), 99, result.User.ID, AdminUpdateUserRequest{Uploaded: &uploaded}); err != nil {
		t.Fatalf("UpdateUser without history repo: %v", err)
	}
}

func TestListUserEditHistory(t *testing.T) {
	svc, _, userID, actorID := newEditHistoryFixture(t)

	uploaded := int64(1024)
	invites := 3
	if _, err := svc.UpdateUser(context.Background(), actorID, userID, AdminUpdateUserRequest{
		Uploaded: &uploaded,
		Invites:  &invites,
	}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	views, total, err := svc.ListUserEditHistory(context.Background(), userID, 50, 0)
	if err != nil {
		t.Fatalf("ListUserEditHistory: %v", err)
	}
	if total != 2 || len(views) != 2 {
		t.Fatalf("want 2 entries, got len=%d total=%d", len(views), total)
	}

	_, _, err = svc.ListUserEditHistory(context.Background(), 424242, 50, 0)
	if !errors.Is(err, ErrAdminUserNotFound) {
		t.Errorf("unknown user err = %v, want ErrAdminUserNotFound", err)
	}
}
