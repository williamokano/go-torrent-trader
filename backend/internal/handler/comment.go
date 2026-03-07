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

// CommentHandler handles comment and rating HTTP endpoints.
type CommentHandler struct {
	commentSvc *service.CommentService
}

// NewCommentHandler creates a new CommentHandler.
func NewCommentHandler(commentSvc *service.CommentService) *CommentHandler {
	return &CommentHandler{commentSvc: commentSvc}
}

// HandleCreateComment handles POST /api/v1/torrents/{id}/comments.
func (h *CommentHandler) HandleCreateComment(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	torrentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || torrentID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid torrent ID")
		return
	}

	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	comment, err := h.commentSvc.CreateComment(r.Context(), torrentID, userID, body.Body)
	if err != nil {
		handleCommentError(w, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"comment": commentResponse(comment),
	})
}

// HandleListComments handles GET /api/v1/torrents/{id}/comments.
func (h *CommentHandler) HandleListComments(w http.ResponseWriter, r *http.Request) {
	torrentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || torrentID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid torrent ID")
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

	comments, total, err := h.commentSvc.ListComments(r.Context(), torrentID, page, perPage)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list comments")
		return
	}

	items := make([]map[string]interface{}, len(comments))
	for i := range comments {
		items[i] = commentResponse(&comments[i])
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"comments": items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// HandleEditComment handles PUT /api/v1/comments/{id}.
func (h *CommentHandler) HandleEditComment(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	perms := middleware.PermissionsFromContext(r.Context())

	commentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || commentID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid comment ID")
		return
	}

	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	comment, err := h.commentSvc.UpdateComment(r.Context(), commentID, userID, perms, body.Body)
	if err != nil {
		handleCommentError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"comment": commentResponse(comment),
	})
}

// HandleDeleteComment handles DELETE /api/v1/comments/{id}.
func (h *CommentHandler) HandleDeleteComment(w http.ResponseWriter, r *http.Request) {
	perms := middleware.PermissionsFromContext(r.Context())

	commentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || commentID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid comment ID")
		return
	}

	if err := h.commentSvc.DeleteComment(r.Context(), commentID, perms); err != nil {
		handleCommentError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleRateTorrent handles POST /api/v1/torrents/{id}/rating.
func (h *CommentHandler) HandleRateTorrent(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	torrentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || torrentID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid torrent ID")
		return
	}

	var body struct {
		Rating int `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if err := h.commentSvc.RateTorrent(r.Context(), torrentID, userID, body.Rating); err != nil {
		handleCommentError(w, err)
		return
	}

	// Return updated rating stats.
	stats, err := h.commentSvc.GetRatingStats(r.Context(), torrentID, &userID)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to get rating stats")
		return
	}

	JSON(w, http.StatusOK, ratingStatsResponse(stats))
}

// HandleGetRating handles GET /api/v1/torrents/{id}/rating.
func (h *CommentHandler) HandleGetRating(w http.ResponseWriter, r *http.Request) {
	torrentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || torrentID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid torrent ID")
		return
	}

	// User ID is optional for the rating endpoint (to include user_rating).
	var userIDPtr *int64
	if uid, ok := middleware.UserIDFromContext(r.Context()); ok {
		userIDPtr = &uid
	}

	stats, err := h.commentSvc.GetRatingStats(r.Context(), torrentID, userIDPtr)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to get rating stats")
		return
	}

	JSON(w, http.StatusOK, ratingStatsResponse(stats))
}

func handleCommentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidComment):
		ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, service.ErrInvalidRating):
		ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, service.ErrCommentNotFound):
		ErrorResponse(w, http.StatusNotFound, "not_found", "comment not found")
	case errors.Is(err, service.ErrTorrentNotFound):
		ErrorResponse(w, http.StatusNotFound, "not_found", "torrent not found")
	case errors.Is(err, service.ErrForbidden):
		ErrorResponse(w, http.StatusForbidden, "forbidden", "you do not have permission to perform this action")
	default:
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

func commentResponse(c *model.Comment) map[string]interface{} {
	return map[string]interface{}{
		"id":         c.ID,
		"torrent_id": c.TorrentID,
		"user_id":    c.UserID,
		"username":   c.Username,
		"body":       c.Body,
		"created_at": c.CreatedAt,
		"updated_at": c.UpdatedAt,
	}
}

func ratingStatsResponse(stats *model.RatingStats) map[string]interface{} {
	resp := map[string]interface{}{
		"average": stats.Average,
		"count":   stats.Count,
	}
	if stats.UserRating != nil {
		resp["user_rating"] = *stats.UserRating
	}
	return resp
}
