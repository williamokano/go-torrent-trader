package handler

import (
	"database/sql"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	mw "github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// Deps holds handler dependencies. Pass nil for a minimal router (e.g. in tests).
type Deps struct {
	DB                  *sql.DB
	AuthService         *service.AuthService
	SessionStore        service.SessionStore
	UserService         *service.UserService
	MemberService       *service.MemberService
	TorrentService      *service.TorrentService
	TrackerService      *service.TrackerService
	ReportService       *service.ReportService
	CommentService      *service.CommentService
	InviteService       *service.InviteService
	AdminService        *service.AdminService
	CategoryService     *service.CategoryService
	ActivityLogService  *service.ActivityLogService
	SiteSettingsService *service.SiteSettingsService
	BanService          *service.BanService
	PeerRepo            repository.PeerRepository
	UserRepo            repository.UserRepository
	RSSConfig           *RSSConfig
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

		// RSS feed (public, authenticated via passkey query param)
		if deps != nil && deps.TorrentService != nil && deps.UserRepo != nil && deps.RSSConfig != nil {
			rssHandler := NewRSSHandler(deps.TorrentService, deps.UserRepo, *deps.RSSConfig)
			r.Get("/rss", rssHandler.HandleRSS)
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

				// Public registration mode endpoint
				if deps.SiteSettingsService != nil {
					settingsHandler := NewSiteSettingsHandler(deps.SiteSettingsService)
					r.Get("/registration-mode", settingsHandler.HandleGetRegistrationMode)
				}

				// Protected auth endpoints
				r.Group(func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))
					r.Post("/logout", auth.HandleLogout)
					r.Get("/me", auth.HandleMe)
				})
			})

			// User profile and member list endpoints (all protected)
			if deps.UserService != nil {
				users := NewUserHandler(deps.UserService)
				r.Route("/users", func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))

					// Member list endpoints (must be before /{id} to avoid Chi matching "staff" as an id)
					if deps.MemberService != nil {
						members := NewMemberHandler(deps.MemberService)
						r.Get("/", members.HandleList)
						r.Get("/staff", members.HandleStaff)
					}

					r.Get("/{id}", users.HandleGetProfile)
					r.Put("/me/profile", users.HandleUpdateProfile)
					r.Put("/me/password", users.HandleChangePassword)
					r.Post("/me/passkey", users.HandleRegeneratePasskey)
				})
			}

			// Torrent endpoints (all protected)
			if deps.TorrentService != nil {
				torrents := NewTorrentHandler(deps.TorrentService, deps.PeerRepo)
				r.Route("/torrents", func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))
					r.Get("/", torrents.HandleList)
					r.Get("/{id}", torrents.HandleGetByID)
					r.Put("/{id}", torrents.HandleEdit)
					r.Delete("/{id}", torrents.HandleDelete)

					// Capability-gated endpoints
					r.With(mw.RequireCapability("upload")).Post("/", torrents.HandleUpload)
					r.With(mw.RequireCapability("download")).Get("/{id}/download", torrents.HandleDownload)

					// Reseed request endpoints
					r.Post("/{id}/reseed", torrents.HandleRequestReseed)
					r.Get("/{id}/reseed", torrents.HandleGetReseedCount)

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

			// Invite endpoints
			if deps.InviteService != nil {
				invites := NewInviteHandler(deps.InviteService)
				r.Route("/invites", func(r chi.Router) {
					// Public: validate invite token (used by registration page)
					r.Get("/{token}", invites.HandleValidateInvite)

					// Protected endpoints
					r.Group(func(r chi.Router) {
						r.Use(mw.RequireAuth(validator))
						r.Get("/", invites.HandleListInvites)
						r.With(mw.RequireCapability("invite")).Post("/", invites.HandleCreateInvite)
					})
				})
			}

			// Activity log endpoints (visible to all authenticated users)
			if deps.ActivityLogService != nil {
				activityLogs := NewActivityLogHandler(deps.ActivityLogService)
				r.Route("/activity-logs", func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))
					r.Get("/", activityLogs.HandleList)
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
			r.Route("/admin", func(r chi.Router) {
				r.Use(mw.RequireAuth(validator))
				r.Use(mw.RequireAdmin)

				if deps.AdminService != nil {
					admin := NewAdminHandler(deps.AdminService)
					r.Get("/users", admin.HandleListUsers)
					r.Put("/users/{id}", admin.HandleUpdateUser)
					r.Get("/groups", admin.HandleListGroups)
				}

				// Site settings (admin only)
				if deps.SiteSettingsService != nil {
					settingsHandler := NewSiteSettingsHandler(deps.SiteSettingsService)
					r.Get("/settings", settingsHandler.HandleGetAllSettings)
					r.Put("/settings/{key}", settingsHandler.HandleUpdateSetting)
				}

				// Ban management endpoints
				if deps.BanService != nil {
					bans := NewBanHandler(deps.BanService)
					r.Get("/bans/emails", bans.HandleListEmailBans)
					r.Post("/bans/emails", bans.HandleCreateEmailBan)
					r.Delete("/bans/emails/{id}", bans.HandleDeleteEmailBan)
					r.Get("/bans/ips", bans.HandleListIPBans)
					r.Post("/bans/ips", bans.HandleCreateIPBan)
					r.Delete("/bans/ips/{id}", bans.HandleDeleteIPBan)
				}

				if deps.CategoryService != nil {
					catAdmin := NewCategoryAdminHandler(deps.CategoryService)
					r.Get("/categories", catAdmin.HandleListCategories)
					r.Post("/categories", catAdmin.HandleCreateCategory)
					r.Put("/categories/{id}", catAdmin.HandleUpdateCategory)
					r.Delete("/categories/{id}", catAdmin.HandleDeleteCategory)
				}
			})
		}
	})

	return r
}
