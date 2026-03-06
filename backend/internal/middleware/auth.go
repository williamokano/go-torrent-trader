package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

type contextKey string

const (
	UserIDKey      contextKey = "user_id"
	GroupIDKey     contextKey = "group_id"
	AccessTokenKey contextKey = "access_token"
)

// SessionValidator validates an access token and returns user ID and group ID.
type SessionValidator interface {
	ValidateSession(accessToken string) (userID int64, groupID int64, ok bool)
}

// RequireAuth is a middleware that extracts the Bearer token from the Authorization
// header, validates it, and sets the user ID, group ID, and access token in the request context.
func RequireAuth(validator SessionValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ExtractBearerToken(r)
			if token == "" {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid authorization header")
				return
			}

			userID, groupID, ok := validator.ValidateSession(token)
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			ctx = context.WithValue(ctx, GroupIDKey, groupID)
			ctx = context.WithValue(ctx, AccessTokenKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin is a middleware that checks the user has the Administrator group (ID=1).
// Must be used after RequireAuth.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		groupID, ok := r.Context().Value(GroupIDKey).(int64)
		if !ok || groupID != 1 {
			writeJSONError(w, http.StatusForbidden, "forbidden", "administrator access required")
			return
		}
		next.ServeHTTP(w, r)
	})
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
