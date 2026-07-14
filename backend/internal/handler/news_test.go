package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- news repository stub ---------------------------------------------------

type stubNewsRepo struct {
	byID      map[int64]*model.NewsArticle
	published map[int64]*model.NewsArticle
	list      []model.NewsArticle
	total     int64
	created   []*model.NewsArticle
	updated   []*model.NewsArticle
	deleted   []int64
	lastOpts  repository.ListNewsOptions

	createErr error
	updateErr error
	deleteErr error
	listErr   error

	nextID int64
}

func newStubNewsRepo() *stubNewsRepo {
	return &stubNewsRepo{
		byID:      map[int64]*model.NewsArticle{},
		published: map[int64]*model.NewsArticle{},
		nextID:    1,
	}
}

func (s *stubNewsRepo) Create(_ context.Context, article *model.NewsArticle) error {
	if s.createErr != nil {
		return s.createErr
	}
	article.ID = s.nextID
	s.nextID++
	s.created = append(s.created, article)
	stored := *article
	s.byID[article.ID] = &stored
	return nil
}

func (s *stubNewsRepo) GetByID(_ context.Context, id int64) (*model.NewsArticle, error) {
	a, ok := s.byID[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return a, nil
}

func (s *stubNewsRepo) GetPublishedByID(_ context.Context, id int64) (*model.NewsArticle, error) {
	a, ok := s.published[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return a, nil
}

func (s *stubNewsRepo) Update(_ context.Context, article *model.NewsArticle) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updated = append(s.updated, article)
	s.byID[article.ID] = article
	return nil
}

func (s *stubNewsRepo) Delete(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, id)
	return nil
}

func (s *stubNewsRepo) List(_ context.Context, opts repository.ListNewsOptions) ([]model.NewsArticle, int64, error) {
	s.lastOpts = opts
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.list, s.total, nil
}

func (s *stubNewsRepo) ListPublished(_ context.Context, _, _ int) ([]model.NewsArticle, int64, error) {
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.list, s.total, nil
}

func newNewsHandler(news *stubNewsRepo) *NewsHandler {
	users := newStubUserRepo(&model.User{ID: 1, Username: "admin"})
	return NewNewsHandler(service.NewNewsService(news, users, event.NewInMemoryBus()))
}

// --- admin create -----------------------------------------------------------

