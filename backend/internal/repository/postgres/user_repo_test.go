package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

func TestUserRepoCreateAndLookups(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	if u.ID == 0 || u.CreatedAt.IsZero() {
		t.Fatalf("Create did not populate ID/CreatedAt: %+v", u)
	}

	t.Run("by id", func(t *testing.T) {
		got, err := repo.GetByID(ctx, u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Username != u.Username {
			t.Errorf("username = %q, want %q", got.Username, u.Username)
		}
	})

	t.Run("by username", func(t *testing.T) {
		got, err := repo.GetByUsername(ctx, u.Username)
		if err != nil {
			t.Fatalf("GetByUsername: %v", err)
		}
		if got.ID != u.ID {
			t.Errorf("id = %d, want %d", got.ID, u.ID)
		}
	})

	t.Run("by email", func(t *testing.T) {
		got, err := repo.GetByEmail(ctx, u.Email)
		if err != nil {
			t.Fatalf("GetByEmail: %v", err)
		}
		if got.ID != u.ID {
			t.Errorf("id = %d, want %d", got.ID, u.ID)
		}
	})

	t.Run("missing id reports no rows", func(t *testing.T) {
		if _, err := repo.GetByID(ctx, 999999); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("err = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestUserRepoGetByPasskey(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)
	u.Passkey = ptr(uniq("passkey"))
	if err := repo.Update(ctx, u); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByPasskey(ctx, *u.Passkey)
	if err != nil {
		t.Fatalf("GetByPasskey: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("id = %d, want %d", got.ID, u.ID)
	}
}

func TestUserRepoUpdatePersistsChanges(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	u.Enabled = false
	u.Donor = true
	u.Title = ptr("Elite")
	if err := repo.Update(ctx, u); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Enabled || !got.Donor || got.Title == nil || *got.Title != "Elite" {
		t.Errorf("update did not persist: enabled=%v donor=%v title=%v",
			got.Enabled, got.Donor, got.Title)
	}
}

// TestUserRepoUpdateDoesNotWriteInvites documents that Update() excludes the
// invites column entirely (mirroring bonus_points and the four privilege
// flags): an in-memory Invites value on the struct passed to Update is
// silently ignored rather than persisted. AdjustInvites/SetInvites are the
// only paths that may change it.
func TestUserRepoUpdateDoesNotWriteInvites(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)
	if err := repo.SetInvites(ctx, u.ID, 1, nil); err != nil {
		t.Fatalf("seed invites: %v", err)
	}

	u.Invites = 99 // an in-memory value Update() must not persist
	if err := repo.Update(ctx, u); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Invites != 1 {
		t.Errorf("invites = %d, want 1 (Update must not write this column)", got.Invites)
	}
}

// TestUserRepoUpdateDoesNotWriteStats documents that Update() excludes
// uploaded/downloaded entirely (mirroring invites and the four privilege
// flags): in-memory Uploaded/Downloaded values on the struct passed to
// Update are silently ignored rather than persisted. IncrementStats (announce
// accrual) and SetStats (admin edit) are the only paths that may change them.
func TestUserRepoUpdateDoesNotWriteStats(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)
	if err := repo.IncrementStats(ctx, u.ID, 100, 50); err != nil {
		t.Fatalf("seed stats: %v", err)
	}

	u.Uploaded = 999999 // in-memory values Update() must not persist
	u.Downloaded = 888888
	if err := repo.Update(ctx, u); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Uploaded != 100 || got.Downloaded != 50 {
		t.Errorf("uploaded=%d downloaded=%d, want 100/50 (Update must not write these columns)", got.Uploaded, got.Downloaded)
	}
}

// TestUserRepoUpdate_DoesNotClobberStats is the regression test for the same
// race class TestUserRepoUpdate_DoesNotClobberPrivilegeFlags closes for the
// privilege flags, applied to uploaded/downloaded — the exact race BE-8.18 was
// written to close: a flow (e.g. an admin profile save) reads a user, and its
// full-row Update() lands AFTER a concurrent announce accrues stats via
// IncrementStats — but was built from data read BEFORE. Because Update() no
// longer writes uploaded/downloaded at all, the stale write must be
// structurally incapable of clobbering the accrual — regardless of timing.
func TestUserRepoUpdate_DoesNotClobberStats(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	// Another flow reads the user before a concurrent announce.
	stale, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	// An announce accrues stats concurrently via the atomic path.
	if err := repo.IncrementStats(ctx, u.ID, 100, 50); err != nil {
		t.Fatalf("IncrementStats: %v", err)
	}

	// The stale flow's own full-row Update() finally lands, carrying
	// Uploaded=Downloaded=0 because that's what it read before the accrual.
	// It also makes an unrelated change, to prove Update still works.
	stale.Title = ptr("unrelated change")
	if err := repo.Update(ctx, stale); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID after stale Update: %v", err)
	}
	if got.Uploaded != 100 || got.Downloaded != 50 {
		t.Errorf("stale full-row Update clobbered concurrently-accrued stats: uploaded=%d downloaded=%d, want 100/50", got.Uploaded, got.Downloaded)
	}
	if got.Title == nil || *got.Title != "unrelated change" {
		t.Error("Update should still persist unrelated columns")
	}
}

func TestUserRepoSetStats(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	uploaded, downloaded := int64(500), int64(250)
	if err := repo.SetStats(ctx, u.ID, &uploaded, &downloaded, nil); err != nil {
		t.Fatalf("SetStats: %v", err)
	}
	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Uploaded != 500 || got.Downloaded != 250 {
		t.Errorf("uploaded=%d downloaded=%d, want 500/250", got.Uploaded, got.Downloaded)
	}

	if err := repo.SetStats(ctx, 999999, &uploaded, &downloaded, nil); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("SetStats(missing user) = %v, want sql.ErrNoRows", err)
	}
}

// TestUserRepoSetStats_PartialUpdateDoesNotClobberConcurrentAccrual proves
// SetStats's COALESCE semantics: an admin edit that only sets one of
// uploaded/downloaded (the other pointer is nil — the field wasn't dirty)
// must not overwrite the untouched counter with a value read at request
// start, which would reopen the exact clobber window this story closes if a
// concurrent announce accrues that counter in between.
func TestUserRepoSetStats_PartialUpdateDoesNotClobberConcurrentAccrual(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	// An admin's edit only touches uploaded; downloaded is deliberately nil,
	// as if the admin only changed the uploaded field on the form.
	uploaded := int64(1000)
	if err := repo.SetStats(ctx, u.ID, &uploaded, nil, nil); err != nil {
		t.Fatalf("SetStats: %v", err)
	}

	// An announce concurrently accrues both counters in between.
	if err := repo.IncrementStats(ctx, u.ID, 10, 20); err != nil {
		t.Fatalf("IncrementStats: %v", err)
	}

	// A second admin edit, still only touching uploaded, must not clobber the
	// downloaded value the announce just accrued.
	uploaded2 := int64(2000)
	if err := repo.SetStats(ctx, u.ID, &uploaded2, nil, nil); err != nil {
		t.Fatalf("SetStats: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Uploaded != 2000 {
		t.Errorf("uploaded = %d, want 2000 (the admin's explicit set)", got.Uploaded)
	}
	if got.Downloaded != 20 {
		t.Errorf("downloaded = %d, want 20 (SetStats(nil) must leave it at the announce-accrued value, not clobber it)", got.Downloaded)
	}
}

// TestUserRepoUpdateWithHistory_RecordsAtomically proves the successful path:
// the full-row write and its audit rows are both visible afterward.
func TestUserRepoUpdateWithHistory_RecordsAtomically(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)
	admin := newUser(t, db)

	u.Title = ptr("Elite")
	entries := []model.UserEditHistory{
		{UserID: u.ID, ChangedBy: &admin.ID, ChangedByUsername: admin.Username, Field: "title", OldValue: "", NewValue: "Elite"},
	}
	if err := repo.UpdateWithHistory(ctx, u, entries); err != nil {
		t.Fatalf("UpdateWithHistory: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Title == nil || *got.Title != "Elite" {
		t.Errorf("title = %v, want Elite", got.Title)
	}

	hist, total, err := NewUserEditHistoryRepo(db).ListByUser(ctx, u.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if total != 1 || len(hist) != 1 || hist[0].Field != "title" {
		t.Fatalf("want 1 title entry, got %+v (total=%d)", hist, total)
	}
}

// TestUserRepoUpdateWithHistory_AuditFailureRollsBackWrite is the regression
// test for this story's headline change: the update and its audit rows now
// commit in one transaction, so an audit insert that fails — here, a
// changed_by referencing a nonexistent admin, which violates the FK — must
// roll back the user update too, not just fail to record while the update
// stands (BE-8.17's prior best-effort behavior).
func TestUserRepoUpdateWithHistory_AuditFailureRollsBackWrite(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	bogusAdmin := int64(999999999)
	u.Title = ptr("should not persist")
	entries := []model.UserEditHistory{
		{UserID: u.ID, ChangedBy: &bogusAdmin, Field: "title", OldValue: "", NewValue: "should not persist"},
	}
	if err := repo.UpdateWithHistory(ctx, u, entries); err == nil {
		t.Fatal("expected an error from the changed_by FK violation")
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Title != nil {
		t.Errorf("title = %v, want nil — a failed audit insert must roll back the update too", got.Title)
	}
}

// TestUserRepoSetStats_AuditFailureRollsBackWrite is SetStats's half of the
// same atomicity guarantee UpdateWithHistory proves above.
func TestUserRepoSetStats_AuditFailureRollsBackWrite(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	bogusAdmin := int64(999999999)
	uploaded := int64(12345)
	entries := []model.UserEditHistory{
		{UserID: u.ID, ChangedBy: &bogusAdmin, Field: "uploaded", OldValue: "0", NewValue: "12345"},
	}
	if err := repo.SetStats(ctx, u.ID, &uploaded, nil, entries); err == nil {
		t.Fatal("expected an error from the changed_by FK violation")
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Uploaded != 0 {
		t.Errorf("uploaded = %d, want 0 — a failed audit insert must roll back the stats write too", got.Uploaded)
	}
}

// TestUserRepoSetInvites_AuditFailureRollsBackWrite is SetInvites's half of
// the same atomicity guarantee.
func TestUserRepoSetInvites_AuditFailureRollsBackWrite(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	bogusAdmin := int64(999999999)
	entries := []model.UserEditHistory{
		{UserID: u.ID, ChangedBy: &bogusAdmin, Field: "invites", OldValue: "0", NewValue: "9"},
	}
	if err := repo.SetInvites(ctx, u.ID, 9, entries); err == nil {
		t.Fatal("expected an error from the changed_by FK violation")
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Invites != 0 {
		t.Errorf("invites = %d, want 0 — a failed audit insert must roll back the invites write too", got.Invites)
	}
}

func TestUserRepoSetPrivilegeFlag(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	cases := []struct {
		restrictionType string
		getter          func(*model.User) bool
	}{
		{model.RestrictionTypeDownload, func(u *model.User) bool { return u.CanDownload }},
		{model.RestrictionTypeUpload, func(u *model.User) bool { return u.CanUpload }},
		{model.RestrictionTypeChat, func(u *model.User) bool { return u.CanChat }},
		{model.RestrictionTypeInvite, func(u *model.User) bool { return u.CanInvite }},
	}

	for _, tc := range cases {
		if err := repo.SetPrivilegeFlag(ctx, u.ID, tc.restrictionType, false); err != nil {
			t.Fatalf("SetPrivilegeFlag(%s, false): %v", tc.restrictionType, err)
		}
		got, err := repo.GetByID(ctx, u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if tc.getter(got) {
			t.Errorf("%s: expected false after SetPrivilegeFlag(false)", tc.restrictionType)
		}

		if err := repo.SetPrivilegeFlag(ctx, u.ID, tc.restrictionType, true); err != nil {
			t.Fatalf("SetPrivilegeFlag(%s, true): %v", tc.restrictionType, err)
		}
		got, err = repo.GetByID(ctx, u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !tc.getter(got) {
			t.Errorf("%s: expected true after SetPrivilegeFlag(true)", tc.restrictionType)
		}
	}
}

func TestUserRepoSetPrivilegeFlag_UnknownType(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	if err := repo.SetPrivilegeFlag(ctx, u.ID, "not_a_real_type", false); err == nil {
		t.Error("expected error for unknown restriction type")
	}
}

func TestUserRepoSetPrivilegeFlag_NotFound(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	if err := repo.SetPrivilegeFlag(ctx, 999999, model.RestrictionTypeDownload, false); err == nil {
		t.Error("expected error for nonexistent user")
	}
}

// TestUserRepoUpdate_DoesNotClobberPrivilegeFlags is the regression test for
// the drift race: another flow (login, invite creation, an admin profile
// save) reads a user, and its full-row Update() lands AFTER a restriction
// has changed a privilege flag but was built from data read BEFORE. Because
// Update() no longer writes can_download/upload/chat/invite at all, the
// stale write must be structurally incapable of clobbering the flag —
// regardless of timing.
func TestUserRepoUpdate_DoesNotClobberPrivilegeFlags(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)
	if !u.CanDownload {
		t.Fatal("fixture user must start with CanDownload=true")
	}

	// Another flow reads the user before any restriction is applied.
	stale, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	// A restriction is applied concurrently via the targeted write path.
	if err := repo.SetPrivilegeFlag(ctx, u.ID, model.RestrictionTypeDownload, false); err != nil {
		t.Fatalf("SetPrivilegeFlag: %v", err)
	}

	// The stale flow's own full-row Update() finally lands, carrying
	// CanDownload=true because that's what it read before the restriction.
	// It also makes an unrelated change, to prove Update still works.
	stale.Title = ptr("unrelated change")
	if err := repo.Update(ctx, stale); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID after stale Update: %v", err)
	}
	if got.CanDownload {
		t.Error("stale full-row Update clobbered a concurrently-applied restriction: can_download should still be false")
	}
	if got.Title == nil || *got.Title != "unrelated change" {
		t.Error("Update should still persist unrelated columns")
	}
}

func TestUserRepoAdjustInvitesAccumulates(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	if err := repo.AdjustInvites(ctx, u.ID, 3); err != nil {
		t.Fatalf("AdjustInvites(+3): %v", err)
	}
	if err := repo.AdjustInvites(ctx, u.ID, -1); err != nil {
		t.Fatalf("AdjustInvites(-1): %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Invites != 2 {
		t.Errorf("invites = %d, want 2 (deltas must accumulate)", got.Invites)
	}

	if err := repo.AdjustInvites(ctx, 999999, 1); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("AdjustInvites(missing user) = %v, want sql.ErrNoRows", err)
	}
}

// TestUserRepoAdjustInvitesDoesNotLoseConcurrentWrite is the regression test
// for the same race class TestUserRepoUpdate_DoesNotClobberPrivilegeFlags
// closes for the privilege flags, applied to the invites balance: a flow
// (e.g. an admin profile save) reads a user, and its full-row Update() lands
// AFTER a concurrent AdjustInvites grant — but was built from data read
// BEFORE. Because Update() no longer writes invites at all, the stale write
// must be structurally incapable of clobbering the grant — regardless of
// timing.
func TestUserRepoAdjustInvitesDoesNotLoseConcurrentWrite(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)
	if err := repo.SetInvites(ctx, u.ID, 5, nil); err != nil {
		t.Fatalf("seed invites: %v", err)
	}

	// Another flow reads the user before a concurrent grant.
	stale, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stale.Invites != 5 {
		t.Fatalf("fixture: expected seeded invites=5, got %d", stale.Invites)
	}

	// A grant (e.g. auto-distribution) applies concurrently via the atomic path.
	if err := repo.AdjustInvites(ctx, u.ID, 1); err != nil {
		t.Fatalf("AdjustInvites: %v", err)
	}

	// The stale flow's own full-row Update() finally lands, carrying
	// Invites=5 because that's what it read before the grant. It also makes an
	// unrelated change, to prove Update still works.
	stale.Title = ptr("unrelated change")
	if err := repo.Update(ctx, stale); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID after stale Update: %v", err)
	}
	if got.Invites != 6 {
		t.Errorf("stale full-row Update clobbered a concurrently-applied grant: invites = %d, want 6 (5 seeded + 1 grant)", got.Invites)
	}
	if got.Title == nil || *got.Title != "unrelated change" {
		t.Error("Update should still persist unrelated columns")
	}
}

func TestUserRepoSetInvitesIsAbsolute(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	if err := repo.AdjustInvites(ctx, u.ID, 7); err != nil {
		t.Fatalf("AdjustInvites: %v", err)
	}
	if err := repo.SetInvites(ctx, u.ID, 2, nil); err != nil {
		t.Fatalf("SetInvites: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Invites != 2 {
		t.Errorf("invites = %d, want 2 (SetInvites is absolute, not a delta)", got.Invites)
	}

	if err := repo.SetInvites(ctx, 999999, 1, nil); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("SetInvites(missing user) = %v, want sql.ErrNoRows", err)
	}
}

func TestUserRepoIncrementStatsAccumulates(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	if err := repo.IncrementStats(ctx, u.ID, 100, 50); err != nil {
		t.Fatalf("IncrementStats: %v", err)
	}
	// A second call must add to the first, not overwrite it.
	if err := repo.IncrementStats(ctx, u.ID, 10, 5); err != nil {
		t.Fatalf("IncrementStats: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Uploaded != 110 || got.Downloaded != 55 {
		t.Errorf("uploaded=%d downloaded=%d, want 110/55 (increments must accumulate)", got.Uploaded, got.Downloaded)
	}
}

func TestUserRepoUpdateLastAccess(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	if err := repo.UpdateLastAccess(ctx, u.ID); err != nil {
		t.Fatalf("UpdateLastAccess: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.LastAccess == nil {
		t.Error("last_access is still NULL after UpdateLastAccess")
	}
}

// TestUserRepoActivatedAtRoundTrips covers the repo-layer plumbing added for
// BE-8.19: Create persists a caller-supplied ActivatedAt, GetByID scans it
// back, and a later full-row Update (e.g. an admin ban, which reads the user
// then writes enabled=false) carries the existing value forward rather than
// dropping it back to NULL.
func TestUserRepoActivatedAtRoundTrips(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)

	t.Run("nil at create means never activated", func(t *testing.T) {
		u := newUser(t, db)
		if u.ActivatedAt != nil {
			t.Fatalf("fixture user should start with ActivatedAt=nil, got %v", *u.ActivatedAt)
		}

		got, err := repo.GetByID(ctx, u.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ActivatedAt != nil {
			t.Errorf("ActivatedAt = %v, want nil", *got.ActivatedAt)
		}
	})

	t.Run("Create persists a supplied ActivatedAt and Update does not clobber it", func(t *testing.T) {
		name := uniq("user")
		activatedAt := time.Now().Add(-time.Hour).Truncate(time.Second)
		u := &model.User{
			Username:       name,
			Email:          name + "@example.test",
			PasswordHash:   "hash",
			PasswordScheme: "argon2id",
			GroupID:        anyGroupID(t, db),
			Enabled:        true,
			ActivatedAt:    &activatedAt,
		}
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := repo.GetByID(ctx, u.ID)
		if err != nil {
			t.Fatalf("GetByID after Create: %v", err)
		}
		if got.ActivatedAt == nil || !got.ActivatedAt.Equal(activatedAt) {
			t.Fatalf("ActivatedAt after Create = %v, want %v", got.ActivatedAt, activatedAt)
		}

		// Simulate an admin ban: read-modify-write the whole row, changing only
		// Enabled. ActivatedAt must survive since it was carried in `got`.
		got.Enabled = false
		if err := repo.Update(ctx, got); err != nil {
			t.Fatalf("Update: %v", err)
		}

		afterBan, err := repo.GetByID(ctx, u.ID)
		if err != nil {
			t.Fatalf("GetByID after Update: %v", err)
		}
		if afterBan.ActivatedAt == nil || !afterBan.ActivatedAt.Equal(activatedAt) {
			t.Errorf("ActivatedAt after ban Update = %v, want unchanged %v", afterBan.ActivatedAt, activatedAt)
		}
		if afterBan.Enabled {
			t.Error("Enabled should be false after the simulated ban")
		}
	})
}

func TestUserRepoCountAndList(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	a := newUser(t, db)
	newUser(t, db)
	newUser(t, db)

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 3 {
		t.Errorf("Count = %d, want 3", count)
	}

	t.Run("search narrows to one", func(t *testing.T) {
		users, total, err := repo.List(ctx, repository.ListUsersOptions{
			Search: a.Username, Page: 1, PerPage: 10,
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 1 || len(users) != 1 || users[0].ID != a.ID {
			t.Errorf("search for %q returned total=%d users=%d, want exactly the one user", a.Username, total, len(users))
		}
	})

	t.Run("pagination splits the result", func(t *testing.T) {
		page1, total, err := repo.List(ctx, repository.ListUsersOptions{Page: 1, PerPage: 2})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 3 || len(page1) != 2 {
			t.Errorf("page 1: total=%d len=%d, want total 3 and 2 rows", total, len(page1))
		}

		page2, _, err := repo.List(ctx, repository.ListUsersOptions{Page: 2, PerPage: 2})
		if err != nil {
			t.Fatalf("List page 2: %v", err)
		}
		if len(page2) != 1 {
			t.Errorf("page 2: len=%d, want the remaining 1 row", len(page2))
		}
	})

	t.Run("enabled filter", func(t *testing.T) {
		a.Enabled = false
		if err := repo.Update(ctx, a); err != nil {
			t.Fatalf("Update: %v", err)
		}
		users, _, err := repo.List(ctx, repository.ListUsersOptions{
			Enabled: ptr(false), Page: 1, PerPage: 10,
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(users) != 1 || users[0].ID != a.ID {
			t.Errorf("enabled=false returned %d users, want just the disabled one", len(users))
		}
	})
}

// DisabledUntilBefore backs the maintenance job that re-enables expired bans,
// so it must match only users whose ban window has actually elapsed.
func TestUserRepoListDisabledUntilBefore(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)

	expired := newUser(t, db)
	expired.DisabledUntil = ptr(time.Now().Add(-48 * time.Hour))
	if err := repo.Update(ctx, expired); err != nil {
		t.Fatalf("Update: %v", err)
	}

	stillBanned := newUser(t, db)
	stillBanned.DisabledUntil = ptr(time.Now().Add(24 * time.Hour))
	if err := repo.Update(ctx, stillBanned); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// A user with no ban at all must never match.
	newUser(t, db)

	users, _, err := repo.List(ctx, repository.ListUsersOptions{
		DisabledUntilBefore: ptr(time.Now()), Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) != 1 || users[0].ID != expired.ID {
		t.Errorf("got %d users, want only the one whose ban has expired", len(users))
	}
}

func TestUserRepoListStaff(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)
	u := newUser(t, db)

	// Promote into the highest-level group, which is what makes a user staff.
	var staffGroup int64
	if err := db.QueryRow(`SELECT id FROM groups ORDER BY level DESC LIMIT 1`).Scan(&staffGroup); err != nil {
		t.Fatalf("finding staff group: %v", err)
	}
	u.GroupID = staffGroup
	if err := repo.Update(ctx, u); err != nil {
		t.Fatalf("Update: %v", err)
	}

	staff, err := repo.ListStaff(ctx)
	if err != nil {
		t.Fatalf("ListStaff: %v", err)
	}
	for _, s := range staff {
		if s.ID == u.ID {
			return
		}
	}
	t.Errorf("promoted user %d not present in ListStaff (%d returned)", u.ID, len(staff))
}

func TestGroupRepoListAndGet(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	repo := NewGroupRepo(db)

	groups, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("no groups — the migrations should have seeded them")
	}

	got, err := repo.GetByID(ctx, groups[0].ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != groups[0].Name {
		t.Errorf("name = %q, want %q", got.Name, groups[0].Name)
	}
}
