package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// CategoryAdminHandler handles admin category HTTP endpoints.
type CategoryAdminHandler struct {
	categories *service.CategoryService
}

// NewCategoryAdminHandler creates a new CategoryAdminHandler.
func NewCategoryAdminHandler(categories *service.CategoryService) *CategoryAdminHandler {
	return &CategoryAdminHandler{categories: categories}
}

// HandleListCategories handles GET /api/v1/admin/categories.
func (h *CategoryAdminHandler) HandleListCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.categories.List(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list categories")
		return
	}

	items := make([]map[string]interface{}, len(cats))
	for i, c := range cats {
		items[i] = map[string]interface{}{
			"id":         c.ID,
			"name":       c.Name,
			"slug":       c.Slug,
			"parent_id":  c.ParentID,
			"image_url":  c.ImageURL,
			"sort_order": c.SortOrder,
			"created_at": c.CreatedAt.Format("2006-01-02T15:04:05Z"),
			"updated_at": c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"categories": items,
	})
}

// HandleCreateCategory handles POST /api/v1/admin/categories.
func (h *CategoryAdminHandler) HandleCreateCategory(w http.ResponseWriter, r *http.Request) {
	var req service.CreateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	cat, err := h.categories.Create(r.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCategory) {
			ErrorResponse(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to create category")
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"category": map[string]interface{}{
			"id":         cat.ID,
			"name":       cat.Name,
			"slug":       cat.Slug,
			"parent_id":  cat.ParentID,
			"image_url":  cat.ImageURL,
			"sort_order": cat.SortOrder,
			"created_at": cat.CreatedAt.Format("2006-01-02T15:04:05Z"),
			"updated_at": cat.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		},
	})
}

// HandleUpdateCategory handles PUT /api/v1/admin/categories/{id}.
func (h *CategoryAdminHandler) HandleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid category ID")
		return
	}

	var req service.UpdateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	cat, err := h.categories.Update(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, service.ErrCategoryNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "category not found")
			return
		}
		if errors.Is(err, service.ErrInvalidCategory) {
			ErrorResponse(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to update category")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"category": map[string]interface{}{
			"id":         cat.ID,
			"name":       cat.Name,
			"slug":       cat.Slug,
			"parent_id":  cat.ParentID,
			"image_url":  cat.ImageURL,
			"sort_order": cat.SortOrder,
			"created_at": cat.CreatedAt.Format("2006-01-02T15:04:05Z"),
			"updated_at": cat.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		},
	})
}

// HandleDeleteCategory handles DELETE /api/v1/admin/categories/{id}.
func (h *CategoryAdminHandler) HandleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid category ID")
		return
	}

	err = h.categories.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrCategoryNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "category not found")
			return
		}
		if errors.Is(err, service.ErrCategoryHasTorrents) {
			ErrorResponse(w, http.StatusConflict, "conflict", "category has torrents and cannot be deleted")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete category")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
