package handler

import (
	"database/sql"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	mw "github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// Deps holds handler dependencies. Pass nil for a minimal router (e.g. in tests).
type Deps struct {
	DB             *sql.DB
	AuthService    *service.AuthService
	SessionStore   *service.SessionStore
	TorrentService *service.TorrentService
	TrackerService *service.TrackerService
}

// NewRouter creates and configures the Chi router with middleware and routes.
func NewRouter(deps *Deps) chi.Router {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(mw.RequestLogger)
	r.Use(chimw.Recoverer)
	r.Use(mw.CORS)

	// Health check
	r.Get("/healthz", HandleHealthz)

	// Tracker endpoints (public, no auth required — use passkey in URL)
	if deps != nil && deps.TrackerService != nil {
		announceHandler := NewAnnounceHandler(deps.TrackerService)
		scrapeHandler := NewScrapeHandler(deps.TrackerService)
		r.Get("/announce", announceHandler.HandleAnnounce)
		r.Get("/scrape", scrapeHandler.HandleScrape)
	}

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public endpoints
		if deps != nil && deps.DB != nil {
			r.Get("/stats", HandleStats(deps.DB))
			r.Get("/categories", HandleCategories(deps.DB))
		}

		if deps != nil && deps.AuthService != nil {
			auth := NewAuthHandler(deps.AuthService)
			validator := NewSessionValidatorAdapter(deps.SessionStore)

			r.Route("/auth", func(r chi.Router) {
				// Public auth endpoints
				r.Post("/register", auth.HandleRegister)
				r.Post("/login", auth.HandleLogin)
				r.Post("/refresh", auth.HandleRefresh)
				r.Post("/forgot-password", auth.HandleForgotPassword)
				r.Post("/reset-password", auth.HandleResetPassword)

				// Protected auth endpoints
				r.Group(func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))
					r.Post("/logout", auth.HandleLogout)
					r.Get("/me", auth.HandleMe)
				})
			})

			// Torrent endpoints (all protected)
			if deps.TorrentService != nil {
				torrents := NewTorrentHandler(deps.TorrentService)
				r.Route("/torrents", func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))
					r.Post("/", torrents.HandleUpload)
					r.Get("/", torrents.HandleList)
					r.Get("/{id}", torrents.HandleGetByID)
					r.Get("/{id}/download", torrents.HandleDownload)
				})
			}
		}
	})

	return r
}
