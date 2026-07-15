package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func TestGroupRepo_CreateUpdateDelete(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewGroupRepo(db)

	name := uniq("Group")
	slug := uniq("group")
	color := "#123456"
	g := &model.Group{
		Name: name, Slug: slug, Level: 42, Color: &color,
		CanUpload: true, CanDownload: true, CanForum: true, IsModerator: true,
	}
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if g.ID == 0 || g.CreatedAt.IsZero() || g.UpdatedAt.IsZero() {
		t.Fatalf("Create did not populate id/timestamps: %+v", g)
	}

	got, err := repo.GetByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != name || got.Level != 42 || got.Color == nil || *got.Color != color || !got.IsModerator {
		t.Errorf("stored group mismatch: %+v", got)
	}

	// Update: change level, color, and permissions.
	g.Level = 55
	newColor := "#ABCDEF"
	g.Color = &newColor
	g.IsModerator = false
	g.CanInvite = true
	if err := repo.Update(ctx, g); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = repo.GetByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Level != 55 || got.Color == nil || *got.Color != newColor || got.IsModerator || !got.CanInvite {
		t.Errorf("update not persisted: %+v", got)
	}

	// Delete the (member-less) group.
	if err := repo.Delete(ctx, g.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, g.ID); err == nil {
		t.Error("GetByID succeeded after delete")
	}
}

func TestGroupRepo_UpdateAndDeleteMissing(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewGroupRepo(db)

	missing := &model.Group{ID: 999999, Name: uniq("Ghost"), Slug: uniq("ghost")}
	if err := repo.Update(ctx, missing); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Update(missing) = %v, want sql.ErrNoRows", err)
	}
	if err := repo.Delete(ctx, 999999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Delete(missing) = %v, want sql.ErrNoRows", err)
	}
}

func TestGroupRepo_CountMembers(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewGroupRepo(db)

	g := &model.Group{Name: uniq("Group"), Slug: uniq("group"), Level: 10}
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create: %v", err)
	}

	n, err := repo.CountMembers(ctx, g.ID)
	if err != nil {
		t.Fatalf("CountMembers: %v", err)
	}
	if n != 0 {
		t.Fatalf("CountMembers on empty group = %d, want 0", n)
	}

	// Move two users into the group and re-count.
	for i := 0; i < 2; i++ {
		u := newUser(t, db)
		if _, err := db.ExecContext(ctx, `UPDATE users SET group_id = $1 WHERE id = $2`, g.ID, u.ID); err != nil {
			t.Fatalf("assigning user to group: %v", err)
		}
	}

	n, err = repo.CountMembers(ctx, g.ID)
	if err != nil {
		t.Fatalf("CountMembers: %v", err)
	}
	if n != 2 {
		t.Errorf("CountMembers = %d, want 2", n)
	}
}
