package service

import (
	"context"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// An Uploader-class member (CanSelfApprove) may approve their own pending upload,
// recorded under their own name — but not anyone else's, and a plain member can't
// self-approve at all (BE-8.22c).
func TestApproveTorrent_UploaderSelfApprove(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, _ := newSvcWithModeration(nil)
	uploaderPerms := model.Permissions{CanSelfApprove: true}

	own := &model.Torrent{UploaderID: 5, ModerationStatus: model.ModerationPending}
	_ = repo.Create(ctx, own)
	other := &model.Torrent{UploaderID: 6, ModerationStatus: model.ModerationPending}
	_ = repo.Create(ctx, other)
	plain := &model.Torrent{UploaderID: 7, ModerationStatus: model.ModerationPending}
	_ = repo.Create(ctx, plain)

	// Uploader approves their own → allowed, self recorded as approver.
	got, err := svc.ApproveTorrent(ctx, own.ID, 5, uploaderPerms)
	if err != nil {
		t.Fatalf("self-approve own: %v", err)
	}
	if got.ApprovedBy == nil || *got.ApprovedBy != 5 {
		t.Errorf("approved_by = %v, want 5 (self)", got.ApprovedBy)
	}

	// Uploader cannot approve a torrent they didn't upload.
	if _, err := svc.ApproveTorrent(ctx, other.ID, 5, uploaderPerms); !errors.Is(err, ErrForbidden) {
		t.Errorf("approve other's torrent err = %v, want ErrForbidden", err)
	}

	// A plain member (no CanSelfApprove) can't approve even their own.
	if _, err := svc.ApproveTorrent(ctx, plain.ID, 7, model.Permissions{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("plain self-approve err = %v, want ErrForbidden", err)
	}
}
