package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

func (d *forumDeps) adminHandler() *ForumAdminHandler {
	return NewForumAdminHandler(service.NewForumService(
		nil, d.categories, d.forums, d.topics, d.posts, d.users, d.groups, event.NewInMemoryBus(),
	))
}

// --- categories -------------------------------------------------------------

func TestAdminListForumCategoriesReturnsItems(t *testing.T) {
	d := newForumDeps()
	d.categories.categories = []model.ForumCategory{{ID: 1, Name: "Main", SortOrder: 0}}
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/forum-categories", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListForumCategories(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	items, ok := decodeBody(t, w)["categories"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("categories = %v, want 1 item", items)
	}
	cat := items[0].(map[string]interface{})
	for _, key := range []string{"id", "name", "sort_order", "created_at"} {
		if _, present := cat[key]; !present {
			t.Errorf("category response is missing key %q", key)
		}
	}
}

func TestAdminListForumCategoriesSurfacesStoreFailure(t *testing.T) {
	d := newForumDeps()
	d.categories.listErr = errStub
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/forum-categories", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListForumCategories(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestAdminCreateForumCategoryRejectsBadJSON(t *testing.T) {
	d := newForumDeps()
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/forum-categories", strings.NewReader("{")), 1)
	w := httptest.NewRecorder()
	h.HandleCreateForumCategory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// A blank name is a validation error (422), distinct from malformed JSON (400).
func TestAdminCreateForumCategoryRejectsEmptyName(t *testing.T) {
	d := newForumDeps()
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/forum-categories",
		strings.NewReader(`{"name":"   "}`)), 1)
	w := httptest.NewRecorder()
	h.HandleCreateForumCategory(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", w.Code)
	}
	if len(d.categories.created) != 0 {
		t.Error("a nameless category was created")
	}
}

func TestAdminCreateForumCategoryStoresCategory(t *testing.T) {
	d := newForumDeps()
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/forum-categories",
		strings.NewReader(`{"name":"  Support  ","sort_order":3}`)), 1)
	w := httptest.NewRecorder()
	h.HandleCreateForumCategory(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(d.categories.created) != 1 {
		t.Fatalf("created %d categories, want 1", len(d.categories.created))
	}
	created := d.categories.created[0]
	if created.Name != "Support" || created.SortOrder != 3 {
		t.Errorf("category = %+v, want a trimmed name and sort order 3", created)
	}
	if _, ok := decodeBody(t, w)["category"]; !ok {
		t.Error("response is missing the category")
	}
}

