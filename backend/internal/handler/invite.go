package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// InviteHandler handles invite HTTP endpoints.
type InviteHandler struct {
	invites *service.InviteService
}

// NewInviteHandler creates a new InviteHandler.
func NewInviteHandler(invites *service.InviteService) *InviteHandler {
	return &InviteHandler{invites: invites}
}

// HandleCreateInvite handles POST /api/v1/invites.
func (h *InviteHandler) HandleCreateInvite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	invite, err := h.invites.CreateInvite(r.Context(), userID)
	if err != nil {
		handleInviteError(w, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"invite": inviteResponse(invite),
	})
}

// HandleListInvites handles GET /api/v1/invites.
func (h *InviteHandler) HandleListInvites(w http.ResponseWriter, r *http.Request) {
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

	invites, total, err := h.invites.ListMyInvites(r.Context(), userID, page, perPage)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list invites")
		return
	}

	items := make([]map[string]interface{}, len(invites))
	for i := range invites {
		items[i] = inviteResponse(&invites[i])
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"invites":  items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// HandleValidateInvite handles GET /api/v1/invites/{token}.
func (h *InviteHandler) HandleValidateInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "missing invite token")
		return
	}

	_, err := h.invites.ValidateInvite(r.Context(), token)
	if err != nil {
		handleInviteError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"valid": true,
	})
}

func handleInviteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrNoInvitesRemaining):
		ErrorResponse(w, http.StatusForbidden, "no_invites", "you have no invites remaining")
	case errors.Is(err, service.ErrInviteNotFound):
		ErrorResponse(w, http.StatusNotFound, "not_found", "invite not found")
	case errors.Is(err, service.ErrInviteExpired):
		ErrorResponse(w, http.StatusGone, "expired", "invite has expired")
	case errors.Is(err, service.ErrInviteRedeemed):
		ErrorResponse(w, http.StatusConflict, "redeemed", "invite has already been redeemed")
	default:
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

func inviteResponse(inv *model.Invite) map[string]interface{} {
	status := "pending"
	if inv.Redeemed {
		status = "redeemed"
	} else if time.Now().After(inv.ExpiresAt) {
		status = "expired"
	}

	resp := map[string]interface{}{
		"id":         inv.ID,
		"token":      inv.Token,
		"status":     status,
		"expires_at": inv.ExpiresAt,
		"created_at": inv.CreatedAt,
	}
	if inv.InviteeID != nil {
		resp["invitee_id"] = *inv.InviteeID
	}
	if inv.RedeemedAt != nil {
		resp["redeemed_at"] = *inv.RedeemedAt
	}
	return resp
}
