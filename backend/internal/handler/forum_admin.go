package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// ForumAdminHandler handles admin forum CRUD HTTP endpoints.
type ForumAdminHandler struct {
	forumSvc *service.ForumService
}

// NewForumAdminHandler creates a new ForumAdminHandler.
func NewForumAdminHandler(forumSvc *service.ForumService) *ForumAdminHandler {
	return &ForumAdminHandler{forumSvc: forumSvc}
}

// HandleListForumCategories handles GET /api/v1/admin/forum-categories.
func (h *ForumAdminHandler) HandleListForumCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.forumSvc.AdminListCategories(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list forum categories")
		return
	}

	items := make([]map[string]interface{}, len(cats))
	for i, c := range cats {
		items[i] = map[string]interface{}{
			"id":         c.ID,
			"name":       c.Name,
			"sort_order": c.SortOrder,
			"created_at": c.CreatedAt,
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"categories": items,
	})
}

// HandleCreateForumCategory handles POST /api/v1/admin/forum-categories.
func (h *ForumAdminHandler) HandleCreateForumCategory(w http.ResponseWriter, r *http.Request) {
	var req service.CreateForumCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	actor := actorFromRequest(r)

	cat, err := h.forumSvc.AdminCreateCategory(r.Context(), req, actor)
	if err != nil {
		if errors.Is(err, service.ErrInvalidForumCategory) {
			ErrorResponse(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to create forum category")
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"category": map[string]interface{}{
			"id":         cat.ID,
			"name":       cat.Name,
			"sort_order": cat.SortOrder,
			"created_at": cat.CreatedAt,
		},
	})
}

// HandleUpdateForumCategory handles PUT /api/v1/admin/forum-categories/{id}.
func (h *ForumAdminHandler) HandleUpdateForumCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid category ID")
		return
	}

	var req service.UpdateForumCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	actor := actorFromRequest(r)

	cat, err := h.forumSvc.AdminUpdateCategory(r.Context(), id, req, actor)
	if err != nil {
		if errors.Is(err, service.ErrForumCategoryNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "forum category not found")
			return
		}
		if errors.Is(err, service.ErrInvalidForumCategory) {
			ErrorResponse(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to update forum category")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"category": map[string]interface{}{
			"id":         cat.ID,
			"name":       cat.Name,
			"sort_order": cat.SortOrder,
			"created_at": cat.CreatedAt,
		},
	})
}

// HandleDeleteForumCategory handles DELETE /api/v1/admin/forum-categories/{id}.
func (h *ForumAdminHandler) HandleDeleteForumCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid category ID")
		return
	}

	actor := actorFromRequest(r)

	err = h.forumSvc.AdminDeleteCategory(r.Context(), id, actor)
	if err != nil {
		if errors.Is(err, service.ErrForumCategoryNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "forum category not found")
			return
		}
		if errors.Is(err, service.ErrForumCategoryHasForums) {
			ErrorResponse(w, http.StatusConflict, "conflict", "category has forums and cannot be deleted")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete forum category")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListForums handles GET /api/v1/admin/forums.
func (h *ForumAdminHandler) HandleListForums(w http.ResponseWriter, r *http.Request) {
	forums, err := h.forumSvc.AdminListForums(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list forums")
		return
	}

	items := make([]map[string]interface{}, len(forums))
	for i, f := range forums {
		items[i] = adminForumResponse(&f)
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"forums": items,
	})
}

// HandleCreateForum handles POST /api/v1/admin/forums.
func (h *ForumAdminHandler) HandleCreateForum(w http.ResponseWriter, r *http.Request) {
	var req service.CreateForumRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	actor := actorFromRequest(r)

	forum, err := h.forumSvc.AdminCreateForum(r.Context(), req, actor)
	if err != nil {
		if errors.Is(err, service.ErrInvalidForum) {
			ErrorResponse(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		if errors.Is(err, service.ErrForumCategoryNotFound) {
			ErrorResponse(w, http.StatusUnprocessableEntity, "validation_error", "category not found")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to create forum")
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"forum": adminForumResponse(forum),
	})
}

// HandleUpdateForum handles PUT /api/v1/admin/forums/{id}.
func (h *ForumAdminHandler) HandleUpdateForum(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid forum ID")
		return
	}

	var req service.UpdateForumRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	actor := actorFromRequest(r)

	forum, err := h.forumSvc.AdminUpdateForum(r.Context(), id, req, actor)
	if err != nil {
		if errors.Is(err, service.ErrForumNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "forum not found")
			return
		}
		if errors.Is(err, service.ErrInvalidForum) {
			ErrorResponse(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		if errors.Is(err, service.ErrForumCategoryNotFound) {
			ErrorResponse(w, http.StatusUnprocessableEntity, "validation_error", "category not found")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to update forum")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"forum": adminForumResponse(forum),
	})
}

// HandleDeleteForum handles DELETE /api/v1/admin/forums/{id}.
func (h *ForumAdminHandler) HandleDeleteForum(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid forum ID")
		return
	}

	actor := actorFromRequest(r)

	err = h.forumSvc.AdminDeleteForum(r.Context(), id, actor)
	if err != nil {
		if errors.Is(err, service.ErrForumNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "forum not found")
			return
		}
		if errors.Is(err, service.ErrForumHasTopics) {
			ErrorResponse(w, http.StatusConflict, "conflict", "forum has topics and cannot be deleted")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete forum")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func adminForumResponse(f *model.Forum) map[string]interface{} {
	return map[string]interface{}{
		"id":              f.ID,
		"category_id":     f.CategoryID,
		"name":            f.Name,
		"description":     f.Description,
		"sort_order":      f.SortOrder,
		"topic_count":     f.TopicCount,
		"post_count":      f.PostCount,
		"min_group_level": f.MinGroupLevel,
		"min_post_level":  f.MinPostLevel,
		"created_at":      f.CreatedAt,
	}
}