func TestAdminCreateForumCategorySurfacesStoreFailure(t *testing.T) {
	d := newForumDeps()
	d.categories.createErr = errStub
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/forum-categories",
		strings.NewReader(`{"name":"Support"}`)), 1)
	w := httptest.NewRecorder()
	h.HandleCreateForumCategory(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestAdminUpdateForumCategoryValidatesInput(t *testing.T) {
	cases := []struct {
		name, id, body string
		want           int
	}{
		{"non-numeric id", "abc", `{"name":"x"}`, http.StatusBadRequest},
		{"malformed json", "1", `{`, http.StatusBadRequest},
		{"unknown category", "99", `{"name":"x"}`, http.StatusNotFound},
		{"blank name", "1", `{"name":"  "}`, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newForumDeps()
			h := d.adminHandler()

			req := adminAuthed(httptest.NewRequest(http.MethodPut,
				"/api/v1/admin/forum-categories/"+tc.id, strings.NewReader(tc.body)), 1)
			req = withURLParam(req, "id", tc.id)
			w := httptest.NewRecorder()
			h.HandleUpdateForumCategory(w, req)

			if w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestAdminUpdateForumCategoryWritesNewFields(t *testing.T) {
	d := newForumDeps()
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/forum-categories/1",
		strings.NewReader(`{"name":"Renamed","sort_order":9}`)), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleUpdateForumCategory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(d.categories.updated) != 1 {
		t.Fatalf("updated %d categories, want 1", len(d.categories.updated))
	}
	if d.categories.updated[0].Name != "Renamed" || d.categories.updated[0].SortOrder != 9 {
		t.Errorf("updated = %+v, want the new name and sort order", d.categories.updated[0])
	}
}

func TestAdminDeleteForumCategoryValidatesID(t *testing.T) {
	d := newForumDeps()
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forum-categories/0", nil), 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleDeleteForumCategory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAdminDeleteForumCategoryReturns404WhenMissing(t *testing.T) {
	d := newForumDeps()
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forum-categories/99", nil), 1)
	req = withURLParam(req, "id", "99")
	w := httptest.NewRecorder()
	h.HandleDeleteForumCategory(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// A category that still holds forums must not be deleted out from under them.
func TestAdminDeleteForumCategoryConflictsWhenItStillHasForums(t *testing.T) {
	d := newForumDeps()
	d.categories.forumCount = 2
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forum-categories/1", nil), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteForumCategory(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", w.Code)
	}
	if len(d.categories.deleted) != 0 {
		t.Error("the non-empty category was deleted anyway")
	}
}

func TestAdminDeleteForumCategoryRemovesEmptyCategory(t *testing.T) {
	d := newForumDeps()
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forum-categories/1", nil), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteForumCategory(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	if len(d.categories.deleted) != 1 || d.categories.deleted[0] != 1 {
		t.Errorf("deleted = %v, want [1]", d.categories.deleted)
	}
}

// --- forums -----------------------------------------------------------------

func TestAdminListForumsReturnsEveryForum(t *testing.T) {
	d := newForumDeps()
	d.forums.list = []model.Forum{
		{ID: 1, Name: "General", MinGroupLevel: 0},
		{ID: 2, Name: "Staff only", MinGroupLevel: 90},
	}
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/forums", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListForums(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	items, ok := decodeBody(t, w)["forums"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("forums = %v, want 2 — the admin list is not level-filtered", items)
	}
}

func TestAdminListForumsSurfacesStoreFailure(t *testing.T) {
	d := newForumDeps()
	d.forums.listErr = errStub
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/forums", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListForums(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestAdminCreateForumValidatesInput(t *testing.T) {
	cases := []struct {
		name, body string
		want       int
	}{
		{"malformed json", `{`, http.StatusBadRequest},
		{"blank name", `{"name":"  ","category_id":1}`, http.StatusUnprocessableEntity},
		{"missing category", `{"name":"News"}`, http.StatusUnprocessableEntity},
		{"negative min_group_level", `{"name":"News","category_id":1,"min_group_level":-1}`, http.StatusUnprocessableEntity},
		{"negative min_post_level", `{"name":"News","category_id":1,"min_post_level":-1}`, http.StatusUnprocessableEntity},
		{"unknown category", `{"name":"News","category_id":99}`, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newForumDeps()
			h := d.adminHandler()

			req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/forums", strings.NewReader(tc.body)), 1)
			w := httptest.NewRecorder()
			h.HandleCreateForum(w, req)

			if w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
			if len(d.forums.created) != 0 {
				t.Error("a forum was created despite the invalid request")
			}
		})
	}
}

