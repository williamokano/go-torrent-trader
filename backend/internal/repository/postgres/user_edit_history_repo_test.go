package postgres

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func TestUserEditHistoryRepo_RecordAndList(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewUserEditHistoryRepo(db)

	target := newUser(t, db)
	admin := newUser(t, db)

	entries := []model.UserEditHistory{
		{UserID: target.ID, ChangedBy: &admin.ID, Field: "uploaded", OldValue: "0", NewValue: "1099511627776"},
		{UserID: target.ID, ChangedBy: &admin.ID, Field: "invites", OldValue: "2", NewValue: "5"},
	}
	if err := repo.Record(ctx, entries); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, total, err := repo.ListByUser(ctx, target.ID, 50, 0)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if total != 2 || len(got) != 2 {
		t.Fatalf("want 2 entries, got len=%d total=%d", len(got), total)
	}
	// Newest-first: same created_at is broken by id DESC, so invites comes first.
	if got[0].Field != "invites" || got[1].Field != "uploaded" {
		t.Errorf("want newest-first [invites uploaded], got [%s %s]", got[0].Field, got[1].Field)
	}
	if got[1].OldValue != "0" || got[1].NewValue != "1099511627776" {
		t.Errorf("uploaded entry values wrong: old=%q new=%q", got[1].OldValue, got[1].NewValue)
	}
	if got[0].ChangedBy == nil || *got[0].ChangedBy != admin.ID {
		t.Errorf("want changed_by=%d, got %v", admin.ID, got[0].ChangedBy)
	}
	if got[0].ChangedByUsername != admin.Username {
		t.Errorf("want changed_by_username=%q, got %q", admin.Username, got[0].ChangedByUsername)
	}
}

func TestUserEditHistoryRepo_RecordEmptyIsNoop(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	repo := NewUserEditHistoryRepo(db)

	if err := repo.Record(context.Background(), nil); err != nil {
		t.Fatalf("Record(nil): %v", err)
	}
}

func TestUserEditHistoryRepo_ListPagination(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewUserEditHistoryRepo(db)

	target := newUser(t, db)
	admin := newUser(t, db)

	var entries []model.UserEditHistory
	for _, f := range []string{"a", "b", "c"} {
		entries = append(entries, model.UserEditHistory{
			UserID: target.ID, ChangedBy: &admin.ID, Field: f, OldValue: "old", NewValue: "new",
		})
	}
	if err := repo.Record(ctx, entries); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, total, err := repo.ListByUser(ctx, target.ID, 2, 1)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if total != 3 {
		t.Errorf("want total=3, got %d", total)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries with limit=2 offset=1, got %d", len(got))
	}
	// Full order is [c b a]; offset 1 gives [b a].
	if got[0].Field != "b" || got[1].Field != "a" {
		t.Errorf("want [b a], got [%s %s]", got[0].Field, got[1].Field)
	}
}

func TestUserEditHistoryRepo_DeletedAdminNullsChangedBy(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewUserEditHistoryRepo(db)

	target := newUser(t, db)
	admin := newUser(t, db)

	entry := []model.UserEditHistory{
		{UserID: target.ID, ChangedBy: &admin.ID, ChangedByUsername: admin.Username, Field: "email", OldValue: "a@x.test", NewValue: "b@x.test"},
	}
	if err := repo.Record(ctx, entry); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id = $1`, admin.ID); err != nil {
		t.Fatalf("delete admin: %v", err)
	}

	got, _, err := repo.ListByUser(ctx, target.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if got[0].ChangedBy != nil {
		t.Errorf("want changed_by NULL after admin deletion, got %v", *got[0].ChangedBy)
	}
	// The write-time snapshot keeps attribution after the admin is gone.
	if got[0].ChangedByUsername != admin.Username {
		t.Errorf("want snapshotted changed_by_username %q, got %q", admin.Username, got[0].ChangedByUsername)
	}
}

func TestUserEditHistoryRepo_SurvivesSubjectDeletion(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewUserEditHistoryRepo(db)

	target := newUser(t, db)
	admin := newUser(t, db)

	entry := []model.UserEditHistory{
		{UserID: target.ID, ChangedBy: &admin.ID, ChangedByUsername: admin.Username, Field: "uploaded", OldValue: "0", NewValue: "1"},
	}
	if err := repo.Record(ctx, entry); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id = $1`, target.ID); err != nil {
		t.Fatalf("delete target: %v", err)
	}

	// Audit rows must outlive their subject — deleting an account cannot
	// destroy the evidence of the edits made to it.
	got, total, err := repo.ListByUser(ctx, target.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if total != 1 || len(got) != 1 {
		t.Fatalf("want the entry to survive user deletion, got len=%d total=%d", len(got), total)
	}
}
