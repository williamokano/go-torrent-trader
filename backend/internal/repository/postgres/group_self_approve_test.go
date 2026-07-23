package postgres

import (
	"context"
	"testing"
)

// The Uploader class is seeded by migration 070 with can_self_approve = true, and
// the group reads must surface it (so it reaches Permissions → the session).
func TestGroupRepo_UploaderGroupHasSelfApprove(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()
	repo := NewGroupRepo(db)

	groups, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var found bool
	for _, g := range groups {
		if g.Slug == "uploader" {
			found = true
			if !g.CanSelfApprove {
				t.Error("Uploader group should have can_self_approve = true")
			}
			// GetByID must carry it too.
			got, err := repo.GetByID(ctx, g.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if !got.CanSelfApprove {
				t.Error("GetByID(Uploader) should have can_self_approve = true")
			}
		} else if g.CanSelfApprove {
			t.Errorf("group %q unexpectedly has can_self_approve = true", g.Slug)
		}
	}
	if !found {
		t.Fatal("seeded Uploader group not found")
	}
}
