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

// ForumHandler handles forum HTTP endpoints.
type ForumHandler struct {
	forumSvc *service.ForumService
}

// NewForumHandler creates a new ForumHandler.
func NewForumHandler(forumSvc *service.ForumService) *ForumHandler {
	return &ForumHandler{forumSvc: forumSvc}
}

// HandleListForums handles GET /api/v1/forums — list all categories with forums.
func (h *ForumHandler) HandleListForums(w http.ResponseWriter, r *http.Request) {
	perms := middleware.PermissionsFromContext(r.Context())

	categories, err := h.forumSvc.ListCategories(r.Context(), perms)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list forums")
		return
	}

	result := make([]map[string]interface{}, 0, len(categories))
	for _, cat := range categories {
		forums := make([]map[string]interface{}, 0, len(cat.Forums))
		for _, f := range cat.Forums {
			forums = append(forums, forumResponse(&f))
		}
		result = append(result, map[string]interface{}{
			"id":         cat.ID,
			"name":       cat.Name,
			"sort_order": cat.SortOrder,
			"forums":     forums,
		})
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"categories": result,
	})
}

// HandleGetForum handles GET /api/v1/forums/{id} — get forum details.
func (h *ForumHandler) HandleGetForum(w http.ResponseWriter, r *http.Request) {
	perms := middleware.PermissionsFromContext(r.Context())

	forumID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || forumID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid forum ID")
		return
	}

	forum, err := h.forumSvc.GetForum(r.Context(), forumID, perms)
	if err != nil {
		handleForumError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"forum": forumResponse(forum),
	})
}

// HandleListTopics handles GET /api/v1/forums/{id}/topics — list topics in a forum.
func (h *ForumHandler) HandleListTopics(w http.ResponseWriter, r *http.Request) {
	perms := middleware.PermissionsFromContext(r.Context())

	forumID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || forumID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid forum ID")
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

	forum, topics, total, err := h.forumSvc.ListTopics(r.Context(), forumID, perms, page, perPage)
	if err != nil {
		handleForumError(w, err)
		return
	}

	items := make([]map[string]interface{}, 0, len(topics))
	for _, t := range topics {
		items = append(items, topicResponse(&t))
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"forum":    forumResponse(forum),
		"topics":   items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// HandleGetTopic handles GET /api/v1/forums/topics/{id} — get topic with posts.
func (h *ForumHandler) HandleGetTopic(w http.ResponseWriter, r *http.Request) {
	perms := middleware.PermissionsFromContext(r.Context())

	topicID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || topicID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid topic ID")
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

	topic, err := h.forumSvc.GetTopic(r.Context(), topicID, perms)
	if err != nil {
		handleForumError(w, err)
		return
	}

	posts, total, err := h.forumSvc.ListPosts(r.Context(), topicID, page, perPage)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list posts")
		return
	}

	postItems := make([]map[string]interface{}, 0, len(posts))
	for _, p := range posts {
		postItems = append(postItems, postResponse(&p))
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"topic":    topicResponse(topic),
		"posts":    postItems,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// HandleCreateTopic handles POST /api/v1/forums/{id}/topics — create new topic.
func (h *ForumHandler) HandleCreateTopic(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	perms := middleware.PermissionsFromContext(r.Context())

	forumID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || forumID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid forum ID")
		return
	}

	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	topic, post, err := h.forumSvc.CreateTopic(r.Context(), forumID, userID, perms, body.Title, body.Body)
	if err != nil {
		handleForumError(w, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"topic": topicResponse(topic),
		"post":  postResponse(post),
	})
}

// HandleCreatePost handles POST /api/v1/forums/topics/{id}/posts — create reply.
func (h *ForumHandler) HandleCreatePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	perms := middleware.PermissionsFromContext(r.Context())

	topicID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || topicID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid topic ID")
		return
	}

	var body struct {
		Body          string `json:"body"`
		ReplyToPostID *int64 `json:"reply_to_post_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	post, err := h.forumSvc.CreatePost(r.Context(), topicID, userID, perms, body.Body, body.ReplyToPostID)
	if err != nil {
		handleForumError(w, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"post": postResponse(post),
	})
}

func handleForumError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrForumNotFound):
		ErrorResponse(w, http.StatusNotFound, "not_found", "forum not found")
	case errors.Is(err, service.ErrTopicNotFound):
		ErrorResponse(w, http.StatusNotFound, "not_found", "topic not found")
	case errors.Is(err, service.ErrTopicLocked):
		ErrorResponse(w, http.StatusForbidden, "forbidden", "topic is locked")
	case errors.Is(err, service.ErrForumAccessDenied):
		ErrorResponse(w, http.StatusForbidden, "forbidden", "you do not have access to this forum")
	case errors.Is(err, service.ErrInvalidTopic):
		ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, service.ErrInvalidPost):
		ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
	default:
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

func forumResponse(f *model.Forum) map[string]interface{} {
	resp := map[string]interface{}{
		"id":              f.ID,
		"category_id":     f.CategoryID,
		"name":            f.Name,
		"description":     f.Description,
		"sort_order":      f.SortOrder,
		"topic_count":     f.TopicCount,
		"post_count":      f.PostCount,
		"min_group_level": f.MinGroupLevel,
		"created_at":      f.CreatedAt,
	}
	if f.LastPostAt != nil {
		resp["last_post_at"] = *f.LastPostAt
	}
	if f.LastPostUsername != nil {
		resp["last_post_username"] = *f.LastPostUsername
	}
	if f.LastPostTopicID != nil {
		resp["last_post_topic_id"] = *f.LastPostTopicID
	}
	if f.LastPostTopicTitle != nil {
		resp["last_post_topic_title"] = *f.LastPostTopicTitle
	}
	return resp
}

func topicResponse(t *model.ForumTopic) map[string]interface{} {
	resp := map[string]interface{}{
		"id":         t.ID,
		"forum_id":   t.ForumID,
		"user_id":    t.UserID,
		"username":   t.Username,
		"title":      t.Title,
		"pinned":     t.Pinned,
		"locked":     t.Locked,
		"post_count": t.PostCount,
		"view_count": t.ViewCount,
		"created_at": t.CreatedAt,
		"updated_at": t.UpdatedAt,
		"forum_name": t.ForumName,
	}
	if t.LastPostAt != nil {
		resp["last_post_at"] = *t.LastPostAt
	}
	if t.LastPostUsername != nil {
		resp["last_post_username"] = *t.LastPostUsername
	}
	return resp
}

func postResponse(p *model.ForumPost) map[string]interface{} {
	resp := map[string]interface{}{
		"id":              p.ID,
		"topic_id":        p.TopicID,
		"user_id":         p.UserID,
		"username":        p.Username,
		"avatar":          p.Avatar,
		"group_name":      p.GroupName,
		"body":            p.Body,
		"created_at":      p.CreatedAt,
		"user_created_at": p.UserCreatedAt,
		"user_post_count": p.UserPostCount,
	}
	if p.ReplyToPostID != nil {
		resp["reply_to_post_id"] = *p.ReplyToPostID
	}
	if p.EditedAt != nil {
		resp["edited_at"] = *p.EditedAt
	}
	if p.EditedBy != nil {
		resp["edited_by"] = *p.EditedBy
	}
	return resp
}
