package service

import (
	"context"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// fakeEditHistoryRepo backs both sides of AdminService's edit-history split:
// ListUserEditHistory reads through it (via ListByUser, the interface
// AdminService actually holds), while the *write* side is fed directly by
// mockUserRepo/fakeBonusRepo's push() calls — mirroring how, in production,
// UserRepo/BonusRepo write into the same user_edit_history table each within
// their own transaction, not through UserEditHistoryRepo.Record. Record()
// still works standalone (it's exercised directly by repository tests and
// kept here so this fake fully implements the interface), but UpdateUser no
// longer calls it.
type fakeEditHistoryRepo struct {
	recorded  []model.UserEditHistory
	recordErr error
}

func (f *fakeEditHistoryRepo) push(entries []model.UserEditHistory) {
	f.recorded = append(f.recorded, entries...)
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
	userRepo.historySink = hist
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

// TestAdminUpdateUser_UpdateWithHistoryFailureAbortsWholeUpdate is the
// regression test for this story's headline change: UpdateWithHistory commits
// the diffed profile fields and their audit rows in one transaction, so a
// failure there — including an audit-insert failure — must abort the whole
// call, instead of BE-8.17's best-effort behavior (a failed audit recording
// was logged and swallowed, leaving the already-committed update in place
// with no trail). UpdateWithHistory runs first, before SetStats/SetInvites/
// SetPoints, so failing it here also proves those later stages never run —
// asserted via Uploaded, since admin.go only mutates user.Uploaded *after*
// SetStats succeeds (unlike the fields UpdateWithHistory itself diffs, which
// are mutated on the shared *model.User before UpdateWithHistory is even
// called — mockUserRepo.GetByID hands back that same live pointer rather than
// a defensive copy, so asserting on one of those wouldn't actually prove
// UpdateWithHistory's failure prevented persistence, just that the caller's
// own in-memory diff happened, which is true either way).
// mockUserRepo.updateErr stands in for "the shared transaction failed" —
// from the caller's side, a real Postgres rollback and this mock's refusal
// to mutate state look the same: the write is not observable afterward.
func TestAdminUpdateUser_UpdateWithHistoryFailureAbortsWholeUpdate(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())
	hist := &fakeEditHistoryRepo{}
	userRepo.historySink = hist
	svc.SetEditHistoryRepo(hist)

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, err := authSvc.Register(context.Background(), RegisterRequest{
		Username: "writefail",
		Email:    "writefail@example.com",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}

	userRepo.updateErr = errors.New("tx rolled back")

	uploaded := int64(42)
	title := "should not persist"
	_, err = svc.UpdateUser(context.Background(), 99, result.User.ID, AdminUpdateUserRequest{Title: &title, Uploaded: &uploaded})
	if err == nil {
		t.Fatal("UpdateUser should fail when the write+audit transaction fails")
	}

	got, getErr := userRepo.GetByID(context.Background(), result.User.ID)
	if getErr != nil {
		t.Fatalf("GetByID: %v", getErr)
	}
	if got.Uploaded != 0 {
		t.Errorf("uploaded = %d, want 0 (UpdateWithHistory failing must stop UpdateUser before SetStats ever runs)", got.Uploaded)
	}
	if len(hist.recorded) != 0 {
		t.Errorf("recorded %d entries for a failed write: %+v (audit must not survive a rolled-back write)", len(hist.recorded), hist.recorded)
	}
}

// TestAdminUpdateUser_LaterStageFailureLeavesEarlierWriteCommitted documents
// the accepted shape of the remaining trade-off: UpdateWithHistory, SetStats,
// SetInvites, and BonusRepo.SetPoints are four independent transactions, each
// atomic with its own audit rows, not one whole-request transaction (that
// wasn't this story's scope — see BE-8.18's "accepted trade-offs" note). So
// when an earlier stage (here, UpdateWithHistory, banning the user) commits
// and a later, unrelated stage (SetStats) then fails, the ban and its audit
// row are durable and — the specific gap this test guards — the UserBanned
// event still fires, because event publishing now happens right after
// UpdateWithHistory commits rather than being deferred to the end of the
// whole call. Only the field the failed stage owns (uploaded) is left
// unchanged, and the admin sees an error either way.
func TestAdminUpdateUser_LaterStageFailureLeavesEarlierWriteCommitted(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	bus := event.NewInMemoryBus()
	svc := NewAdminService(userRepo, groupRepo, bus)
	hist := &fakeEditHistoryRepo{}
	userRepo.historySink = hist
	svc.SetEditHistoryRepo(hist)

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, err := authSvc.Register(context.Background(), RegisterRequest{
		Username: "laterstagefail",
		Email:    "laterstagefail@example.com",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("register fixture user: %v", err)
	}

	var banned bool
	bus.Subscribe(event.UserBanned, func(_ context.Context, _ event.Event) error {
		banned = true
		return nil
	})

	userRepo.statsErr = errors.New("stats tx rolled back")

	enabled := false
	uploaded := int64(42)
	_, err = svc.UpdateUser(context.Background(), 99, result.User.ID, AdminUpdateUserRequest{Enabled: &enabled, Uploaded: &uploaded})
	if err == nil {
		t.Fatal("UpdateUser should fail when SetStats's transaction fails")
	}

	got, getErr := userRepo.GetByID(context.Background(), result.User.ID)
	if getErr != nil {
		t.Fatalf("GetByID: %v", getErr)
	}
	if got.Enabled {
		t.Error("enabled = true, want false — UpdateWithHistory already committed the ban before SetStats failed")
	}
	if got.Uploaded != 0 {
		t.Errorf("uploaded = %d, want 0 — SetStats's own transaction must have rolled back", got.Uploaded)
	}
	if !banned {
		t.Error("UserBannedEvent was not published — a committed ban must still be evented even when a later, unrelated write fails")
	}
	if e := findEntry(hist.recorded, "enabled"); e == nil || e.NewValue != "false" {
		t.Errorf("enabled audit entry = %+v, want a committed true->false entry", e)
	}
	if e := findEntry(hist.recorded, "uploaded"); e != nil {
		t.Errorf("uploaded audit entry = %+v, want none — SetStats's audit insert must not have survived its rollback", e)
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