func TestCreateNewsRequiresAuth(t *testing.T) {
	h := newNewsHandler(newStubNewsRepo())

	w := httptest.NewRecorder()
	h.HandleAdminCreateNews(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/news", strings.NewReader(`{}`)))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestCreateNewsRejectsBadJSON(t *testing.T) {
	h := newNewsHandler(newStubNewsRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/news", strings.NewReader("{")), 1)
	w := httptest.NewRecorder()
	h.HandleAdminCreateNews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a malformed body", w.Code)
	}
}

func TestCreateNewsRejectsEmptyTitleAndBody(t *testing.T) {
	for _, body := range []string{`{"title":"  ","body":"text"}`, `{"title":"t","body":"   "}`} {
		h := newNewsHandler(newStubNewsRepo())

		req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/news", strings.NewReader(body)), 1)
		w := httptest.NewRecorder()
		h.HandleAdminCreateNews(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
	}
}

func TestCreateNewsStoresArticleWithAuthor(t *testing.T) {
	news := newStubNewsRepo()
	h := newNewsHandler(news)

	body := `{"title":"Tracker news","body":"We are live","published":true}`
	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/news", strings.NewReader(body)), 1)
	w := httptest.NewRecorder()
	h.HandleAdminCreateNews(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(news.created) != 1 {
		t.Fatalf("created %d articles, want 1", len(news.created))
	}
	created := news.created[0]
	if created.AuthorID == nil || *created.AuthorID != 1 {
		t.Errorf("author_id = %v, want the authenticated actor 1", created.AuthorID)
	}
	if !created.Published {
		t.Error("published = false, want true")
	}
	if _, ok := decodeBody(t, w)["article"]; !ok {
		t.Error("response is missing the \"article\" key")
	}
}

func TestCreateNewsSurfacesStoreFailure(t *testing.T) {
	news := newStubNewsRepo()
	news.createErr = errStub
	h := newNewsHandler(news)

	body := `{"title":"t","body":"b"}`
	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/news", strings.NewReader(body)), 1)
	w := httptest.NewRecorder()
	h.HandleAdminCreateNews(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- admin list -------------------------------------------------------------

func TestAdminListNewsAppliesPaginationDefaults(t *testing.T) {
	news := newStubNewsRepo()
	news.list = []model.NewsArticle{{ID: 1, Title: "t"}}
	news.total = 1
	h := newNewsHandler(news)

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/news", nil), 1)
	w := httptest.NewRecorder()
	h.HandleAdminListNews(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if news.lastOpts.Page != 1 || news.lastOpts.PerPage != 25 {
		t.Errorf("opts = %+v, want the defaults page=1 per_page=25", news.lastOpts)
	}
	body := decodeBody(t, w)
	if items, ok := body["articles"].([]interface{}); !ok || len(items) != 1 {
		t.Errorf("articles = %v, want 1 item", body["articles"])
	}
	if body["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", body["total"])
	}
}

// per_page above the 100 cap must fall back to the default rather than let a
// caller pull the whole table in one request.
func TestAdminListNewsClampsOversizedPerPage(t *testing.T) {
	news := newStubNewsRepo()
	h := newNewsHandler(news)

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/news?page=2&per_page=5000", nil), 1)
	w := httptest.NewRecorder()
	h.HandleAdminListNews(w, req)

	if news.lastOpts.Page != 2 {
		t.Errorf("page = %d, want 2", news.lastOpts.Page)
	}
	if news.lastOpts.PerPage != 25 {
		t.Errorf("per_page = %d, want the default 25 — an out-of-range value must not be honoured", news.lastOpts.PerPage)
	}
}

func TestAdminListNewsReturnsEmptyArrayNotNull(t *testing.T) {
	h := newNewsHandler(newStubNewsRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/news", nil), 1)
	w := httptest.NewRecorder()
	h.HandleAdminListNews(w, req)

	if items, ok := decodeBody(t, w)["articles"].([]interface{}); !ok || len(items) != 0 {
		t.Errorf("articles = %v, want an empty array", items)
	}
}

func TestAdminListNewsSurfacesStoreFailure(t *testing.T) {
	news := newStubNewsRepo()
	news.listErr = errStub
	h := newNewsHandler(news)

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/news", nil), 1)
	w := httptest.NewRecorder()
	h.HandleAdminListNews(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- admin update -----------------------------------------------------------

func TestUpdateNewsRequiresAuth(t *testing.T) {
	h := newNewsHandler(newStubNewsRepo())

	req := withURLParam(httptest.NewRequest(http.MethodPut, "/api/v1/admin/news/1", strings.NewReader(`{}`)), "id", "1")
	w := httptest.NewRecorder()
	h.HandleAdminUpdateNews(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestUpdateNewsRejectsInvalidID(t *testing.T) {
	h := newNewsHandler(newStubNewsRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/news/0", strings.NewReader(`{}`)), 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleAdminUpdateNews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestUpdateNewsRejectsBadJSON(t *testing.T) {
	h := newNewsHandler(newStubNewsRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/news/1", strings.NewReader("{")), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleAdminUpdateNews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestUpdateNewsReturns404WhenMissing(t *testing.T) {
	h := newNewsHandler(newStubNewsRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/news/9", strings.NewReader(`{"title":"t","body":"b"}`)), 1)
	req = withURLParam(req, "id", "9")
	w := httptest.NewRecorder()
	h.HandleAdminUpdateNews(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestUpdateNewsRejectsEmptyTitle(t *testing.T) {
	news := newStubNewsRepo()
	news.byID[1] = &model.NewsArticle{ID: 1, Title: "old", Body: "old"}
	h := newNewsHandler(news)

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/news/1", strings.NewReader(`{"title":"","body":"b"}`)), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleAdminUpdateNews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a blank title", w.Code)
	}
}

func TestUpdateNewsWritesNewFields(t *testing.T) {
	news := newStubNewsRepo()
	news.byID[1] = &model.NewsArticle{ID: 1, Title: "old", Body: "old", Published: false}
	h := newNewsHandler(news)

	body := `{"title":"new title","body":"new body","published":true}`
	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/news/1", strings.NewReader(body)), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleAdminUpdateNews(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(news.updated) != 1 {
		t.Fatalf("updated %d articles, want 1", len(news.updated))
	}
	updated := news.updated[0]
	if updated.Title != "new title" || updated.Body != "new body" || !updated.Published {
		t.Errorf("updated = %+v, want the new title/body and published=true", updated)
	}
}

func TestUpdateNewsSurfacesStoreFailure(t *testing.T) {
	news := newStubNewsRepo()
	news.byID[1] = &model.NewsArticle{ID: 1}
	news.updateErr = errStub
	h := newNewsHandler(news)

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/news/1", strings.NewReader(`{"title":"t","body":"b"}`)), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleAdminUpdateNews(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- admin delete -----------------------------------------------------------

func TestDeleteNewsRejectsInvalidID(t *testing.T) {
	h := newNewsHandler(newStubNewsRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/news/abc", nil), 1)
	req = withURLParam(req, "id", "abc")
	w := httptest.NewRecorder()
	h.HandleAdminDeleteNews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDeleteNewsRemovesArticle(t *testing.T) {
	news := newStubNewsRepo()
	h := newNewsHandler(news)

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/news/3", nil), 1)
	req = withURLParam(req, "id", "3")
	w := httptest.NewRecorder()
	h.HandleAdminDeleteNews(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if len(news.deleted) != 1 || news.deleted[0] != 3 {
		t.Errorf("deleted = %v, want [3]", news.deleted)
	}
}

func TestDeleteNewsReturns404WhenMissing(t *testing.T) {
	news := newStubNewsRepo()
	news.deleteErr = sql.ErrNoRows
	h := newNewsHandler(news)

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/news/3", nil), 1)
	req = withURLParam(req, "id", "3")
	w := httptest.NewRecorder()
	h.HandleAdminDeleteNews(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestDeleteNewsSurfacesStoreFailure(t *testing.T) {
	news := newStubNewsRepo()
	news.deleteErr = errStub
	h := newNewsHandler(news)

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/news/3", nil), 1)
	req = withURLParam(req, "id", "3")
	w := httptest.NewRecorder()
	h.HandleAdminDeleteNews(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- public endpoints -------------------------------------------------------

func TestListPublishedNewsIsPublic(t *testing.T) {
	news := newStubNewsRepo()
	news.list = []model.NewsArticle{{ID: 1, Title: "t", Published: true}}
	news.total = 1
	h := newNewsHandler(news)

	// No auth context at all — this endpoint is public.
	w := httptest.NewRecorder()
	h.HandleListPublishedNews(w, httptest.NewRequest(http.MethodGet, "/api/v1/news", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	if items, ok := body["articles"].([]interface{}); !ok || len(items) != 1 {
		t.Errorf("articles = %v, want 1 item", body["articles"])
	}
	if body["page"].(float64) != 1 || body["per_page"].(float64) != 25 {
		t.Errorf("page/per_page = %v/%v, want the defaults 1/25", body["page"], body["per_page"])
	}
}

func TestListPublishedNewsSurfacesStoreFailure(t *testing.T) {
	news := newStubNewsRepo()
	news.listErr = errStub
	h := newNewsHandler(news)

	w := httptest.NewRecorder()
	h.HandleListPublishedNews(w, httptest.NewRequest(http.MethodGet, "/api/v1/news", nil))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetPublishedNewsRejectsInvalidID(t *testing.T) {
	h := newNewsHandler(newStubNewsRepo())

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/news/abc", nil), "id", "abc")
	w := httptest.NewRecorder()
	h.HandleGetPublishedNews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetPublishedNewsReturnsArticle(t *testing.T) {
	news := newStubNewsRepo()
	news.published[1] = &model.NewsArticle{ID: 1, Title: "t", Body: "b", Published: true}
	h := newNewsHandler(news)

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/news/1", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleGetPublishedNews(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	article, ok := decodeBody(t, w)["article"].(map[string]interface{})
	if !ok {
		t.Fatal("response has no article object")
	}
	if article["title"] != "t" {
		t.Errorf("title = %v, want \"t\"", article["title"])
	}
}

// An unpublished draft must not be reachable through the public endpoint.
func TestGetPublishedNewsHidesDrafts(t *testing.T) {
	news := newStubNewsRepo()
	news.byID[1] = &model.NewsArticle{ID: 1, Title: "draft", Published: false} // not in `published`
	h := newNewsHandler(news)

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/news/1", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleGetPublishedNews(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 — unpublished drafts must not leak through the public endpoint", w.Code)
	}
}