func TestAdminCreateForumStoresForum(t *testing.T) {
	d := newForumDeps()
	h := d.adminHandler()

	body := `{"name":"  Announcements  ","description":" news ","category_id":1,"sort_order":2,"min_group_level":10,"min_post_level":90}`
	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/forums", strings.NewReader(body)), 1)
	w := httptest.NewRecorder()
	h.HandleCreateForum(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(d.forums.created) != 1 {
		t.Fatalf("created %d forums, want 1", len(d.forums.created))
	}
	created := d.forums.created[0]
	if created.Name != "Announcements" || created.Description != "news" {
		t.Errorf("forum = %+v, want trimmed name and description", created)
	}
	if created.MinGroupLevel != 10 || created.MinPostLevel != 90 {
		t.Errorf("levels = %d/%d, want 10/90 — a read-only announcement forum", created.MinGroupLevel, created.MinPostLevel)
	}
	forum := decodeBody(t, w)["forum"].(map[string]interface{})
	for _, key := range []string{"id", "category_id", "name", "description", "sort_order", "min_group_level", "min_post_level"} {
		if _, present := forum[key]; !present {
			t.Errorf("forum response is missing key %q", key)
		}
	}
}

func TestAdminCreateForumSurfacesStoreFailure(t *testing.T) {
	d := newForumDeps()
	d.forums.createErr = errStub
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/forums",
		strings.NewReader(`{"name":"News","category_id":1}`)), 1)
	w := httptest.NewRecorder()
	h.HandleCreateForum(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestAdminUpdateForumValidatesInput(t *testing.T) {
	cases := []struct {
		name, id, body string
		want           int
	}{
		{"non-numeric id", "abc", `{"name":"x","category_id":1}`, http.StatusBadRequest},
		{"malformed json", "1", `{`, http.StatusBadRequest},
		{"unknown forum", "99", `{"name":"x","category_id":1}`, http.StatusNotFound},
		{"blank name", "1", `{"name":"  ","category_id":1}`, http.StatusUnprocessableEntity},
		{"unknown category", "1", `{"name":"x","category_id":99}`, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newForumDeps()
			h := d.adminHandler()

			req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/forums/"+tc.id,
				strings.NewReader(tc.body)), 1)
			req = withURLParam(req, "id", tc.id)
			w := httptest.NewRecorder()
			h.HandleUpdateForum(w, req)

			if w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestAdminUpdateForumWritesNewFields(t *testing.T) {
	d := newForumDeps()
	h := d.adminHandler()

	body := `{"name":"Renamed","description":"d","category_id":1,"sort_order":4,"min_group_level":50,"min_post_level":60}`
	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/forums/1", strings.NewReader(body)), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleUpdateForum(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(d.forums.updated) != 1 {
		t.Fatalf("updated %d forums, want 1", len(d.forums.updated))
	}
	updated := d.forums.updated[0]
	if updated.Name != "Renamed" || updated.MinGroupLevel != 50 || updated.MinPostLevel != 60 {
		t.Errorf("updated = %+v, want the new name and levels", updated)
	}
}

func TestAdminDeleteForumValidatesID(t *testing.T) {
	d := newForumDeps()
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forums/x", nil), 1)
	req = withURLParam(req, "id", "x")
	w := httptest.NewRecorder()
	h.HandleDeleteForum(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAdminDeleteForumReturns404WhenMissing(t *testing.T) {
	d := newForumDeps()
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forums/99", nil), 1)
	req = withURLParam(req, "id", "99")
	w := httptest.NewRecorder()
	h.HandleDeleteForum(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// Deleting a forum that still has topics would orphan them.
func TestAdminDeleteForumConflictsWhenItStillHasTopics(t *testing.T) {
	d := newForumDeps()
	d.forums.topicCount = 5
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forums/1", nil), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteForum(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", w.Code)
	}
	if len(d.forums.deleted) != 0 {
		t.Error("the non-empty forum was deleted anyway")
	}
}

func TestAdminDeleteForumRemovesEmptyForum(t *testing.T) {
	d := newForumDeps()
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forums/1", nil), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteForum(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	if len(d.forums.deleted) != 1 || d.forums.deleted[0] != 1 {
		t.Errorf("deleted = %v, want [1]", d.forums.deleted)
	}
}

func TestAdminDeleteForumSurfacesStoreFailure(t *testing.T) {
	d := newForumDeps()
	d.forums.deleteErr = errStub
	h := d.adminHandler()

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forums/1", nil), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteForum(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
