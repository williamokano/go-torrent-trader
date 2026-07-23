package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

type stubModerationMessageRepo struct {
	msgs   []model.TorrentModerationMessage
	nextID int64
}

func (s *stubModerationMessageRepo) Create(_ context.Context, m *model.TorrentModerationMessage) error {
	s.nextID++
	m.ID = s.nextID
	s.msgs = append(s.msgs, *m)
	return nil
}

func (s *stubModerationMessageRepo) ListByTorrent(_ context.Context, torrentID int64) ([]model.TorrentModerationMessage, error) {
	var out []model.TorrentModerationMessage
	for _, m := range s.msgs {
		if m.TorrentID == torrentID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *stubModerationMessageRepo) CountByTorrent(_ context.Context, torrentID int64) (int, error) {
	n := 0
	for _, m := range s.msgs {
		if m.TorrentID == torrentID {
			n++
		}
	}
	return n, nil
}

func newMsgHandler(torrents map[int64]*model.Torrent, msgRepo *stubModerationMessageRepo) *ModerationHandler {
	tr := &stubTorrentRepo{torrents: torrents}
	mr := &stubModerationRepo{torrents: torrents}
	svc := service.NewTorrentService(nil, tr, newStubUserRepo(&model.User{ID: 7, Username: "mod"}),
		&stubStorage{}, service.TorrentServiceConfig{}, event.NewInMemoryBus(), nil)
	svc.SetModerationRepo(mr)
	svc.SetModerationMessageRepo(msgRepo)
	return NewModerationHandler(svc)
}

func msgReq(method, bodyJSON string, userID *int64, perms model.Permissions) *http.Request {
	var r *http.Request
	if bodyJSON == "" {
		r = httptest.NewRequest(method, "/", nil)
	} else {
		r = httptest.NewRequest(method, "/", strings.NewReader(bodyJSON))
	}
	ctx := r.Context()
	if userID != nil {
		ctx = context.WithValue(ctx, middleware.UserIDKey, *userID)
	}
	ctx = context.WithValue(ctx, middleware.PermissionsKey, perms)
	return withURLParam(r.WithContext(ctx), "id", "1")
}

func pendingOwnedBy(uploaderID int64) map[int64]*model.Torrent {
	return map[int64]*model.Torrent{
		1: {ID: 1, Name: "Pending", UploaderID: uploaderID, ModerationStatus: model.ModerationPending},
	}
}

func TestModerationMessagesPost(t *testing.T) {
	t.Run("uploader posts", func(t *testing.T) {
		h := newMsgHandler(pendingOwnedBy(5), &stubModerationMessageRepo{})
		w := httptest.NewRecorder()
		h.HandlePostMessage(w, msgReq(http.MethodPost, `{"body":"fix the nfo"}`, int64p(5), model.Permissions{}))
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Message map[string]interface{} `json:"message"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Message["body"] != "fix the nfo" {
			t.Errorf("body = %v, want 'fix the nfo'", resp.Message["body"])
		}
	})

	t.Run("staff posts", func(t *testing.T) {
		h := newMsgHandler(pendingOwnedBy(5), &stubModerationMessageRepo{})
		w := httptest.NewRecorder()
		h.HandlePostMessage(w, msgReq(http.MethodPost, `{"body":"looking"}`, int64p(9), model.Permissions{IsModerator: true}))
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("stranger forbidden", func(t *testing.T) {
		h := newMsgHandler(pendingOwnedBy(5), &stubModerationMessageRepo{})
		w := httptest.NewRecorder()
		h.HandlePostMessage(w, msgReq(http.MethodPost, `{"body":"hi"}`, int64p(99), model.Permissions{}))
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("empty body rejected", func(t *testing.T) {
		h := newMsgHandler(pendingOwnedBy(5), &stubModerationMessageRepo{})
		w := httptest.NewRecorder()
		h.HandlePostMessage(w, msgReq(http.MethodPost, `{"body":"  "}`, int64p(5), model.Permissions{}))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
		}
	})
}

func TestModerationMessagesList(t *testing.T) {
	msgRepo := &stubModerationMessageRepo{}
	_ = msgRepo.Create(context.Background(), &model.TorrentModerationMessage{TorrentID: 1, AuthorID: 5, Body: "hello"})
	h := newMsgHandler(pendingOwnedBy(5), msgRepo)

	w := httptest.NewRecorder()
	h.HandleListMessages(w, msgReq(http.MethodGet, "", int64p(5), model.Permissions{}))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Messages) != 1 || resp.Messages[0]["body"] != "hello" {
		t.Errorf("messages = %v, want one 'hello'", resp.Messages)
	}

	// A stranger gets 403.
	w = httptest.NewRecorder()
	h.HandleListMessages(w, msgReq(http.MethodGet, "", int64p(99), model.Permissions{}))
	if w.Code != http.StatusForbidden {
		t.Errorf("stranger list status = %d, want 403", w.Code)
	}
}
