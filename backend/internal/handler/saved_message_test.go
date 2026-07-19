package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

func newSavedMessageHandler(saved *stubSavedMessageRepo, messages *stubMessageRepo, users *stubUserRepo) *SavedMessageHandler {
	return NewSavedMessageHandler(service.NewSavedMessageService(saved, users, messages))
}

func newSavedMessageHandlerWithMessageSvc() (*SavedMessageHandler, *stubSavedMessageRepo) {
	saved := newStubSavedMessageRepo()
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())
	return h, saved
}

// --- save draft ---------------------------------------------------------------

func TestSaveDraftRequiresAuth(t *testing.T) {
	h, _ := newSavedMessageHandlerWithMessageSvc()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages/drafts", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.HandleSaveDraft(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestSaveDraftRejectsBadJSON(t *testing.T) {
	h, _ := newSavedMessageHandlerWithMessageSvc()

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages/drafts", strings.NewReader("{oops")), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSaveDraft(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for malformed body", w.Code)
	}
}

func TestSaveDraftCreatesDraftEvenWithoutSubjectOrBody(t *testing.T) {
	saved := newStubSavedMessageRepo()
	users := newStubUserRepo(&model.User{ID: 9, Username: "receiver"})
	h := newSavedMessageHandler(saved, newStubMessageRepo(), users)

	body := `{"receiver_id":9}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages/drafts", strings.NewReader(body)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSaveDraft(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	resp := decodeBody(t, w)
	draft, ok := resp["draft"].(map[string]interface{})
	if !ok {
		t.Fatal("response has no draft object")
	}
	if draft["receiver_id"].(float64) != 9 {
		t.Errorf("receiver_id = %v, want 9", draft["receiver_id"])
	}
	if draft["version"].(float64) != 1 {
		t.Errorf("version = %v, want 1 for a freshly created draft (BE-7.5)", draft["version"])
	}
}

func TestSaveDraftRejectsEntirelyEmpty(t *testing.T) {
	h, _ := newSavedMessageHandlerWithMessageSvc()

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages/drafts", strings.NewReader(`{}`)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSaveDraft(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a wholly empty draft", w.Code)
	}
}

// --- save template --------------------------------------------------------

func TestSaveTemplateCreatesTemplate(t *testing.T) {
	h, saved := newSavedMessageHandlerWithMessageSvc()

	body := `{"subject":"Welcome","body":"Thanks for joining!"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages/templates", strings.NewReader(body)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSaveTemplate(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(saved.byID) != 1 {
		t.Fatalf("stored %d saved messages, want 1", len(saved.byID))
	}
	resp := decodeBody(t, w)
	tmpl, ok := resp["template"].(map[string]interface{})
	if !ok {
		t.Fatal("response has no template object")
	}
	if _, present := tmpl["receiver_id"]; present {
		t.Error("template response must not carry a receiver_id")
	}
	if tmpl["version"].(float64) != 1 {
		t.Errorf("version = %v, want 1 for a freshly created template (BE-7.5)", tmpl["version"])
	}
}

func TestSaveTemplateRejectsEmptyBody(t *testing.T) {
	h, _ := newSavedMessageHandlerWithMessageSvc()

	body := `{"subject":"Welcome"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages/templates", strings.NewReader(body)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSaveTemplate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an empty template body", w.Code)
	}
}

func TestSaveTemplateRejectsReceiver(t *testing.T) {
	h, _ := newSavedMessageHandlerWithMessageSvc()

	body := `{"receiver_id":9,"body":"hi"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages/templates", strings.NewReader(body)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSaveTemplate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when a template names a receiver", w.Code)
	}
}

// --- update -----------------------------------------------------------------

func TestUpdateDraftRequiresAuth(t *testing.T) {
	h, _ := newSavedMessageHandlerWithMessageSvc()

	req := withURLParam(httptest.NewRequest(http.MethodPut, "/api/v1/messages/drafts/1", strings.NewReader(`{}`)), "id", "1")
	w := httptest.NewRecorder()
	h.HandleUpdateDraft(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestUpdateDraftRejectsInvalidID(t *testing.T) {
	h, _ := newSavedMessageHandlerWithMessageSvc()

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/messages/drafts/abc", strings.NewReader(`{"body":"x"}`)), 7, 1)
	req = withURLParam(req, "id", "abc")
	w := httptest.NewRecorder()
	h.HandleUpdateDraft(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a non-numeric id", w.Code)
	}
}

func TestUpdateDraftUpdatesOwnDraft(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 7, Kind: model.SavedMessageKindDraft, Body: "v1"}
	saved.nextID = 2
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/messages/drafts/1", strings.NewReader(`{"body":"v2"}`)), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleUpdateDraft(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if saved.byID[1].Body != "v2" {
		t.Errorf("body = %q, want v2", saved.byID[1].Body)
	}
}

