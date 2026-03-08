package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/williamokano/go-torrent-trader/backend/internal/config"
	"github.com/williamokano/go-torrent-trader/backend/internal/database"
	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/listener"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository/postgres"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/storage"
	"github.com/williamokano/go-torrent-trader/backend/internal/worker"
)

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func run() int {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		return 1
	}

	logLevel := parseLogLevel(cfg.LogLevel)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	// Connect to PostgreSQL
	connCfg := database.ConnConfig{
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
	}
	db, err := database.Connect(cfg.Database.URL, connCfg)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	slog.Info("connected to database")

	// Run migrations
	if err := database.RunMigrations(db, "migrations"); err != nil {
		slog.Error("failed to run migrations", "error", err)
		return 1
	}
	slog.Info("migrations applied")

	// Build dependencies
	userRepo := postgres.NewUserRepo(db)
	torrentRepo := postgres.NewTorrentRepo(db)
	peerRepo := postgres.NewPeerRepo(db)
	groupRepo := postgres.NewGroupRepo(db)

	sessionStore, err := service.NewSessionStore(service.SessionStoreConfig{
		Type:            cfg.Session.Store,
		RedisURL:        cfg.Redis.URL,
		AccessTokenTTL:  cfg.Session.AccessTokenTTL,
		RefreshTokenTTL: cfg.Session.RefreshTokenTTL,
	})
	if err != nil {
		slog.Error("failed to initialize session store", "error", err)
		return 1
	}
	// Close the session store on shutdown if it implements io.Closer.
	if closer, ok := sessionStore.(interface{ Close() error }); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				slog.Error("failed to close session store", "error", err)
			}
		}()
	}
	slog.Info("session store initialized", "type", cfg.Session.Store)

	passwordResetStore := postgres.NewPasswordResetRepo(db)

	// Event bus
	eventBus := event.NewInMemoryBus()

	emailSender := service.NewSMTPSender(cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.From)
	authService := service.NewAuthServiceWithTTL(userRepo, sessionStore, passwordResetStore, emailSender, cfg.Site.BaseURL, cfg.Session.AccessTokenTTL, cfg.Session.RefreshTokenTTL, groupRepo, eventBus)
	userService := service.NewUserService(userRepo, sessionStore, groupRepo, peerRepo, torrentRepo)
	trackerService := service.NewTrackerService(userRepo, torrentRepo, peerRepo)

	// File storage
	fileStore, err := storage.New(cfg.Storage)
	if err != nil {
		slog.Error("failed to initialize file storage", "error", err)
		return 1
	}

	reseedRequestRepo := postgres.NewReseedRequestRepo(db)
	torrentService := service.NewTorrentService(db, torrentRepo, userRepo, fileStore, service.TorrentServiceConfig{
		AnnounceURL:      fmt.Sprintf("%s/announce", cfg.Site.ApiURL),
		TorrentComment:   cfg.Site.BaseURL,
		TorrentCreatedBy: cfg.Site.Name,
	}, eventBus, reseedRequestRepo)

	reportRepo := postgres.NewReportRepo(db)
	reportService := service.NewReportService(reportRepo, eventBus)

	commentRepo := postgres.NewCommentRepo(db)
	ratingRepo := postgres.NewRatingRepo(db)
	commentService := service.NewCommentService(commentRepo, ratingRepo, torrentRepo, eventBus)

	messageRepo := postgres.NewMessageRepo(db)
	messageService := service.NewMessageService(messageRepo, userRepo, eventBus)

	inviteRepo := postgres.NewInviteRepo(db)
	inviteService := service.NewInviteService(inviteRepo, userRepo, eventBus)

	siteSettingsRepo := postgres.NewSiteSettingsRepo(db)
	siteSettingsService := service.NewSiteSettingsService(siteSettingsRepo, eventBus)

	// Wire site settings + invite service into auth service for registration mode checks
	authService.SetSiteSettings(siteSettingsService)
	authService.SetInviteService(inviteService)

	// Activity log — register event listeners
	activityLogRepo := postgres.NewActivityLogRepo(db)
	activityLogService := service.NewActivityLogService(activityLogRepo)
	listener.RegisterActivityLogListeners(eventBus, activityLogService)
	listener.RegisterReseedEmailListener(eventBus, emailSender, cfg.Site.BaseURL)

	banRepo := postgres.NewBanRepo(db)
	banService := service.NewBanService(banRepo, eventBus)
	authService.SetBanChecker(banService)

	chatMessageRepo := postgres.NewChatMessageRepo(db)
	chatService := service.NewChatService(chatMessageRepo, userRepo, eventBus)

	adminService := service.NewAdminService(userRepo, groupRepo, eventBus)
	categoryRepo := postgres.NewCategoryRepo(db)
	categoryService := service.NewCategoryService(categoryRepo)
	memberService := service.NewMemberService(userRepo, groupRepo)

	chatHub := handler.NewChatHub(chatService, sessionStore, []string{cfg.Site.BaseURL})
	go chatHub.Run()

	deps := &handler.Deps{
		DB:             db,
		AuthService:    authService,
		SessionStore:   sessionStore,
		UserService:    userService,
		MemberService:  memberService,
		TorrentService: torrentService,
		TrackerService: trackerService,
		ReportService:      reportService,
		CommentService:     commentService,
		InviteService:       inviteService,
		AdminService:        adminService,
		CategoryService:     categoryService,
		ActivityLogService:  activityLogService,
		SiteSettingsService: siteSettingsService,
		BanService:          banService,
		MessageService:      messageService,
		ChatService:        chatService,
		ChatHub:            chatHub,
		PeerRepo:           peerRepo,
		UserRepo:           userRepo,
		CategoryRepo:       categoryRepo,
		RSSConfig: &handler.RSSConfig{
			SiteName: cfg.Site.Name,
			BaseURL:  cfg.Site.BaseURL,
			ApiURL:   cfg.Site.ApiURL,
		},
	}

	// Start background worker (asynq server + scheduler)
	workerDeps := &worker.WorkerDeps{
		PeerRepo:    peerRepo,
		TorrentRepo: torrentRepo,
		DB:          db,
	}

	workerSrv, err := worker.NewServer(cfg.Redis.URL, 10)
	if err != nil {
		slog.Error("failed to create worker server", "error", err)
		return 1
	}

	workerMux := worker.NewMux(workerDeps)
	go func() {
		if err := workerSrv.Run(workerMux); err != nil {
			slog.Error("worker server error", "error", err)
		}
	}()
	slog.Info("worker server started")

	var scheduler *asynq.Scheduler
	if cfg.Worker.EnableScheduler {
		scheduler, err = worker.NewScheduler(cfg.Redis.URL)
		if err != nil {
			slog.Error("failed to create scheduler", "error", err)
			return 1
		}

		if err := worker.RegisterPeriodicTasks(scheduler); err != nil {
			slog.Error("failed to register periodic tasks", "error", err)
			return 1
		}

		go func() {
			if err := scheduler.Run(); err != nil {
				slog.Error("scheduler error", "error", err)
			}
		}()
		slog.Info("scheduler started")
	} else {
		slog.Info("scheduler disabled via ENABLE_SCHEDULER=false")
	}

	// Start HTTP server
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: handler.NewRouter(deps),
	}

	slog.Info("server starting", "addr", addr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			return 1
		}
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("http shutdown error", "error", err)
		}
		workerSrv.Shutdown()
		if scheduler != nil {
			scheduler.Shutdown()
		}
	}

	slog.Info("server stopped")
	return 0
}

func main() {
	os.Exit(run())
}
