package handler

import (
	"net/http"
	"strconv"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// MemberHandler handles member list HTTP endpoints.
type MemberHandler struct {
	members *service.MemberService
}

// NewMemberHandler creates a new MemberHandler.
func NewMemberHandler(members *service.MemberService) *MemberHandler {
	return &MemberHandler{members: members}
}

// HandleList handles GET /api/v1/users (paginated member list).
func (h *MemberHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	opts := repository.ListUsersOptions{}

	if search := r.URL.Query().Get("search"); search != "" {
		opts.Search = search
	}
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		opts.Page, _ = strconv.Atoi(pageStr)
	}
	if ppStr := r.URL.Query().Get("per_page"); ppStr != "" {
		opts.PerPage, _ = strconv.Atoi(ppStr)
	}

	users, total, err := h.members.ListMembers(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list members")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"users":    users,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
	})
}

// HandleStaff handles GET /api/v1/users/staff.
func (h *MemberHandler) HandleStaff(w http.ResponseWriter, r *http.Request) {
	staff, err := h.members.ListStaff(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list staff")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"staff": staff,
	})
}
