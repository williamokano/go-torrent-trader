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
	SessionStore   service.SessionStore
	UserService    *service.UserService
	TorrentService *service.TorrentService
	TrackerService *service.TrackerService
	ReportService  *service.ReportService
	CommentService *service.CommentService
	AdminService   *service.AdminService
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
			auth := NewAuthHandler(deps.AuthService, deps.UserService)
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

			// User profile endpoints (all protected)
			if deps.UserService != nil {
				users := NewUserHandler(deps.UserService)
				r.Route("/users", func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))
					r.Get("/{id}", users.HandleGetProfile)
					r.Put("/me/profile", users.HandleUpdateProfile)
					r.Put("/me/password", users.HandleChangePassword)
					r.Post("/me/passkey", users.HandleRegeneratePasskey)
				})
			}

			// Torrent endpoints (all protected)
			if deps.TorrentService != nil {
				torrents := NewTorrentHandler(deps.TorrentService)
				r.Route("/torrents", func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))
					r.Get("/", torrents.HandleList)
					r.Get("/{id}", torrents.HandleGetByID)
					r.Put("/{id}", torrents.HandleEdit)
					r.Delete("/{id}", torrents.HandleDelete)

					// Capability-gated endpoints
					r.With(mw.RequireCapability("upload")).Post("/", torrents.HandleUpload)
					r.With(mw.RequireCapability("download")).Get("/{id}/download", torrents.HandleDownload)

					// Comment and rating endpoints
					if deps.CommentService != nil {
						comments := NewCommentHandler(deps.CommentService)
						r.Get("/{id}/comments", comments.HandleListComments)
						r.Get("/{id}/rating", comments.HandleGetRating)
						r.With(mw.RequireCapability("comment")).Post("/{id}/comments", comments.HandleCreateComment)
						r.With(mw.RequireCapability("comment")).Post("/{id}/rating", comments.HandleRateTorrent)
					}
				})
			}

			// Comment edit/delete endpoints (separate route group)
			if deps.CommentService != nil {
				comments := NewCommentHandler(deps.CommentService)
				r.Route("/comments", func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))
					r.Put("/{id}", comments.HandleEditComment)
					r.Delete("/{id}", comments.HandleDeleteComment)
				})
			}

			// Report endpoints
			if deps.ReportService != nil {
				reports := NewReportHandler(deps.ReportService)
				r.Route("/reports", func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))
					// Any authenticated user can submit a report
					r.Post("/", reports.HandleCreate)

					// Admin-only endpoints
					r.Group(func(r chi.Router) {
						r.Use(mw.RequireAdmin)
						r.Get("/", reports.HandleList)
						r.Put("/{id}/resolve", reports.HandleResolve)
					})
				})
			}

			// Admin endpoints
			if deps.AdminService != nil {
				admin := NewAdminHandler(deps.AdminService)
				r.Route("/admin", func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))
					r.Use(mw.RequireAdmin)
					r.Get("/users", admin.HandleListUsers)
					r.Put("/users/{id}", admin.HandleUpdateUser)
					r.Get("/groups", admin.HandleListGroups)
				})
			}
		}
	})

	return r
}
