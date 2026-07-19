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