func TestUpdateDraftRejectsNonOwner(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 99, Kind: model.SavedMessageKindDraft, Body: "v1"}
	saved.nextID = 2
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/messages/drafts/1", strings.NewReader(`{"body":"v2"}`)), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleUpdateDraft(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for someone else's draft", w.Code)
	}
}

func TestUpdateTemplateRejectsKindMismatch(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 7, Kind: model.SavedMessageKindDraft, Body: "v1"}
	saved.nextID = 2
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/messages/templates/1", strings.NewReader(`{"body":"v2"}`)), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleUpdateTemplate(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when updating a draft via the templates route", w.Code)
	}
}

// --- update: optimistic concurrency (BE-7.5) -------------------------------

// A PUT that carries a stale version (someone else's save already landed)
// must come back as a clean 409 with the current server-side state attached
// — not a silent overwrite, and not a generic 500.
func TestUpdateDraftRejectsStaleVersion(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 7, Kind: model.SavedMessageKindDraft, Body: "server has this", Version: 3}
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/messages/drafts/1", strings.NewReader(`{"body":"stale edit","version":2}`)), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleUpdateDraft(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
	resp := decodeBody(t, w)
	errBody, ok := resp["error"].(map[string]interface{})
	if !ok || errBody["code"] != "conflict" {
		t.Errorf("error.code = %v, want conflict", resp["error"])
	}
	draft, ok := resp["draft"].(map[string]interface{})
	if !ok {
		t.Fatal("409 response must carry the current draft so the client can react")
	}
	if draft["body"] != "server has this" {
		t.Errorf("returned body = %q, want the server's current content, not the stale request's", draft["body"])
	}
	if draft["version"].(float64) != 3 {
		t.Errorf("returned version = %v, want 3 (the current server-side version)", draft["version"])
	}
	// The stale write must not have applied.
	if saved.byID[1].Body != "server has this" {
		t.Errorf("stale update must not have overwritten the newer content, got body = %q", saved.byID[1].Body)
	}
}

// A PUT that carries the version the server currently has must succeed and
// bump the version, so a subsequent stale attempt (still holding the old
// version) is correctly rejected as a conflict rather than succeeding too.
func TestUpdateDraftWithCurrentVersionSucceedsAndBumpsVersion(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 7, Kind: model.SavedMessageKindDraft, Body: "v1", Version: 1}
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/messages/drafts/1", strings.NewReader(`{"body":"v2","version":1}`)), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleUpdateDraft(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	draft := decodeBody(t, w)["draft"].(map[string]interface{})
	if draft["version"].(float64) != 2 {
		t.Errorf("version = %v, want 2 after a successful update", draft["version"])
	}
	if saved.byID[1].Version != 2 || saved.byID[1].Body != "v2" {
		t.Errorf("stored draft = %+v, want body v2 version 2", saved.byID[1])
	}
}

// The 409 path is shared plumbing (handleUpdate/handleSavedMessageError) for
// both drafts and templates, but the response is built under a different key
// per kind ("template" here vs "draft" above) — worth its own test rather
// than assuming the draft coverage implies the template path works too.
func TestUpdateTemplateRejectsStaleVersion(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[5] = &model.SavedMessage{ID: 5, UserID: 7, Kind: model.SavedMessageKindTemplate, Body: "server has this", Version: 2}
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/messages/templates/5", strings.NewReader(`{"body":"stale edit","version":1}`)), 7, 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleUpdateTemplate(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
	resp := decodeBody(t, w)
	errBody, ok := resp["error"].(map[string]interface{})
	if !ok || errBody["code"] != "conflict" {
		t.Errorf("error.code = %v, want conflict", resp["error"])
	}
	tmpl, ok := resp["template"].(map[string]interface{})
	if !ok {
		t.Fatal("409 response must carry the current template under the \"template\" key")
	}
	if tmpl["body"] != "server has this" || tmpl["version"].(float64) != 2 {
		t.Errorf("returned template = %v, want the server's current content at version 2", tmpl)
	}
	if saved.byID[5].Body != "server has this" {
		t.Errorf("stale update must not have overwritten the newer content, got body = %q", saved.byID[5].Body)
	}
}

// getOwnedOfKind runs before the version comparison, so a wrong-kind request
// must be rejected as 404 regardless of what version it carries — a stale
// version on a kind mismatch must not somehow read as a 409.
func TestUpdateTemplateRejectsKindMismatchEvenWithStaleVersion(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 7, Kind: model.SavedMessageKindDraft, Body: "v1", Version: 5}
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/messages/templates/1", strings.NewReader(`{"body":"v2","version":1}`)), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleUpdateTemplate(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (kind mismatch must win over a stale version), body: %s", w.Code, w.Body.String())
	}
}

// --- list ---------------------------------------------------------------------

