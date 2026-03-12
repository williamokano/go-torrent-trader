package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// mockHandlerForumPostRepo is a minimal mock of ForumPostRepository for handler tests.
type mockHandlerForumPostRepo struct{}

func (m *mockHandlerForumPostRepo) GetByID(_ context.Context, _ int64) (*model.ForumPost, error) { return nil, nil }
func (m *mockHandlerForumPostRepo) ListByTopic(_ context.Context, _ int64, _, _ int) ([]model.ForumPost, int64, error) { return nil, 0, nil }
func (m *mockHandlerForumPostRepo) Create(_ context.Context, post *model.ForumPost) error { post.ID = 1; post.CreatedAt = time.Now(); return nil }
func (m *mockHandlerForumPostRepo) Update(_ context.Context, _ *model.ForumPost) error { return nil }
func (m *mockHandlerForumPostRepo) Delete(_ context.Context, _ int64) error { return nil }
func (m *mockHandlerForumPostRepo) CountByUser(_ context.Context, _ int64) (int, error) { return 0, nil }
func (m *mockHandlerForumPostRepo) Search(_ context.Context, _ string, _ *int64, _ int, _, _ int) ([]model.ForumSearchResult, int64, error) { return nil, 0, nil }
func (m *mockHandlerForumPostRepo) GetFirstPostIDByTopic(_ context.Context, _ int64) (int64, error) { return 0, nil }

func withForumAuth(r *http.Request, userID int64, perms model.Permissions) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserIDKey, userID)
	ctx = context.WithValue(ctx, middleware.PermissionsKey, perms)
	return r.WithContext(ctx)
}

func TestHandleListForums(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("GET", "/api/v1/forums", nil)
	req = withForumAuth(req, 1, model.Permissions{Level: 5})
	w := httptest.NewRecorder()
	defer func() { _ = recover() }()
	h.HandleListForums(w, req)
}

func TestHandleCreateTopic_BadJSON(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("POST", "/api/v1/forums/1/topics", strings.NewReader("not json"))
	req = withForumAuth(req, 1, model.Permissions{Level: 5})
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleCreateTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { t.Fatalf("decode: %v", err) }
	if resp["error"].(map[string]interface{})["code"] != "bad_request" { t.Error("expected bad_request") }
}

