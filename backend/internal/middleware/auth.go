package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

type contextKey string

const (
	UserIDKey      contextKey = "user_id"
	GroupIDKey     contextKey = "group_id"
	PermissionsKey contextKey = "permissions"
	AccessTokenKey contextKey = "access_token"
)

// SessionValidator validates an access token and returns user info and permissions.
type SessionValidator interface {
	ValidateSession(accessToken string) (userID int64, perms model.Permissions, ok bool)
}

// RequireAuth is a middleware that extracts the Bearer token from the Authorization
// header, validates it, and sets the user ID, permissions, and access token in the request context.
func RequireAuth(validator SessionValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ExtractBearerToken(r)
			if token == "" {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid authorization header")
				return
			}

			userID, perms, ok := validator.ValidateSession(token)
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			ctx = context.WithValue(ctx, GroupIDKey, perms.GroupID)
			ctx = context.WithValue(ctx, PermissionsKey, perms)
			ctx = context.WithValue(ctx, AccessTokenKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin is a middleware that checks the user has admin permissions.
// Must be used after RequireAuth.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		perms := PermissionsFromContext(r.Context())
		if !perms.IsAdmin {
			writeJSONError(w, http.StatusForbidden, "forbidden", "administrator access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireStaff is a middleware that checks the user is admin or moderator.
// Must be used after RequireAuth.
func RequireStaff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		perms := PermissionsFromContext(r.Context())
		if !perms.IsStaff() {
			writeJSONError(w, http.StatusForbidden, "forbidden", "staff access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireCapability returns a middleware that checks for a specific group capability.
// Supported capabilities: "upload", "download", "invite", "comment", "forum".
func RequireCapability(cap string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			perms := PermissionsFromContext(r.Context())
			allowed := false
			switch cap {
			case "upload":
				allowed = perms.CanUpload
			case "download":
				allowed = perms.CanDownload
			case "invite":
				allowed = perms.CanInvite
			case "comment":
				allowed = perms.CanComment
			case "forum":
				allowed = perms.CanForum
			}
			if !allowed {
				writeJSONError(w, http.StatusForbidden, "forbidden", "you do not have the "+cap+" capability")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// UserIDFromContext extracts the user ID from the request context.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(UserIDKey).(int64)
	return id, ok
}

// GroupIDFromContext extracts the group ID from the request context.
func GroupIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(GroupIDKey).(int64)
	return id, ok
}

// PermissionsFromContext extracts the Permissions from the request context.
// Returns zero-value Permissions if not set (safe: all false).
func PermissionsFromContext(ctx context.Context) model.Permissions {
	perms, _ := ctx.Value(PermissionsKey).(model.Permissions)
	return perms
}

// AccessTokenFromContext extracts the raw access token from the request context.
func AccessTokenFromContext(ctx context.Context) (string, bool) {
	tok, ok := ctx.Value(AccessTokenKey).(string)
	return tok, ok
}

// ExtractBearerToken parses a Bearer token from the Authorization header.
func ExtractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("failed to encode error response", "error", err)
	}
}