func TestListDraftsRequiresAuth(t *testing.T) {
	h, _ := newSavedMessageHandlerWithMessageSvc()

	w := httptest.NewRecorder()
	h.HandleListDrafts(w, httptest.NewRequest(http.MethodGet, "/api/v1/messages/drafts", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestListDraftsReturnsItemsAndPaginationDefaults(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.listResult = []model.SavedMessage{{ID: 1, UserID: 7, Kind: model.SavedMessageKindDraft, Body: "v1"}}
	saved.listTotal = 1
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/drafts", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListDrafts(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	items, ok := body["drafts"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("drafts = %v, want 1 item", body["drafts"])
	}
	if body["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", body["total"])
	}
	if body["page"].(float64) != 1 || body["per_page"].(float64) != 25 {
		t.Errorf("page/per_page = %v/%v, want defaults 1/25", body["page"], body["per_page"])
	}
}

func TestListTemplatesReturnsItems(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.listResult = []model.SavedMessage{{ID: 2, UserID: 7, Kind: model.SavedMessageKindTemplate, Body: "v1"}}
	saved.listTotal = 1
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/templates", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListTemplates(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	items, ok := decodeBody(t, w)["templates"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("templates = %v, want 1 item", items)
	}
}

func TestListDraftsSurfacesStoreFailure(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.listErr = errStub
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/drafts", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListDrafts(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- get ------------------------------------------------------------------

func TestGetDraftLoadsSubjectAndBody(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 7, Kind: model.SavedMessageKindDraft, Subject: "unfinished", Body: "still writing"}
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/drafts/1", nil), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleGetDraft(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	draft := decodeBody(t, w)["draft"].(map[string]interface{})
	if draft["subject"] != "unfinished" || draft["body"] != "still writing" {
		t.Errorf("subject/body = %v/%v, want unfinished/still writing", draft["subject"], draft["body"])
	}
}

func TestGetDraftRejectsInvalidID(t *testing.T) {
	h, _ := newSavedMessageHandlerWithMessageSvc()

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/drafts/abc", nil), 7, 1)
	req = withURLParam(req, "id", "abc")
	w := httptest.NewRecorder()
	h.HandleGetDraft(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a non-numeric id", w.Code)
	}
}

func TestGetTemplateRequiresAuth(t *testing.T) {
	h, _ := newSavedMessageHandlerWithMessageSvc()

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/messages/templates/1", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleGetTemplate(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestGetTemplateLoadsSubjectAndBody(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 7, Kind: model.SavedMessageKindTemplate, Subject: "Welcome", Body: "Thanks!"}
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/templates/1", nil), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleGetTemplate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	tmpl := decodeBody(t, w)["template"].(map[string]interface{})
	if tmpl["subject"] != "Welcome" || tmpl["body"] != "Thanks!" {
		t.Errorf("subject/body = %v/%v, want Welcome/Thanks!", tmpl["subject"], tmpl["body"])
	}
}

func TestGetDraftAsTemplateReturnsNotFound(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 7, Kind: model.SavedMessageKindDraft, Body: "v1"}
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/templates/1", nil), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleGetTemplate(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 — a draft must not be loadable through the templates route", w.Code)
	}
}

// --- delete -----------------------------------------------------------------

func TestDeleteDraftRequiresAuth(t *testing.T) {
	h, _ := newSavedMessageHandlerWithMessageSvc()

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/messages/drafts/1", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteDraft(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestDeleteDraftRemovesOwnDraft(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 7, Kind: model.SavedMessageKindDraft, Body: "v1"}
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/messages/drafts/1", nil), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteDraft(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	if len(saved.deleted) != 1 || saved.deleted[0] != 1 {
		t.Errorf("deleted = %v, want [1]", saved.deleted)
	}
}

func TestDeleteTemplateRejectsNonOwner(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 99, Kind: model.SavedMessageKindTemplate, Body: "v1"}
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/messages/templates/1", nil), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteTemplate(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for someone else's template", w.Code)
	}
	if len(saved.deleted) != 0 {
		t.Errorf("expected no deletion, got %v", saved.deleted)
	}
}

func TestDeleteDraftSurfacesRepositoryFailure(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 7, Kind: model.SavedMessageKindDraft, Body: "v1"}
	saved.deleteErr = errStub
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/messages/drafts/1", nil), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteDraft(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 when the store fails — it must not report success", w.Code)
	}
}

func TestSaveTemplateSurfacesRepositoryFailure(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.createErr = errStub
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	body := `{"body":"hi"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages/templates", strings.NewReader(body)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSaveTemplate(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 when the store fails", w.Code)
	}
}

func TestUpdateDraftSurfacesRepositoryFailure(t *testing.T) {
	saved := newStubSavedMessageRepo()
	saved.byID[1] = &model.SavedMessage{ID: 1, UserID: 7, Kind: model.SavedMessageKindDraft, Body: "v1"}
	saved.updateErr = errStub
	h := newSavedMessageHandler(saved, newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/messages/drafts/1", strings.NewReader(`{"body":"v2"}`)), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleUpdateDraft(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 when the store fails", w.Code)
	}
}

func TestDeleteDraftRejectsInvalidID(t *testing.T) {
	h, _ := newSavedMessageHandlerWithMessageSvc()

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/messages/drafts/0", nil), 7, 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleDeleteDraft(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
