package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// SavedMessageHandler handles HTTP endpoints for PM drafts and templates
// (BE-7.2). Drafts and templates are exposed as distinct REST resources
// (/messages/drafts, /messages/templates) since they're a distinct concept to
// callers, but both are backed by the same model.SavedMessage/kind
// discriminator, so each pair of handler methods below is a thin wrapper
// around one shared implementation parameterized by kind.
type SavedMessageHandler struct {
	svc *service.SavedMessageService
}

// NewSavedMessageHandler creates a new SavedMessageHandler.
func NewSavedMessageHandler(svc *service.SavedMessageService) *SavedMessageHandler {
	return &SavedMessageHandler{svc: svc}
}

type saveMessageBody struct {
	ReceiverID *int64 `json:"receiver_id,omitempty"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	ParentID   *int64 `json:"parent_id,omitempty"`
	// Version is only meaningful on PUT (handleUpdate) — it must be the
	// version the client last saw (from a prior save/get/list response), and
	// is compared against the stored row to detect a lost-update race. It's
	// ignored on POST (handleSave): a brand-new row has nothing to conflict
	// with yet.
	Version int64 `json:"version"`
}

// HandleSaveDraft handles POST /api/v1/messages/drafts.
func (h *SavedMessageHandler) HandleSaveDraft(w http.ResponseWriter, r *http.Request) {
	h.handleSave(w, r, model.SavedMessageKindDraft)
}

// HandleSaveTemplate handles POST /api/v1/messages/templates.
func (h *SavedMessageHandler) HandleSaveTemplate(w http.ResponseWriter, r *http.Request) {
	h.handleSave(w, r, model.SavedMessageKindTemplate)
}

// HandleUpdateDraft handles PUT /api/v1/messages/drafts/{id}.
func (h *SavedMessageHandler) HandleUpdateDraft(w http.ResponseWriter, r *http.Request) {
	h.handleUpdate(w, r, model.SavedMessageKindDraft)
}

// HandleUpdateTemplate handles PUT /api/v1/messages/templates/{id}.
func (h *SavedMessageHandler) HandleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	h.handleUpdate(w, r, model.SavedMessageKindTemplate)
}

// HandleListDrafts handles GET /api/v1/messages/drafts.
func (h *SavedMessageHandler) HandleListDrafts(w http.ResponseWriter, r *http.Request) {
	h.handleList(w, r, model.SavedMessageKindDraft)
}

// HandleListTemplates handles GET /api/v1/messages/templates.
func (h *SavedMessageHandler) HandleListTemplates(w http.ResponseWriter, r *http.Request) {
	h.handleList(w, r, model.SavedMessageKindTemplate)
}

// HandleGetDraft handles GET /api/v1/messages/drafts/{id}.
func (h *SavedMessageHandler) HandleGetDraft(w http.ResponseWriter, r *http.Request) {
	h.handleGet(w, r, model.SavedMessageKindDraft)
}

// HandleGetTemplate handles GET /api/v1/messages/templates/{id} — used by the
// frontend to load a template's subject/body into the compose form.
func (h *SavedMessageHandler) HandleGetTemplate(w http.ResponseWriter, r *http.Request) {
	h.handleGet(w, r, model.SavedMessageKindTemplate)
}

// HandleDeleteDraft handles DELETE /api/v1/messages/drafts/{id}.
func (h *SavedMessageHandler) HandleDeleteDraft(w http.ResponseWriter, r *http.Request) {
	h.handleDelete(w, r, model.SavedMessageKindDraft)
}

// HandleDeleteTemplate handles DELETE /api/v1/messages/templates/{id}.
func (h *SavedMessageHandler) HandleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	h.handleDelete(w, r, model.SavedMessageKindTemplate)
}

func (h *SavedMessageHandler) handleSave(w http.ResponseWriter, r *http.Request, kind model.SavedMessageKind) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var body saveMessageBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	sm, err := h.svc.Save(r.Context(), userID, kind, service.SaveMessageRequest{
		ReceiverID: body.ReceiverID,
		Subject:    body.Subject,
		Body:       body.Body,
		ParentID:   body.ParentID,
	})
	if err != nil {
		handleSavedMessageError(w, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{string(kind): savedMessageResponse(sm)})
}

func (h *SavedMessageHandler) handleUpdate(w http.ResponseWriter, r *http.Request, kind model.SavedMessageKind) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}

	var body saveMessageBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	sm, err := h.svc.Update(r.Context(), id, userID, kind, body.Version, service.SaveMessageRequest{
		ReceiverID: body.ReceiverID,
		Subject:    body.Subject,
		Body:       body.Body,
		ParentID:   body.ParentID,
	})
	if err != nil {
		handleSavedMessageError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{string(kind): savedMessageResponse(sm)})
}

func (h *SavedMessageHandler) handleList(w http.ResponseWriter, r *http.Request, kind model.SavedMessageKind) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	page := 1
	perPage := 25
	if p := r.URL.Query().Get("page"); p != "" {
		page, _ = strconv.Atoi(p)
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		perPage, _ = strconv.Atoi(pp)
	}

	items, total, err := h.svc.List(r.Context(), userID, kind, page, perPage)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list "+string(kind)+"s")
		return
	}

	resp := make([]map[string]interface{}, len(items))
	for i := range items {
		resp[i] = savedMessageResponse(&items[i])
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		string(kind) + "s": resp,
		"total":            total,
		"page":             page,
		"per_page":         perPage,
	})
}

func (h *SavedMessageHandler) handleGet(w http.ResponseWriter, r *http.Request, kind model.SavedMessageKind) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}

	sm, err := h.svc.Get(r.Context(), id, userID, kind)
	if err != nil {
		handleSavedMessageError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{string(kind): savedMessageResponse(sm)})
}

func (h *SavedMessageHandler) handleDelete(w http.ResponseWriter, r *http.Request, kind model.SavedMessageKind) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}

	if err := h.svc.Delete(r.Context(), id, userID, kind); err != nil {
		handleSavedMessageError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleSavedMessageError(w http.ResponseWriter, err error) {
	var conflict *service.SavedMessageConflictError
	switch {
	case errors.As(err, &conflict):
		// The client's PUT carried a stale version — another save (a
		// different tab, a different device) already landed. Echo back the
		// saved message as it stands right now (same shape/key as every
		// other endpoint, via savedMessageResponse) so the frontend can show
		// the caller what actually happened without a second request.
		JSON(w, http.StatusConflict, map[string]interface{}{
			"error": map[string]string{
				"code":    "conflict",
				"message": "this was changed elsewhere — reload before saving again",
			},
			string(conflict.Current.Kind): savedMessageResponse(conflict.Current),
		})
	case errors.Is(err, service.ErrSavedMessageNotFound):
		ErrorResponse(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, service.ErrInvalidSavedMessage):
		ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
	default:
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

func savedMessageResponse(sm *model.SavedMessage) map[string]interface{} {
	resp := map[string]interface{}{
		"id":         sm.ID,
		"kind":       sm.Kind,
		"subject":    sm.Subject,
		"body":       sm.Body,
		"version":    sm.Version,
		"created_at": sm.CreatedAt,
		"updated_at": sm.UpdatedAt,
	}
	if sm.ReceiverID != nil {
		resp["receiver_id"] = *sm.ReceiverID
		resp["receiver_username"] = sm.ReceiverUsername
	}
	if sm.ParentID != nil {
		resp["parent_id"] = *sm.ParentID
	}
	return resp
}
