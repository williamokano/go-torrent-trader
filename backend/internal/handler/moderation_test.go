package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// stubModerationRepo implements repository.TorrentModerationRepository over the
// shared stubTorrentRepo map so reads reflect writes.
type stubModerationRepo struct {
	torrents map[int64]*model.Torrent
	queue    []model.Torrent
}

func (s *stubModerationRepo) ClaimModeration(_ context.Context, id, mod int64) error {
	t, ok := s.torrents[id]
	if !ok {
		return sql.ErrNoRows
	}
	t.AssignedModeratorID = &mod
	return nil
}

func (s *stubModerationRepo) UnclaimModeration(_ context.Context, id int64) error {
	t, ok := s.torrents[id]
	if !ok {
		return sql.ErrNoRows
	}
	t.AssignedModeratorID = nil
	return nil
}

func (s *stubModerationRepo) ApproveTorrent(_ context.Context, id, approver int64) error {
	t, ok := s.torrents[id]
	if !ok {
		return sql.ErrNoRows
	}
	t.ModerationStatus = model.ModerationApproved
	t.ApprovedBy = &approver
	return nil
}

func (s *stubModerationRepo) RejectTorrent(_ context.Context, id int64) error {
	t, ok := s.torrents[id]
	if !ok {
		return sql.ErrNoRows
	}
	t.ModerationStatus = model.ModerationRejected
	return nil
}

func (s *stubModerationRepo) ListModerationQueue(_ context.Context, _ repository.ModerationQueueOptions) ([]model.Torrent, int64, error) {
	return s.queue, int64(len(s.queue)), nil
}

func newModerationHandler(torrents map[int64]*model.Torrent, queue []model.Torrent) *ModerationHandler {
	tr := &stubTorrentRepo{torrents: torrents}
	mr := &stubModerationRepo{torrents: torrents, queue: queue}
	svc := service.NewTorrentService(nil, tr, newStubUserRepo(&model.User{ID: 7, Username: "mod"}),
		&stubStorage{}, service.TorrentServiceConfig{}, event.NewInMemoryBus(), nil)
	svc.SetModerationRepo(mr)
	return NewModerationHandler(svc)
}

func moderationReq(method, target string, userID *int64, perms model.Permissions) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	ctx := r.Context()
	if userID != nil {
		ctx = context.WithValue(ctx, middleware.UserIDKey, *userID)
	}
	ctx = context.WithValue(ctx, middleware.PermissionsKey, perms)
	r = r.WithContext(ctx)
	return withURLParam(r, "id", "1")
}

func int64p(v int64) *int64 { return &v }

func pendingTorrentMap() map[int64]*model.Torrent {
	return map[int64]*model.Torrent{
		1: {ID: 1, Name: "Pending", UploaderID: 5, CategoryID: 1, ModerationStatus: model.ModerationPending},
	}
}

func moderationBlock(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var resp struct {
		Torrent map[string]interface{} `json:"torrent"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v; body: %s", err, body)
	}
	m, ok := resp.Torrent["moderation"].(map[string]interface{})
	if !ok {
		t.Fatalf("no moderation block in response: %s", body)
	}
	return m
}

func TestModerationApprove(t *testing.T) {
	t.Run("staff approves", func(t *testing.T) {
		h := newModerationHandler(pendingTorrentMap(), nil)
		w := httptest.NewRecorder()
		h.HandleApprove(w, moderationReq(http.MethodPost, "/", int64p(7), model.Permissions{IsAdmin: true}))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		if got := moderationBlock(t, w.Body.Bytes())["status"]; got != model.ModerationApproved {
			t.Errorf("status = %v, want approved", got)
		}
	})

	t.Run("non-staff forbidden", func(t *testing.T) {
		h := newModerationHandler(pendingTorrentMap(), nil)
		w := httptest.NewRecorder()
		h.HandleApprove(w, moderationReq(http.MethodPost, "/", int64p(9), model.Permissions{}))
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthenticated", func(t *testing.T) {
		h := newModerationHandler(pendingTorrentMap(), nil)
		w := httptest.NewRecorder()
		h.HandleApprove(w, moderationReq(http.MethodPost, "/", nil, model.Permissions{}))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body: %s", w.Code, w.Body.String())
		}
	})
}

func TestModerationClaimAndReject(t *testing.T) {
	t.Run("claim assigns the actor", func(t *testing.T) {
		h := newModerationHandler(pendingTorrentMap(), nil)
		w := httptest.NewRecorder()
		h.HandleClaim(w, moderationReq(http.MethodPost, "/", int64p(7), model.Permissions{IsModerator: true}))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		if got := moderationBlock(t, w.Body.Bytes())["assigned_moderator_id"]; got != float64(7) {
			t.Errorf("assigned_moderator_id = %v, want 7", got)
		}
	})

	t.Run("reject sets status", func(t *testing.T) {
		h := newModerationHandler(pendingTorrentMap(), nil)
		w := httptest.NewRecorder()
		h.HandleReject(w, moderationReq(http.MethodPost, "/", int64p(7), model.Permissions{IsModerator: true}))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		if got := moderationBlock(t, w.Body.Bytes())["status"]; got != model.ModerationRejected {
			t.Errorf("status = %v, want rejected", got)
		}
	})
}

func TestModerationQueue(t *testing.T) {
	queue := []model.Torrent{
		{ID: 1, Name: "A", ModerationStatus: model.ModerationPending},
		{ID: 2, Name: "B", ModerationStatus: model.ModerationPending},
	}
	h := newModerationHandler(map[int64]*model.Torrent{}, queue)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/?assigned=unassigned", nil)
	r = r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, int64(7)))
	h.HandleQueue(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Torrents []map[string]interface{} `json:"torrents"`
		Total    int64                    `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 || len(resp.Torrents) != 2 {
		t.Errorf("queue = %d torrents (total %d), want 2", len(resp.Torrents), resp.Total)
	}
}
