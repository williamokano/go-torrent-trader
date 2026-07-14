package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- warnings ---------------------------------------------------------------

func newWarning(t *testing.T, db *sql.DB, userID int64, wtype, status string, expires *time.Time) *model.Warning {
	t.Helper()

	w := &model.Warning{
		UserID:    userID,
		Type:      wtype,
		Reason:    "test reason",
		Status:    status,
		ExpiresAt: expires,
	}
	if err := NewWarningRepo(db).Create(context.Background(), w); err != nil {
		t.Fatalf("creating warning: %v", err)
	}
	return w
}

func TestWarningRepoCreateGetUpdate(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewWarningRepo(db)
	u := newUser(t, db)
	w := newWarning(t, db, u.ID, "manual", "active", nil)

	got, err := repo.GetByID(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Reason != "test reason" || got.Status != "active" {
		t.Errorf("got reason=%q status=%q", got.Reason, got.Status)
	}

	w.Status = "lifted"
	w.LiftedReason = ptr("appealed")
	w.LiftedAt = ptr(time.Now())
	if err := repo.Update(ctx, w); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err = repo.GetByID(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Status != "lifted" || got.LiftedReason == nil || *got.LiftedReason != "appealed" {
		t.Errorf("lift did not persist: status=%q reason=%v", got.Status, got.LiftedReason)
	}
}

func TestWarningRepoListByUserRespectsIncludeInactive(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewWarningRepo(db)
	u := newUser(t, db)
	newWarning(t, db, u.ID, "manual", "active", nil)
	newWarning(t, db, u.ID, "ratio", "lifted", nil)

	active, err := repo.ListByUser(ctx, u.ID, false)
	if err != nil {
		t.Fatalf("ListByUser(active): %v", err)
	}
	if len(active) != 1 {
		t.Errorf("active-only returned %d, want 1", len(active))
	}

	all, err := repo.ListByUser(ctx, u.ID, true)
	if err != nil {
		t.Fatalf("ListByUser(all): %v", err)
	}
	if len(all) != 2 {
		t.Errorf("includeInactive returned %d, want 2", len(all))
	}
}

func TestWarningRepoCountsActiveByType(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewWarningRepo(db)
	u := newUser(t, db)
	newWarning(t, db, u.ID, "manual", "active", nil)
	newWarning(t, db, u.ID, "ratio", "active", nil)
	newWarning(t, db, u.ID, "manual", "lifted", nil)

	total, err := repo.CountActiveByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("CountActiveByUser: %v", err)
	}
	if total != 2 {
		t.Errorf("active = %d, want 2 (lifted must not count)", total)
	}

	manual, err := repo.CountActiveManualByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("CountActiveManualByUser: %v", err)
	}
	if manual != 1 {
		t.Errorf("active manual = %d, want 1", manual)
	}
}

func TestWarningRepoGetActiveRatioWarning(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewWarningRepo(db)
	u := newUser(t, db)

	if _, err := repo.GetActiveRatioWarning(ctx, u.ID); err == nil {
		t.Error("expected an error when the user has no ratio warning")
	}

	w := newWarning(t, db, u.ID, "ratio_soft", "active", nil) // the query looks for ratio_soft specifically
	got, err := repo.GetActiveRatioWarning(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetActiveRatioWarning: %v", err)
	}
	if got.ID != w.ID {
		t.Errorf("id = %d, want %d", got.ID, w.ID)
	}
}