func TestHandleCreateTopic_InvalidID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("POST", "/api/v1/forums/abc/topics", strings.NewReader(`{"title":"t","body":"b"}`))
	req = withForumAuth(req, 1, model.Permissions{Level: 5})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleCreateTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleCreatePost_BadJSON(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("POST", "/api/v1/forums/topics/1/posts", strings.NewReader("{bad"))
	req = withForumAuth(req, 1, model.Permissions{Level: 5})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleCreatePost(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleGetTopic_InvalidID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("GET", "/api/v1/forums/topics/xyz", nil)
	req = withForumAuth(req, 1, model.Permissions{Level: 5})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "xyz")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleGetTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleGetForum_InvalidID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("GET", "/api/v1/forums/0", nil)
	req = withForumAuth(req, 1, model.Permissions{Level: 5})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "0")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleGetForum(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleCreateTopic_Unauthorized(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("POST", "/api/v1/forums/1/topics", strings.NewReader(`{"title":"t","body":"b"}`))
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleCreateTopic(w, req)
	if w.Code != http.StatusUnauthorized { t.Errorf("expected 401, got %d", w.Code) }
}

func TestHandleCreatePost_Unauthorized(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("POST", "/api/v1/forums/topics/1/posts", strings.NewReader(`{"body":"b"}`))
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleCreatePost(w, req)
	if w.Code != http.StatusUnauthorized { t.Errorf("expected 401, got %d", w.Code) }
}

func TestHandleSearchForum_EmptyQuery(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("GET", "/api/v1/forums/search", nil)
	req = withForumAuth(req, 1, model.Permissions{Level: 5})
	w := httptest.NewRecorder()
	h.HandleSearchForum(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleSearchForum_InvalidForumID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("GET", "/api/v1/forums/search?q=hello&forum_id=abc", nil)
	req = withForumAuth(req, 1, model.Permissions{Level: 5})
	w := httptest.NewRecorder()
	h.HandleSearchForum(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleSearchForum_Success(t *testing.T) {
	forumSvc := service.NewForumService(nil, nil, nil, nil, &mockHandlerForumPostRepo{}, nil, nil, nil)
	h := NewForumHandler(forumSvc)
	req := httptest.NewRequest("GET", "/api/v1/forums/search?q=hello&page=1&per_page=10", nil)
	req = withForumAuth(req, 1, model.Permissions{Level: 5})
	w := httptest.NewRecorder()
	h.HandleSearchForum(w, req)
	if w.Code != http.StatusOK { t.Errorf("expected 200, got %d", w.Code) }
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { t.Fatalf("decode: %v", err) }
	if resp["results"] == nil { t.Error("expected results key") }
	if resp["total"] == nil { t.Error("expected total key") }
}

func TestHandleForumError(t *testing.T) {
	tests := []struct{ err error; expected int }{
		{service.ErrForumNotFound, http.StatusNotFound},
		{service.ErrTopicNotFound, http.StatusNotFound},
		{service.ErrPostNotFound, http.StatusNotFound},
		{service.ErrTopicLocked, http.StatusForbidden},
		{service.ErrForumAccessDenied, http.StatusForbidden},
		{service.ErrModHierarchyDenied, http.StatusForbidden},
		{service.ErrPostEditDenied, http.StatusForbidden},
		{service.ErrPostDeleteDenied, http.StatusForbidden},
		{service.ErrCannotDeleteFirstPost, http.StatusBadRequest},
		{service.ErrInvalidPost, http.StatusBadRequest},
		{service.ErrTopicDeleteDenied, http.StatusForbidden},
		{service.ErrSameForum, http.StatusBadRequest},
		{service.ErrInvalidReply, http.StatusBadRequest},
	}
	for _, tc := range tests {
		w := httptest.NewRecorder()
		handleForumError(w, tc.err)
		if w.Code != tc.expected { t.Errorf("for %v: expected %d, got %d", tc.err, tc.expected, w.Code) }
	}
}

func TestHandleEditPost_Unauthorized(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("PUT", "/api/v1/forums/posts/1", strings.NewReader(`{"body":"b"}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleEditPost(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleEditPost_InvalidID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("PUT", "/api/v1/forums/posts/abc", strings.NewReader(`{"body":"b"}`))
	req = withForumAuth(req, 1, model.Permissions{Level: 5})
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleEditPost(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleEditPost_BadJSON(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("PUT", "/api/v1/forums/posts/1", strings.NewReader("{bad"))
	req = withForumAuth(req, 1, model.Permissions{Level: 5})
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleEditPost(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDeletePost_Unauthorized(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("DELETE", "/api/v1/forums/posts/1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleDeletePost(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleDeletePost_InvalidID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("DELETE", "/api/v1/forums/posts/abc", nil)
	req = withForumAuth(req, 1, model.Permissions{Level: 5})
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleDeletePost(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleLockTopic_InvalidID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("POST", "/api/v1/forums/topics/abc/lock", nil)
	req = withForumAuth(req, 1, model.Permissions{Level: 100, IsAdmin: true})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleLockTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleUnlockTopic_InvalidID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("POST", "/api/v1/forums/topics/0/unlock", nil)
	req = withForumAuth(req, 1, model.Permissions{Level: 100, IsAdmin: true})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "0")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleUnlockTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandlePinTopic_InvalidID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("POST", "/api/v1/forums/topics/xyz/pin", nil)
	req = withForumAuth(req, 1, model.Permissions{Level: 100, IsAdmin: true})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "xyz")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandlePinTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleUnpinTopic_InvalidID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("POST", "/api/v1/forums/topics/-1/unpin", nil)
	req = withForumAuth(req, 1, model.Permissions{Level: 100, IsAdmin: true})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleUnpinTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleRenameTopic_InvalidID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("PUT", "/api/v1/forums/topics/abc/title", strings.NewReader(`{"title":"new"}`))
	req = withForumAuth(req, 1, model.Permissions{Level: 100, IsAdmin: true})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleRenameTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleRenameTopic_BadJSON(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("PUT", "/api/v1/forums/topics/1/title", strings.NewReader("{bad"))
	req = withForumAuth(req, 1, model.Permissions{Level: 100, IsAdmin: true})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleRenameTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleMoveTopic_InvalidID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("POST", "/api/v1/forums/topics/abc/move", strings.NewReader(`{"forum_id":2}`))
	req = withForumAuth(req, 1, model.Permissions{Level: 100, IsAdmin: true})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleMoveTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleMoveTopic_BadJSON(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("POST", "/api/v1/forums/topics/1/move", strings.NewReader("{bad"))
	req = withForumAuth(req, 1, model.Permissions{Level: 100, IsAdmin: true})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleMoveTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleMoveTopic_InvalidForumID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("POST", "/api/v1/forums/topics/1/move", strings.NewReader(`{"forum_id":0}`))
	req = withForumAuth(req, 1, model.Permissions{Level: 100, IsAdmin: true})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleMoveTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}

func TestHandleDeleteTopic_InvalidID(t *testing.T) {
	h := NewForumHandler(nil)
	req := httptest.NewRequest("DELETE", "/api/v1/forums/topics/abc", nil)
	req = withForumAuth(req, 1, model.Permissions{Level: 100, IsAdmin: true})
	rctx := chi.NewRouteContext(); rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleDeleteTopic(w, req)
	if w.Code != http.StatusBadRequest { t.Errorf("expected 400, got %d", w.Code) }
}
