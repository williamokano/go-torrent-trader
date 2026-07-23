package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// newPendingTorrent inserts a torrent awaiting moderation.
func newPendingTorrent(t *testing.T, db *sql.DB, uploaderID int64) *model.Torrent {
	t.Helper()
	tor := &model.Torrent{
		Name:             uniq("Pending"),
		InfoHash:         infoHash(uniq("phash")),
		Size:             1024,
		CategoryID:       newCategoryID(t, db),
		UploaderID:       uploaderID,
		Visible:          true,
		ModerationStatus: model.ModerationPending,
		FileCount:        1,
	}
	if err := NewTorrentRepo(db).Create(context.Background(), tor); err != nil {
		t.Fatalf("creating pending torrent: %v", err)
	}
	return tor
}

func TestTorrentModeration_ClaimApproveRejectUnclaim(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewTorrentRepo(db)

	uploader := newUser(t, db)
	moderator := newUser(t, db)
	approver := newUser(t, db)

	t.Run("claim assigns the moderator", func(t *testing.T) {
		tor := newPendingTorrent(t, db, uploader.ID)
		if err := repo.ClaimModeration(ctx, tor.ID, moderator.ID); err != nil {
			t.Fatalf("ClaimModeration: %v", err)
		}
		got, err := repo.GetByID(ctx, tor.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.AssignedModeratorID == nil || *got.AssignedModeratorID != moderator.ID {
			t.Fatalf("assigned moderator id = %v, want %d", got.AssignedModeratorID, moderator.ID)
		}
		if got.AssignedModeratorName != moderator.Username {
			t.Errorf("assigned moderator name = %q, want %q", got.AssignedModeratorName, moderator.Username)
		}

		// Unclaim clears it.
		if err := repo.UnclaimModeration(ctx, tor.ID); err != nil {
			t.Fatalf("UnclaimModeration: %v", err)
		}
		got, _ = repo.GetByID(ctx, tor.ID)
		if got.AssignedModeratorID != nil {
			t.Errorf("assigned moderator id = %v after unclaim, want nil", got.AssignedModeratorID)
		}
	})

	t.Run("approve records the approver and timestamp", func(t *testing.T) {
		tor := newPendingTorrent(t, db, uploader.ID)
		if err := repo.ApproveTorrent(ctx, tor.ID, approver.ID); err != nil {
			t.Fatalf("ApproveTorrent: %v", err)
		}
		got, err := repo.GetByID(ctx, tor.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ModerationStatus != model.ModerationApproved {
			t.Errorf("status = %q, want approved", got.ModerationStatus)
		}
		if got.ApprovedBy == nil || *got.ApprovedBy != approver.ID {
			t.Fatalf("approved_by = %v, want %d", got.ApprovedBy, approver.ID)
		}
		if got.ApprovedByName != approver.Username {
			t.Errorf("approved_by_name = %q, want %q", got.ApprovedByName, approver.Username)
		}
		if got.ApprovedAt == nil {
			t.Error("approved_at is nil, want a timestamp")
		}
	})

	t.Run("reject clears any approval", func(t *testing.T) {
		tor := newPendingTorrent(t, db, uploader.ID)
		if err := repo.RejectTorrent(ctx, tor.ID); err != nil {
			t.Fatalf("RejectTorrent: %v", err)
		}
		got, _ := repo.GetByID(ctx, tor.ID)
		if got.ModerationStatus != model.ModerationRejected {
			t.Errorf("status = %q, want rejected", got.ModerationStatus)
		}
		if got.ApprovedBy != nil {
			t.Errorf("approved_by = %v, want nil", got.ApprovedBy)
		}
	})

	t.Run("acting on a missing torrent returns ErrNoRows", func(t *testing.T) {
		if err := repo.ClaimModeration(ctx, 999999, moderator.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ClaimModeration(missing) err = %v, want sql.ErrNoRows", err)
		}
		if err := repo.ApproveTorrent(ctx, 999999, approver.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("ApproveTorrent(missing) err = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestListModerationQueue_Filters(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewTorrentRepo(db)

	uploader := newUser(t, db)
	modA := newUser(t, db)
	modB := newUser(t, db)

	// t1 unassigned, t2 claimed by modA, t3 claimed by modB (created in order).
	t1 := newPendingTorrent(t, db, uploader.ID)
	t2 := newPendingTorrent(t, db, uploader.ID)
	t3 := newPendingTorrent(t, db, uploader.ID)
	if err := repo.ClaimModeration(ctx, t2.ID, modA.ID); err != nil {
		t.Fatalf("claim t2: %v", err)
	}
	if err := repo.ClaimModeration(ctx, t3.ID, modB.ID); err != nil {
		t.Fatalf("claim t3: %v", err)
	}
	// An approved torrent must never appear in the pending queue.
	newTorrent(t, db, uploader.ID)

	t.Run("all pending, oldest first", func(t *testing.T) {
		got, total, err := repo.ListModerationQueue(ctx, repository.ModerationQueueOptions{
			Assigned: repository.ModAssignedAll,
		})
		if err != nil {
			t.Fatalf("ListModerationQueue: %v", err)
		}
		if total != 3 {
			t.Fatalf("total = %d, want 3", total)
		}
		if got[0].ID != t1.ID || got[1].ID != t2.ID || got[2].ID != t3.ID {
			t.Errorf("order = [%d %d %d], want [%d %d %d]", got[0].ID, got[1].ID, got[2].ID, t1.ID, t2.ID, t3.ID)
		}
	})

	t.Run("unassigned only", func(t *testing.T) {
		got, total, err := repo.ListModerationQueue(ctx, repository.ModerationQueueOptions{
			Assigned: repository.ModAssignedUnassigned,
		})
		if err != nil {
			t.Fatalf("ListModerationQueue: %v", err)
		}
		if total != 1 || got[0].ID != t1.ID {
			t.Errorf("unassigned = %v (total %d), want [%d]", ids(got), total, t1.ID)
		}
	})

	t.Run("mine only", func(t *testing.T) {
		got, total, err := repo.ListModerationQueue(ctx, repository.ModerationQueueOptions{
			Assigned:    repository.ModAssignedMine,
			ModeratorID: modA.ID,
		})
		if err != nil {
			t.Fatalf("ListModerationQueue: %v", err)
		}
		if total != 1 || got[0].ID != t2.ID {
			t.Errorf("mine(modA) = %v (total %d), want [%d]", ids(got), total, t2.ID)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		got, total, err := repo.ListModerationQueue(ctx, repository.ModerationQueueOptions{
			Assigned: repository.ModAssignedAll,
			Page:     2,
			PerPage:  1,
		})
		if err != nil {
			t.Fatalf("ListModerationQueue: %v", err)
		}
		if total != 3 {
			t.Fatalf("total = %d, want 3", total)
		}
		if len(got) != 1 || got[0].ID != t2.ID {
			t.Errorf("page 2 (size 1) = %v, want [%d]", ids(got), t2.ID)
		}
	})
}

func TestList_ExcludesNonApproved(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewTorrentRepo(db)

	uploader := newUser(t, db)
	approved := newTorrent(t, db, uploader.ID)
	pending := newPendingTorrent(t, db, uploader.ID)

	got, total, err := repo.List(ctx, repository.ListTorrentsOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].ID != approved.ID {
		t.Errorf("public List = %v (total %d), want only approved [%d]", ids(got), total, approved.ID)
	}

	// Admin context sees everything.
	_, totalAll, err := repo.List(ctx, repository.ListTorrentsOptions{IncludeHidden: true})
	if err != nil {
		t.Fatalf("List(IncludeHidden): %v", err)
	}
	if totalAll != 2 {
		t.Errorf("admin List total = %d, want 2 (incl pending %d)", totalAll, pending.ID)
	}
}

func ids(ts []model.Torrent) []int64 {
	out := make([]int64, len(ts))
	for i := range ts {
		out[i] = ts[i].ID
	}
	return out
}
