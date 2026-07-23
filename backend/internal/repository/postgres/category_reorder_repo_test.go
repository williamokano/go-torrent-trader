package postgres

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

func TestCategoryRepoReorder(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewCategoryRepo(db)

	a := newCategoryID(t, db)
	b := newCategoryID(t, db)
	c := newCategoryID(t, db)

	t.Run("applies parent and sort order", func(t *testing.T) {
		err := repo.Reorder(ctx, []repository.CategoryPlacement{
			{ID: a, ParentID: nil, SortOrder: 1},
			{ID: b, ParentID: nil, SortOrder: 0},
			{ID: c, ParentID: &a, SortOrder: 0}, // c becomes a child of a
		})
		if err != nil {
			t.Fatalf("Reorder: %v", err)
		}

		gotC, err := repo.GetByID(ctx, c)
		if err != nil {
			t.Fatalf("GetByID(c): %v", err)
		}
		if gotC.ParentID == nil || *gotC.ParentID != a {
			t.Errorf("c parent = %v, want %d", gotC.ParentID, a)
		}
		gotA, _ := repo.GetByID(ctx, a)
		if gotA.SortOrder != 1 {
			t.Errorf("a sort_order = %d, want 1", gotA.SortOrder)
		}
	})

	t.Run("rolls back the whole batch on a constraint violation", func(t *testing.T) {
		bad := int64(999999999)
		err := repo.Reorder(ctx, []repository.CategoryPlacement{
			{ID: b, ParentID: nil, SortOrder: 42}, // would succeed alone
			{ID: c, ParentID: &bad, SortOrder: 0}, // FK violation on parent_id
		})
		if err == nil {
			t.Fatal("expected an error from the invalid parent FK")
		}
		// b's earlier update in the same tx must have been rolled back.
		gotB, _ := repo.GetByID(ctx, b)
		if gotB.SortOrder == 42 {
			t.Error("b's sort_order changed despite the batch failing (not atomic)")
		}
	})

	t.Run("empty placements is a no-op", func(t *testing.T) {
		if err := repo.Reorder(ctx, nil); err != nil {
			t.Errorf("Reorder(nil) = %v, want nil", err)
		}
	})
}
