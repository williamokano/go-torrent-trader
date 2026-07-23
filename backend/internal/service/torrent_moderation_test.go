package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// fakeModerationRepo implements repository.TorrentModerationRepository backed by a
// memTorrentRepo, so the service's post-write GetByID reflects each mutation.
type fakeModerationRepo struct {
	repo  *memTorrentRepo
	queue []model.Torrent
	err   error
}

func (f *fakeModerationRepo) find(id int64) *model.Torrent {
	for _, t := range f.repo.torrents {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (f *fakeModerationRepo) ClaimModeration(_ context.Context, id, mod int64) error {
	t := f.find(id)
	if t == nil {
		return sql.ErrNoRows
	}
	t.AssignedModeratorID = &mod
	return nil
}

func (f *fakeModerationRepo) UnclaimModeration(_ context.Context, id int64) error {
	t := f.find(id)
	if t == nil {
		return sql.ErrNoRows
	}
	t.AssignedModeratorID = nil
	return nil
}

func (f *fakeModerationRepo) ApproveTorrent(_ context.Context, id, approver int64) error {
	t := f.find(id)
	if t == nil {
		return sql.ErrNoRows
	}
	t.ModerationStatus = model.ModerationApproved
	t.ApprovedBy = &approver
	return nil
}

func (f *fakeModerationRepo) RejectTorrent(_ context.Context, id int64) error {
	t := f.find(id)
	if t == nil {
		return sql.ErrNoRows
	}
	t.ModerationStatus = model.ModerationRejected
	t.ApprovedBy = nil
	return nil
}

func (f *fakeModerationRepo) ListModerationQueue(_ context.Context, _ repository.ModerationQueueOptions) ([]model.Torrent, int64, error) {
	return f.queue, int64(len(f.queue)), f.err
}

func settingsWith(kv map[string]string) *SiteSettingsService {
	repo := newMockSiteSettingsRepo()
	for k, v := range kv {
		_ = repo.Set(context.Background(), k, v)
	}
	return NewSiteSettingsService(repo, event.NewInMemoryBus())
}

func newSvcWithModeration(settings *SiteSettingsService) (*TorrentService, *memTorrentRepo, *memUserRepo, *fakeModerationRepo) {
	repo := newMemTorrentRepo()
	userRepo := newMemUserRepo()
	svc := NewTorrentService(nil, repo, userRepo, newMemStorage(),
		TorrentServiceConfig{AnnounceURL: "http://localhost/announce"}, event.NewInMemoryBus(), nil)
	mod := &fakeModerationRepo{repo: repo}
	svc.SetModerationRepo(mod)
	if settings != nil {
		svc.SetSiteSettings(settings)
	}
	return svc, repo, userRepo, mod
}

func TestUpload_ModerationDefault(t *testing.T) {
	ctx := context.Background()

	t.Run("pending by default", func(t *testing.T) {
		svc, _, userRepo, _ := newSvcWithModeration(nil)
		userRepo.addUser(&model.User{ID: 1})
		tor, err := svc.Upload(ctx, buildTorrentFile("modx"), UploadTorrentRequest{CategoryID: 1}, 1)
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if tor.ModerationStatus != model.ModerationPending {
			t.Errorf("status = %q, want pending", tor.ModerationStatus)
		}
	})

	t.Run("auto-approved when moderation disabled", func(t *testing.T) {
		settings := settingsWith(map[string]string{SettingModerationEnabled: "false"})
		svc, _, userRepo, _ := newSvcWithModeration(settings)
		userRepo.addUser(&model.User{ID: 1})
		tor, err := svc.Upload(ctx, buildTorrentFile("mody"), UploadTorrentRequest{CategoryID: 1}, 1)
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if tor.ModerationStatus != model.ModerationApproved {
			t.Errorf("status = %q, want approved (moderation disabled)", tor.ModerationStatus)
		}
	})
}

func TestGetByIDForViewer(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, _ := newSvcWithModeration(nil)
	pend := &model.Torrent{UploaderID: 5, ModerationStatus: model.ModerationPending}
	_ = repo.Create(ctx, pend)

	if _, err := svc.GetByIDForViewer(ctx, pend.ID, 5, model.Permissions{}); err != nil {
		t.Errorf("owner should see pending torrent: %v", err)
	}
	if _, err := svc.GetByIDForViewer(ctx, pend.ID, 99, model.Permissions{IsModerator: true}); err != nil {
		t.Errorf("staff should see pending torrent: %v", err)
	}
	if _, err := svc.GetByIDForViewer(ctx, pend.ID, 99, model.Permissions{}); !errors.Is(err, ErrTorrentNotFound) {
		t.Errorf("other viewer err = %v, want ErrTorrentNotFound", err)
	}
}

func TestGetByIDForViewer_PublicVisibility(t *testing.T) {
	ctx := context.Background()
	settings := settingsWith(map[string]string{SettingModerationPublicVisibility: "true"})
	svc, repo, _, _ := newSvcWithModeration(settings)

	pend := &model.Torrent{UploaderID: 5, ModerationStatus: model.ModerationPending}
	_ = repo.Create(ctx, pend)
	rej := &model.Torrent{UploaderID: 5, ModerationStatus: model.ModerationRejected}
	_ = repo.Create(ctx, rej)

	if _, err := svc.GetByIDForViewer(ctx, pend.ID, 99, model.Permissions{}); err != nil {
		t.Errorf("public visibility should reveal pending torrent: %v", err)
	}
	if _, err := svc.GetByIDForViewer(ctx, rej.ID, 99, model.Permissions{}); !errors.Is(err, ErrTorrentNotFound) {
		t.Errorf("rejected torrent must stay hidden even with public visibility, got %v", err)
	}
}

func TestDownloadTorrent_PendingGate(t *testing.T) {
	ctx := context.Background()
	svc, repo, userRepo, _ := newSvcWithModeration(nil)
	userRepo.addUser(&model.User{ID: 99})
	pend := &model.Torrent{UploaderID: 5, ModerationStatus: model.ModerationPending}
	_ = repo.Create(ctx, pend)

	if _, _, err := svc.DownloadTorrent(ctx, pend.ID, 99, model.Permissions{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-owner/non-staff download err = %v, want ErrForbidden", err)
	}
	// Staff clears the moderation gate (the subsequent storage miss is a different error).
	if _, _, err := svc.DownloadTorrent(ctx, pend.ID, 99, model.Permissions{IsModerator: true}); errors.Is(err, ErrForbidden) {
		t.Error("staff should clear the pending download gate")
	}
}

func TestApproveTorrent(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, _ := newSvcWithModeration(nil)
	staff := model.Permissions{IsAdmin: true}
	pend := &model.Torrent{UploaderID: 5, ModerationStatus: model.ModerationPending}
	_ = repo.Create(ctx, pend)

	if _, err := svc.ApproveTorrent(ctx, pend.ID, 7, model.Permissions{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-staff approve err = %v, want ErrForbidden", err)
	}

	got, err := svc.ApproveTorrent(ctx, pend.ID, 7, staff)
	if err != nil {
		t.Fatalf("staff approve: %v", err)
	}
	if got.ModerationStatus != model.ModerationApproved {
		t.Errorf("status = %q, want approved", got.ModerationStatus)
	}
	if got.ApprovedBy == nil || *got.ApprovedBy != 7 {
		t.Errorf("approved_by = %v, want 7", got.ApprovedBy)
	}

	if _, err := svc.ApproveTorrent(ctx, pend.ID, 7, staff); !errors.Is(err, ErrNotPending) {
		t.Errorf("re-approve err = %v, want ErrNotPending", err)
	}
	if _, err := svc.ApproveTorrent(ctx, 999, 7, staff); !errors.Is(err, ErrTorrentNotFound) {
		t.Errorf("approve missing err = %v, want ErrTorrentNotFound", err)
	}
}

func TestClaimModeration_RejectsNonPending(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, _ := newSvcWithModeration(nil)
	appr := &model.Torrent{UploaderID: 5, ModerationStatus: model.ModerationApproved}
	_ = repo.Create(ctx, appr)

	if _, err := svc.ClaimModeration(ctx, appr.ID, 7); !errors.Is(err, ErrNotPending) {
		t.Errorf("claim approved err = %v, want ErrNotPending", err)
	}
}

func TestRejectTorrent_StaffOnly(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, _ := newSvcWithModeration(nil)
	pend := &model.Torrent{UploaderID: 5, ModerationStatus: model.ModerationPending}
	_ = repo.Create(ctx, pend)

	if _, err := svc.RejectTorrent(ctx, pend.ID, 7, model.Permissions{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-staff reject err = %v, want ErrForbidden", err)
	}
	got, err := svc.RejectTorrent(ctx, pend.ID, 7, model.Permissions{IsModerator: true})
	if err != nil {
		t.Fatalf("staff reject: %v", err)
	}
	if got.ModerationStatus != model.ModerationRejected {
		t.Errorf("status = %q, want rejected", got.ModerationStatus)
	}
}

func TestModeration_Unavailable(t *testing.T) {
	ctx := context.Background()
	repo := newMemTorrentRepo()
	userRepo := newMemUserRepo()
	svc := NewTorrentService(nil, repo, userRepo, newMemStorage(),
		TorrentServiceConfig{}, event.NewInMemoryBus(), nil)
	// moderation repo intentionally not wired.
	if _, err := svc.ApproveTorrent(ctx, 1, 1, model.Permissions{IsAdmin: true}); !errors.Is(err, ErrModerationUnavailable) {
		t.Errorf("approve err = %v, want ErrModerationUnavailable", err)
	}
	if _, _, err := svc.ListModerationQueue(ctx, repository.ModerationQueueOptions{}); !errors.Is(err, ErrModerationUnavailable) {
		t.Errorf("queue err = %v, want ErrModerationUnavailable", err)
	}
}

func TestListModerationQueue_Delegates(t *testing.T) {
	ctx := context.Background()
	svc, _, _, mod := newSvcWithModeration(nil)
	mod.queue = []model.Torrent{{ID: 1}, {ID: 2}}
	got, total, err := svc.ListModerationQueue(ctx, repository.ModerationQueueOptions{})
	if err != nil {
		t.Fatalf("ListModerationQueue: %v", err)
	}
	if total != 2 || len(got) != 2 {
		t.Errorf("queue = %d items (total %d), want 2", len(got), total)
	}
}
