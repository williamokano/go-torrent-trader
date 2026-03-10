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
	StatsCache          *service.StatsCache
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
	MessageService      *service.MessageService
	ChatService         *service.ChatService
	WarningService      *service.WarningService
	NewsService         *service.NewsService
	ChatHub             *ChatHub
	PeerRepo             repository.PeerRepository
	UserRepo             repository.UserRepository
	CategoryRepo         repository.CategoryRepository
	TransferHistoryRepo  repository.TransferHistoryRepository
	DashboardRepo        repository.DashboardRepository
	RSSConfig            *RSSConfig
}

// NewRouter creates and configures the Chi router with middleware and routes.
func NewRouter(deps *Deps) chi.Router {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(mw.RequestLogger)
	r.Use(mw.CORS)
	r.Use(chimw.Recoverer)

	// WebSocket endpoint (auth via query param, not middleware).
	// The handler unwraps the ResponseWriter to bypass Recoverer's
	// wrapper that strips http.Hijacker.
	if deps != nil && deps.ChatHub != nil {
		r.Get("/ws/chat", deps.ChatHub.HandleWebSocket)
	}

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
		if deps != nil && deps.StatsCache != nil {
			r.Get("/stats", HandleStats(deps.StatsCache))
		}
		if deps != nil && deps.DB != nil {
			r.Get("/categories", HandleCategories(deps.DB))
		}

		// RSS feed (public, authenticated via passkey query param)
		if deps != nil && deps.TorrentService != nil && deps.UserRepo != nil && deps.RSSConfig != nil {
			rssHandler := NewRSSHandler(deps.TorrentService, deps.UserRepo, *deps.RSSConfig)
			r.Get("/rss", rssHandler.HandleRSS)
		}

		// Public news endpoints (no auth required)
		if deps != nil && deps.NewsService != nil {
			newsHandler := NewNewsHandler(deps.NewsService)
			r.Get("/news", newsHandler.HandleListPublishedNews)
			r.Get("/news/{id}", newsHandler.HandleGetPublishedNews)
		}

		if deps != nil && deps.AuthService != nil {
			auth := NewAuthHandler(deps.AuthService, deps.UserService)
			validator := NewSessionValidatorAdapter(deps.SessionStore)

			authMw := mw.RequireAuth(validator)
			var activityTracker *mw.ActivityTracker
			if deps.UserRepo != nil {
				activityTracker = mw.NewActivityTracker(deps.UserRepo)
			}
			authMiddleware := func(r chi.Router) {
				r.Use(authMw)
				if activityTracker != nil {
					r.Use(activityTracker.Track)
				}
			}

			r.Route("/auth", func(r chi.Router) {
				// Public auth endpoints
				r.Post("/register", auth.HandleRegister)
				r.Post("/login", auth.HandleLogin)
				r.Post("/refresh", auth.HandleRefresh)
				r.Post("/forgot-password", auth.HandleForgotPassword)
				r.Post("/reset-password", auth.HandleResetPassword)
				r.Post("/confirm-email", auth.HandleConfirmEmail)
				r.Post("/resend-confirmation", auth.HandleResendConfirmation)

				// Public registration mode endpoint
				if deps.SiteSettingsService != nil {
					settingsHandler := NewSiteSettingsHandler(deps.SiteSettingsService)
					r.Get("/registration-mode", settingsHandler.HandleGetRegistrationMode)
				}

				// Protected auth endpoints
				r.Group(func(r chi.Router) {
					authMiddleware(r)
					r.Post("/logout", auth.HandleLogout)
					r.Get("/me", auth.HandleMe)
				})
			})

			// User profile and member list endpoints
			if deps.UserService != nil {
				users := NewUserHandler(deps.UserService)
				r.Route("/users", func(r chi.Router) {
					// Create the activity handler once (reused for both public and private endpoints)
					var activity *UserActivityHandler
					if deps.TorrentService != nil && deps.PeerRepo != nil && deps.TransferHistoryRepo != nil {
						activity = NewUserActivityHandler(deps.TorrentService, deps.PeerRepo, deps.TransferHistoryRepo)
					}

					// Public endpoint with optional auth (for anonymous torrent filtering)
					if activity != nil {
						r.With(mw.OptionalAuth(validator)).Get("/{id}/torrents", activity.HandleUserTorrents)
					}

					// All remaining endpoints require auth
					r.Group(func(r chi.Router) {
						authMiddleware(r)

						// Member list endpoints (must be before /{id} to avoid Chi matching "staff" as an id)
						if deps.MemberService != nil {
							members := NewMemberHandler(deps.MemberService)
							r.Get("/", members.HandleList)
							r.Get("/staff", members.HandleStaff)
						}

						r.Get("/{id}", users.HandleGetProfile)

						// User warnings endpoint (owner sees active, staff sees all)
						if deps.WarningService != nil {
							warningHandler := NewWarningHandler(deps.WarningService)
							r.Get("/{id}/warnings", warningHandler.HandleGetUserWarnings)
						}

						// User activity endpoint (seeding/leeching/history — owner + staff only)
						if activity != nil {
							r.Get("/{id}/activity", activity.HandleUserActivity)
						}

						r.Put("/me/profile", users.HandleUpdateProfile)
						r.Put("/me/password", users.HandleChangePassword)
						r.Post("/me/passkey", users.HandleRegeneratePasskey)
					})
				})
			}

			// Torrent endpoints (all protected)
			if deps.TorrentService != nil {
				torrents := NewTorrentHandler(deps.TorrentService, deps.PeerRepo, deps.UserRepo, deps.CategoryRepo)
				r.Route("/torrents", func(r chi.Router) {
					authMiddleware(r)
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
					authMiddleware(r)
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
						authMiddleware(r)
						r.Get("/", invites.HandleListInvites)
						r.With(mw.RequireCapability("invite")).Post("/", invites.HandleCreateInvite)
					})
				})
			}

			// Activity log endpoints (visible to all authenticated users)
			if deps.ActivityLogService != nil {
				activityLogs := NewActivityLogHandler(deps.ActivityLogService)
				r.Route("/activity-logs", func(r chi.Router) {
					authMiddleware(r)
					r.Get("/", activityLogs.HandleList)
				})
			}

			// Message endpoints
			if deps.MessageService != nil {
				messages := NewMessageHandler(deps.MessageService)
				r.Route("/messages", func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))
					r.Post("/", messages.HandleSendMessage)
					r.Get("/inbox", messages.HandleListInbox)
					r.Get("/outbox", messages.HandleListOutbox)
					r.Get("/unread-count", messages.HandleUnreadCount)
					r.Get("/{id}", messages.HandleGetMessage)
					r.Delete("/{id}", messages.HandleDeleteMessage)
				})
			}

			// Chat endpoints
			if deps.ChatService != nil && deps.ChatHub != nil {
				chat := NewChatHandler(deps.ChatService, deps.ChatHub)
				r.Route("/chat", func(r chi.Router) {
					r.Use(mw.RequireAuth(validator))
					r.Get("/history", chat.HandleHistory)
					r.Get("/mute-status", chat.HandleMuteStatus)
					r.Delete("/{id}", chat.HandleDelete)
				})
			}

			// Report endpoints
			if deps.ReportService != nil {
				reports := NewReportHandler(deps.ReportService)
				r.Route("/reports", func(r chi.Router) {
					authMiddleware(r)
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
				authMiddleware(r)

				// Admin-only endpoints
				r.Group(func(r chi.Router) {
					r.Use(mw.RequireAdmin)

					if deps.DashboardRepo != nil && deps.ActivityLogService != nil {
						r.Get("/dashboard", HandleDashboard(deps.DashboardRepo, deps.ActivityLogService))
					}

					if deps.AdminService != nil {
						admin := NewAdminHandler(deps.AdminService)
						r.Get("/users", admin.HandleListUsers)
						r.Put("/users/{id}", admin.HandleUpdateUser)
						r.Put("/users/{id}/reset-password", admin.HandleResetPassword)
						r.Put("/users/{id}/reset-passkey", admin.HandleResetPasskey)
						r.Get("/groups", admin.HandleListGroups)
					}

					// Warning management endpoints
					if deps.WarningService != nil {
						warnings := NewWarningHandler(deps.WarningService)
						r.Post("/warnings", warnings.HandleIssueWarning)
						r.Get("/warnings", warnings.HandleListWarnings)
						r.Post("/warnings/{id}/lift", warnings.HandleLiftWarning)
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

					// News management endpoints
					if deps.NewsService != nil {
						newsAdmin := NewNewsHandler(deps.NewsService)
						r.Post("/news", newsAdmin.HandleAdminCreateNews)
						r.Get("/news", newsAdmin.HandleAdminListNews)
						r.Put("/news/{id}", newsAdmin.HandleAdminUpdateNews)
						r.Delete("/news/{id}", newsAdmin.HandleAdminDeleteNews)
					}
				})

				// Staff-level endpoints (accessible by admins and moderators)
				r.Group(func(r chi.Router) {
					r.Use(mw.RequireStaff)

					if deps.ChatService != nil && deps.ChatHub != nil {
						chatAdmin := NewChatAdminHandler(deps.ChatService, deps.ChatHub)
						r.Get("/chat/mutes", chatAdmin.HandleListActiveMutes)
						r.Delete("/chat/messages/{id}", chatAdmin.HandleDeleteMessage)
						r.Delete("/chat/users/{id}/messages", chatAdmin.HandleDeleteUserMessages)
						r.Post("/chat/users/{id}/mute", chatAdmin.HandleMuteUser)
						r.Delete("/chat/users/{id}/mute", chatAdmin.HandleUnmuteUser)
					}
				})
			})
		}
	})

	return r
}
