package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// NewsHandler handles news HTTP endpoints.
type NewsHandler struct {
	news *service.NewsService
}

// NewNewsHandler creates a new NewsHandler.
func NewNewsHandler(news *service.NewsService) *NewsHandler {
	return &NewsHandler{news: news}
}

// HandleAdminCreateNews handles POST /api/v1/admin/news.
func (h *NewsHandler) HandleAdminCreateNews(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req service.CreateNewsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	article, err := h.news.Create(r.Context(), req, actorID)
	if err != nil {
		if errors.Is(err, service.ErrInvalidNews) {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to create news article")
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"article": article,
	})
}

// HandleAdminListNews handles GET /api/v1/admin/news.
func (h *NewsHandler) HandleAdminListNews(w http.ResponseWriter, r *http.Request) {
	page, perPage := parsePagination(r)
	opts := repository.ListNewsOptions{
		Page:    page,
		PerPage: perPage,
	}

	articles, total, err := h.news.List(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list news")
		return
	}

	if articles == nil {
		articles = []model.NewsArticle{}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"articles": articles,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
	})
}

// HandleAdminUpdateNews handles PUT /api/v1/admin/news/{id}.
func (h *NewsHandler) HandleAdminUpdateNews(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid article ID")
		return
	}

	var req service.UpdateNewsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	article, err := h.news.Update(r.Context(), id, req, actorID)
	if err != nil {
		if errors.Is(err, service.ErrNewsNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "news article not found")
			return
		}
		if errors.Is(err, service.ErrInvalidNews) {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to update news article")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"article": article,
	})
}

// HandleAdminDeleteNews handles DELETE /api/v1/admin/news/{id}.
func (h *NewsHandler) HandleAdminDeleteNews(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid article ID")
		return
	}

	if err := h.news.Delete(r.Context(), id); err != nil {
		if errors.Is(err, service.ErrNewsNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "news article not found")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete news article")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListPublishedNews handles GET /api/v1/news.
func (h *NewsHandler) HandleListPublishedNews(w http.ResponseWriter, r *http.Request) {
	page, perPage := parsePagination(r)

	articles, total, err := h.news.ListPublished(r.Context(), page, perPage)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list news")
		return
	}

	if articles == nil {
		articles = []model.NewsArticle{}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"articles": articles,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// HandleGetPublishedNews handles GET /api/v1/news/{id}.
func (h *NewsHandler) HandleGetPublishedNews(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid article ID")
		return
	}

	article, err := h.news.GetPublished(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrNewsNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "news article not found")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to get news article")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"article": article,
	})
}
