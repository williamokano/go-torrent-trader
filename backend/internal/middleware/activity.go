package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

const activityDebounce = 5 * time.Minute

// ActivityTracker is an HTTP middleware that periodically updates the
// last_access timestamp for authenticated users. It debounces updates so
// that the database is touched at most once per activityDebounce interval
// per user.
type ActivityTracker struct {
	users    repository.UserRepository
	mu       sync.Mutex
	lastSeen map[int64]time.Time
}

// NewActivityTracker creates a new ActivityTracker.
func NewActivityTracker(users repository.UserRepository) *ActivityTracker {
	return &ActivityTracker{
		users:    users,
		lastSeen: make(map[int64]time.Time),
	}
}

// Track returns middleware that updates the user's last_access timestamp.
func (a *ActivityTracker) Track(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Let the request complete first.
		next.ServeHTTP(w, r)

		userID, ok := UserIDFromContext(r.Context())
		if !ok || userID == 0 {
			return
		}

		a.mu.Lock()
		last, exists := a.lastSeen[userID]
		now := time.Now()
		if exists && now.Sub(last) < activityDebounce {
			a.mu.Unlock()
			return
		}
		a.lastSeen[userID] = now
		a.mu.Unlock()

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := a.users.UpdateLastAccess(ctx, userID); err != nil {
				slog.Error("failed to update last access", "user_id", userID, "error", err)
			}
		}()
	})
}
