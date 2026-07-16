package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// BonusStoreHandler handles the user-facing bonus store endpoints.
type BonusStoreHandler struct {
	svc *service.BonusService
}

// NewBonusStoreHandler creates a new BonusStoreHandler.
func NewBonusStoreHandler(svc *service.BonusService) *BonusStoreHandler {
	return &BonusStoreHandler{svc: svc}
}

// HandleListItems handles GET /api/v1/store/items. Always 200: when the bonus
// system is disabled the payload says so and the catalogue is empty, letting
// the page render the disabled state instead of erroring.
func (h *BonusStoreHandler) HandleListItems(w http.ResponseWriter, r *http.Request) {
	view, err := h.svc.ListItems(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to load the store")
		return
	}
	JSON(w, http.StatusOK, view)
}

// HandlePurchase handles POST /api/v1/store/purchase/{itemId}.
func (h *BonusStoreHandler) HandlePurchase(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	itemID, err := strconv.ParseInt(chi.URLParam(r, "itemId"), 10, 64)
	if err != nil || itemID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid item ID")
		return
	}

	result, err := h.svc.Purchase(r.Context(), userID, itemID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrBonusDisabled):
			ErrorResponse(w, http.StatusForbidden, "forbidden", err.Error())
		case errors.Is(err, service.ErrBonusItemNotFound):
			ErrorResponse(w, http.StatusNotFound, "not_found", err.Error())
		case errors.Is(err, service.ErrBonusItemNotAvailable),
			errors.Is(err, service.ErrBonusInsufficientPoints):
			ErrorResponse(w, http.StatusConflict, "conflict", err.Error())
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "purchase failed")
		}
		return
	}

	JSON(w, http.StatusOK, result)
}