func TestWarningRepoListAllFilters(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewWarningRepo(db)
	a := newUser(t, db)
	b := newUser(t, db)
	newWarning(t, db, a.ID, "manual", "active", nil)
	newWarning(t, db, b.ID, "ratio", "lifted", nil)

	all, total, err := repo.ListAll(ctx, repository.ListWarningsOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if total != 2 || len(all) != 2 {
		t.Errorf("total=%d len=%d, want 2", total, len(all))
	}

	byUser, _, err := repo.ListAll(ctx, repository.ListWarningsOptions{
		UserID: ptr(a.ID), Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("ListAll(user): %v", err)
	}
	if len(byUser) != 1 || byUser[0].UserID != a.ID {
		t.Errorf("user filter returned %d, want 1", len(byUser))
	}

	byStatus, _, err := repo.ListAll(ctx, repository.ListWarningsOptions{
		Status: ptr("active"), Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("ListAll(status): %v", err)
	}
	if len(byStatus) != 1 || byStatus[0].Status != "active" {
		t.Errorf("status filter returned %d, want 1 active", len(byStatus))
	}
}

// The maintenance job resolves manual warnings whose expiry has passed. A
// warning with no expiry is permanent and must never be auto-resolved.
func TestWarningRepoResolveExpiredManualWarnings(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewWarningRepo(db)
	u := newUser(t, db)

	expired := newWarning(t, db, u.ID, "manual", "active", ptr(time.Now().Add(-time.Hour)))
	future := newWarning(t, db, u.ID, "manual", "active", ptr(time.Now().Add(time.Hour)))
	permanent := newWarning(t, db, u.ID, "manual", "active", nil)

	resolved, err := repo.ResolveExpiredManualWarnings(ctx)
	if err != nil {
		t.Fatalf("ResolveExpiredManualWarnings: %v", err)
	}
	if len(resolved) != 1 || resolved[0] != u.ID {
		t.Fatalf("resolved = %v, want the one user with an expired warning", resolved)
	}

	check := func(id int64, wantStatus string) {
		t.Helper()
		got, err := repo.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("GetByID(%d): %v", id, err)
		}
		if got.Status != wantStatus {
			t.Errorf("warning %d status = %q, want %q", id, got.Status, wantStatus)
		}
	}
	check(expired.ID, "resolved")
	check(future.ID, "active")
	check(permanent.ID, "active")
}

func TestWarningRepoGetUsersWithLowRatio(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewWarningRepo(db)
	userRepo := NewUserRepo(db)

	// Ratio 0.1 — well under the threshold, and enough downloaded to qualify.
	bad := newUser(t, db)
	if err := userRepo.IncrementStats(ctx, bad.ID, 100, 1000); err != nil {
		t.Fatalf("IncrementStats: %v", err)
	}
	// Ratio 2.0 — healthy.
	good := newUser(t, db)
	if err := userRepo.IncrementStats(ctx, good.ID, 2000, 1000); err != nil {
		t.Fatalf("IncrementStats: %v", err)
	}
	// Bad ratio but barely downloaded anything: must not be punished.
	tiny := newUser(t, db)
	if err := userRepo.IncrementStats(ctx, tiny.ID, 0, 10); err != nil {
		t.Fatalf("IncrementStats: %v", err)
	}

	users, err := repo.GetUsersWithLowRatio(ctx, 0.5, 500)
	if err != nil {
		t.Fatalf("GetUsersWithLowRatio: %v", err)
	}
	if len(users) != 1 || users[0].ID != bad.ID {
		t.Errorf("got %d users, want only the one below the ratio AND above the download floor", len(users))
	}
}

// --- bans -------------------------------------------------------------------

func TestBanRepoEmailBans(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewBanRepo(db)

	ban := &model.BannedEmail{Pattern: "%@spam.test", Reason: ptr("spam domain")}
	if err := repo.CreateEmailBan(ctx, ban); err != nil {
		t.Fatalf("CreateEmailBan: %v", err)
	}

	banned, err := repo.IsEmailBanned(ctx, "someone@spam.test")
	if err != nil {
		t.Fatalf("IsEmailBanned: %v", err)
	}
	if !banned {
		t.Error("someone@spam.test should match the %@spam.test pattern")
	}

	ok, err := repo.IsEmailBanned(ctx, "someone@good.test")
	if err != nil {
		t.Fatalf("IsEmailBanned: %v", err)
	}
	if ok {
		t.Error("someone@good.test must not be banned")
	}

	list, err := repo.ListEmailBans(ctx)
	if err != nil {
		t.Fatalf("ListEmailBans: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d email bans, want 1", len(list))
	}

	if err := repo.DeleteEmailBan(ctx, ban.ID); err != nil {
		t.Fatalf("DeleteEmailBan: %v", err)
	}
	list, err = repo.ListEmailBans(ctx)
	if err != nil {
		t.Fatalf("ListEmailBans after delete: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("got %d email bans after delete, want 0", len(list))
	}
}

func TestBanRepoIPBans(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewBanRepo(db)

	ban := &model.BannedIP{IPRange: "10.0.0.0/8", Reason: ptr("bad range")}
	if err := repo.CreateIPBan(ctx, ban); err != nil {
		t.Fatalf("CreateIPBan: %v", err)
	}

	banned, err := repo.IsIPBanned(ctx, "10.1.2.3")
	if err != nil {
		t.Fatalf("IsIPBanned: %v", err)
	}
	if !banned {
		t.Error("10.1.2.3 falls inside 10.0.0.0/8 and should be banned")
	}

	ok, err := repo.IsIPBanned(ctx, "192.168.1.1")
	if err != nil {
		t.Fatalf("IsIPBanned: %v", err)
	}
	if ok {
		t.Error("192.168.1.1 is outside the banned range")
	}

	list, err := repo.ListIPBans(ctx)
	if err != nil {
		t.Fatalf("ListIPBans: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d IP bans, want 1", len(list))
	}

	if err := repo.DeleteIPBan(ctx, ban.ID); err != nil {
		t.Fatalf("DeleteIPBan: %v", err)
	}
}

// --- restrictions -----------------------------------------------------------

func TestRestrictionRepoLifecycle(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewRestrictionRepo(db)
	u := newUser(t, db)
	staff := newUser(t, db)

	r := &model.Restriction{
		UserID:          u.ID,
		RestrictionType: "upload",
		Reason:          "abuse",
		IssuedBy:        &staff.ID,
	}
	if err := repo.Create(ctx, r); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.RestrictionType != "upload" {
		t.Errorf("type = %q, want upload", got.RestrictionType)
	}

	has, err := repo.HasActiveByType(ctx, u.ID, "upload")
	if err != nil {
		t.Fatalf("HasActiveByType: %v", err)
	}
	if !has {
		t.Error("expected an active upload restriction")
	}

	other, err := repo.HasActiveByType(ctx, u.ID, "chat")
	if err != nil {
		t.Fatalf("HasActiveByType(chat): %v", err)
	}
	if other {
		t.Error("a chat restriction was reported but never issued")
	}

	byUser, err := repo.ListByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(byUser) != 1 {
		t.Errorf("got %d restrictions, want 1", len(byUser))
	}

	active, err := repo.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("got %d active restrictions, want 1", len(active))
	}

	// Lifting must clear the active state.
	if err := repo.Lift(ctx, r.ID, &staff.ID); err != nil {
		t.Fatalf("Lift: %v", err)
	}
	has, err = repo.HasActiveByType(ctx, u.ID, "upload")
	if err != nil {
		t.Fatalf("HasActiveByType after lift: %v", err)
	}
	if has {
		t.Error("restriction is still active after being lifted")
	}
}

// Only restrictions whose expiry has passed may be auto-lifted; an open-ended
// restriction stands until a human lifts it.
func TestRestrictionRepoLiftExpired(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewRestrictionRepo(db)
	u := newUser(t, db)

	expired := &model.Restriction{
		UserID: u.ID, RestrictionType: "chat", Reason: "spam",
		ExpiresAt: ptr(time.Now().Add(-time.Hour)),
	}
	if err := repo.Create(ctx, expired); err != nil {
		t.Fatalf("Create(expired): %v", err)
	}
	permanent := &model.Restriction{
		UserID: u.ID, RestrictionType: "upload", Reason: "abuse",
	}
	if err := repo.Create(ctx, permanent); err != nil {
		t.Fatalf("Create(permanent): %v", err)
	}

	lifted, err := repo.LiftExpired(ctx)
	if err != nil {
		t.Fatalf("LiftExpired: %v", err)
	}
	if len(lifted) != 1 || lifted[0].ID != expired.ID {
		t.Fatalf("lifted %d restrictions, want only the expired one", len(lifted))
	}

	if has, _ := repo.HasActiveByType(ctx, u.ID, "upload"); !has {
		t.Error("the open-ended restriction was lifted — only expired ones may be")
	}
}

// --- cheat flags ------------------------------------------------------------

func TestCheatFlagRepoCreateListDismiss(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewCheatFlagRepo(db)
	u := newUser(t, db)
	staff := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	f := &model.CheatFlag{
		UserID:    u.ID,
		TorrentID: &tor.ID,
		FlagType:  "impossible_speed",
		Details:   `{"reason":"1TB in 3 seconds"}`, // details is JSONB
	}
	if err := repo.Create(ctx, f); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Username != u.Username || got.TorrentName != tor.Name {
		t.Errorf("joins not resolved: username=%q torrent=%q", got.Username, got.TorrentName)
	}

	flags, total, err := repo.List(ctx, repository.ListCheatFlagsOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(flags) != 1 {
		t.Errorf("total=%d len=%d, want 1", total, len(flags))
	}

	undismissed, _, err := repo.List(ctx, repository.ListCheatFlagsOptions{
		Dismissed: ptr(false), Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("List(undismissed): %v", err)
	}
	if len(undismissed) != 1 {
		t.Errorf("got %d undismissed flags, want 1", len(undismissed))
	}

	if err := repo.Dismiss(ctx, f.ID, staff.ID); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}

	got, err = repo.GetByID(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetByID after dismiss: %v", err)
	}
	if !got.Dismissed || got.DismissedBy == nil || *got.DismissedBy != staff.ID {
		t.Errorf("dismiss did not persist: dismissed=%v by=%v", got.Dismissed, got.DismissedBy)
	}

	stillUndismissed, _, err := repo.List(ctx, repository.ListCheatFlagsOptions{
		Dismissed: ptr(false), Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("List(undismissed) after dismiss: %v", err)
	}
	if len(stillUndismissed) != 0 {
		t.Errorf("got %d undismissed flags after dismissing the only one", len(stillUndismissed))
	}
}

// The cooldown stops the detector re-flagging the same user/torrent every
// announce. A dismissed flag must not keep the cooldown alive.
func TestCheatFlagRepoHasRecentUndismissed(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewCheatFlagRepo(db)
	u := newUser(t, db)
	staff := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	f := &model.CheatFlag{
		UserID: u.ID, TorrentID: &tor.ID,
		FlagType: "impossible_speed", Details: `{"reason":"x"}`,
	}
	if err := repo.Create(ctx, f); err != nil {
		t.Fatalf("Create: %v", err)
	}

	recent, err := repo.HasRecentUndismissed(ctx, u.ID, tor.ID, "impossible_speed", 24)
	if err != nil {
		t.Fatalf("HasRecentUndismissed: %v", err)
	}
	if !recent {
		t.Error("expected the fresh flag to be within the cooldown")
	}

	// A different flag type is not in cooldown.
	other, err := repo.HasRecentUndismissed(ctx, u.ID, tor.ID, "ratio_spike", 24)
	if err != nil {
		t.Fatalf("HasRecentUndismissed(other type): %v", err)
	}
	if other {
		t.Error("a different flag type must not share the cooldown")
	}

	if err := repo.Dismiss(ctx, f.ID, staff.ID); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	recent, err = repo.HasRecentUndismissed(ctx, u.ID, tor.ID, "impossible_speed", 24)
	if err != nil {
		t.Fatalf("HasRecentUndismissed after dismiss: %v", err)
	}
	if recent {
		t.Error("a dismissed flag must not keep the cooldown alive")
	}
}

// --- mod notes --------------------------------------------------------------

func TestModNoteRepoCRUD(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewModNoteRepo(db)
	u := newUser(t, db)
	author := newUser(t, db)

	n := &model.ModNote{UserID: u.ID, AuthorID: author.ID, Note: "watch this one"}
	if err := repo.Create(ctx, n); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, n.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Note != "watch this one" || got.AuthorUsername != author.Username {
		t.Errorf("note=%q author=%q — the author JOIN should resolve", got.Note, got.AuthorUsername)
	}

	list, err := repo.ListByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("got %d notes, want 1", len(list))
	}

	if err := repo.Delete(ctx, n.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, n.ID); err == nil {
		t.Error("GetByID succeeded after Delete")
	}
}
