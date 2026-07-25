package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// ConnectorStatusProvider reports the lifecycle state of persistent connector
// instances on this node. Implemented by connector.Manager; nil when no
// persistent connectors are wired.
type ConnectorStatusProvider interface {
	Status() map[int64]connector.ManagerStatus
}

// ConnectorHandler serves the admin API for external notification connectors.
type ConnectorHandler struct {
	svc    *service.ConnectorService
	status ConnectorStatusProvider
}

// NewConnectorHandler creates a new ConnectorHandler. status may be nil, in
// which case the status endpoint reports an empty set.
func NewConnectorHandler(svc *service.ConnectorService, status ConnectorStatusProvider) *ConnectorHandler {
	return &ConnectorHandler{svc: svc, status: status}
}

// connectorErrorStatus maps the service's sentinel errors onto HTTP statuses so
// admin input problems never look like server failures.
func connectorErrorStatus(err error) (int, bool) {
	switch {
	case errors.Is(err, service.ErrConnectorNotFound):
		return http.StatusNotFound, true
	case errors.Is(err, service.ErrConnectorSingleton):
		return http.StatusConflict, true
	case errors.Is(err, service.ErrConnectorUnknownKind),
		errors.Is(err, service.ErrConnectorKindImmutable),
		errors.Is(err, service.ErrConnectorInvalid):
		return http.StatusBadRequest, true
	default:
		return 0, false
	}
}

func (h *ConnectorHandler) respondError(w http.ResponseWriter, err error, fallback string) {
	if status, ok := connectorErrorStatus(err); ok {
		code := "bad_request"
		switch status {
		case http.StatusNotFound:
			code = "not_found"
		case http.StatusConflict:
			code = "conflict"
		}
		ErrorResponse(w, status, code, err.Error())
		return
	}
	// The client gets a generic message; the cause only exists here, so it has
	// to be logged or an unexpected failure leaves no trace at all.
	slog.Error("connector admin request failed", "detail", fallback, "error", err)
	ErrorResponse(w, http.StatusInternalServerError, "internal_error", fallback)
}

func connectorID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// HandleList handles GET /api/v1/admin/connectors.
//
// The response carries the registered kinds and the template surface alongside
// the instances, so the UI's kind picker and template help always match what
// this build actually supports.
func (h *ConnectorHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	connectors, err := h.svc.List(r.Context())
	if err != nil {
		h.respondError(w, err, "failed to list connectors")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"connectors": connectors,
		"kinds":      h.svc.Kinds(),
		// The template surface travels with the list so the help beside the
		// template box always describes this build, rather than a copy in the
		// frontend that can drift from it.
		"template_fields": connector.TemplateFields(),
	})
}

// HandleStatus handles GET /api/v1/admin/connectors/status.
//
// The answer is deliberately node-local: it says what THIS process is doing, so
// a standby honestly reports not_owner rather than guessing about the cluster.
// With the single-process default that distinction never comes up.
func (h *ConnectorHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	statuses := map[string]connector.ManagerStatus{}
	if h.status != nil {
		for id, status := range h.status.Status() {
			statuses[strconv.FormatInt(id, 10)] = status
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{"statuses": statuses})
}

// HandleCreate handles POST /api/v1/admin/connectors.
func (h *ConnectorHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var in service.ConnectorInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	view, err := h.svc.Create(r.Context(), in)
	if err != nil {
		h.respondError(w, err, "failed to create connector")
		return
	}
	JSON(w, http.StatusCreated, map[string]interface{}{"connector": view})
}

// HandleGet handles GET /api/v1/admin/connectors/{id}.
func (h *ConnectorHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, ok := connectorID(r)
	if !ok {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid connector ID")
		return
	}
	view, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.respondError(w, err, "failed to load connector")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"connector": view})
}

// HandleUpdate handles PUT /api/v1/admin/connectors/{id}. It is also how the
// admin list toggles an instance on or off: the row is submitted whole with
// `enabled` flipped, and omitted secrets keep their stored values.
func (h *ConnectorHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := connectorID(r)
	if !ok {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid connector ID")
		return
	}
	var in service.ConnectorInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	view, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		h.respondError(w, err, "failed to update connector")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"connector": view})
}

// HandleDelete handles DELETE /api/v1/admin/connectors/{id}.
func (h *ConnectorHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := connectorID(r)
	if !ok {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid connector ID")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.respondError(w, err, "failed to delete connector")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

// HandleTestSend handles POST /api/v1/admin/connectors/{id}/test.
//
// A connector that refuses the test is not an API error: the request succeeded,
// the answer is "this destination did not accept it". So a failed send comes
// back 200 with status "failed" and the (redacted) reason, which is what the UI
// shows the admin.
func (h *ConnectorHandler) HandleTestSend(w http.ResponseWriter, r *http.Request) {
	id, ok := connectorID(r)
	if !ok {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid connector ID")
		return
	}
	result, err := h.svc.TestSend(r.Context(), id)
	if err != nil {
		h.respondError(w, err, "failed to run connector test")
		return
	}
	body := map[string]interface{}{
		"status":   result.Status,
		"delivery": result,
	}
	if result.Status != model.DeliverySent {
		body["error"] = result.LastError
	}
	JSON(w, http.StatusOK, body)
}

// HandleDeliveries handles GET /api/v1/admin/connectors/{id}/deliveries.
func (h *ConnectorHandler) HandleDeliveries(w http.ResponseWriter, r *http.Request) {
	id, ok := connectorID(r)
	if !ok {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid connector ID")
		return
	}

	page, perPage := parsePagination(r)

	deliveries, total, err := h.svc.Deliveries(r.Context(), id, page, perPage)
	if err != nil {
		h.respondError(w, err, "failed to list connector deliveries")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"deliveries": deliveries,
		"total":      total,
		"page":       page,
		"per_page":   perPage,
	})
}
