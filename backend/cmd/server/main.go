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
	"github.com/redis/go-redis/v9"
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

	// Shared Redis client used by both session store and stats cache.
	redisOpts, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		slog.Error("failed to parse redis URL", "error", err)
		return 1
	}
	redisClient := redis.NewClient(redisOpts)
	defer func() { _ = redisClient.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		slog.Error("failed to ping redis", "error", err)
		return 1
	}
	slog.Info("redis connected")

	sessionStore := service.NewSessionStoreWithClient(redisClient, cfg.Session.AccessTokenTTL, cfg.Session.RefreshTokenTTL)
	slog.Info("session store initialized", "type", cfg.Session.Store)

	statsCache := service.NewStatsCache(db, redisClient, cfg.Cache.StatsTTL)
	slog.Info("stats cache initialized", "ttl", cfg.Cache.StatsTTL)

	passwordResetStore := postgres.NewPasswordResetRepo(db)

	// Event bus
	eventBus := event.NewInMemoryBus()

	emailSender := service.NewSMTPSender(cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.From)
	authService := service.NewAuthServiceWithTTL(userRepo, sessionStore, passwordResetStore, emailSender, cfg.Site.BaseURL, cfg.Session.AccessTokenTTL, cfg.Session.RefreshTokenTTL, groupRepo, eventBus)

	// Background email sending via asynq
	asynqClient, err := worker.NewClient(cfg.Redis.URL)
	if err != nil {
		slog.Error("failed to create asynq client", "error", err)
		return 1
	}
	defer func() { _ = asynqClient.Close() }()
	authService.SetTaskEnqueuer(worker.NewAsynqEmailEnqueuer(asynqClient))

	// Email confirmation
	emailConfirmRepo := postgres.NewEmailConfirmationRepo(db)
	authService.SetEmailConfirmationStore(emailConfirmRepo)
	authService.SetRequireEmailConfirm(cfg.Site.RegistrationEmailConfirm)
	authService.SetSiteName(cfg.Site.Name)
	userService := service.NewUserService(userRepo, sessionStore, groupRepo, peerRepo, torrentRepo)
	transferHistoryRepo := postgres.NewTransferHistoryRepo(db)
	trackerService := service.NewTrackerService(userRepo, torrentRepo, peerRepo)
	trackerService.SetTransferHistoryRepo(transferHistoryRepo)

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
	reportService := service.NewReportService(reportRepo, torrentRepo, userRepo, eventBus)

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
	listener.RegisterActivityLogListeners(eventBus, activityLogService, userRepo)
	listener.RegisterReseedEmailListener(eventBus, emailSender, cfg.Site.BaseURL)

	banRepo := postgres.NewBanRepo(db)
	banService := service.NewBanService(banRepo, eventBus)
	authService.SetBanChecker(banService)

	chatMessageRepo := postgres.NewChatMessageRepo(db)
	chatMuteRepo := postgres.NewChatMuteRepo(db)
	chatService := service.NewChatService(chatMessageRepo, chatMuteRepo, userRepo, eventBus)

	warningRepo := postgres.NewWarningRepo(db)
	warningService := service.NewWarningService(warningRepo, userRepo, messageRepo, eventBus)

	newsRepo := postgres.NewNewsRepo(db)
	newsService := service.NewNewsService(newsRepo, userRepo, eventBus)

	restrictionRepo := postgres.NewRestrictionRepo(db)
	restrictionService := service.NewRestrictionService(restrictionRepo, userRepo, eventBus)

	adminService := service.NewAdminService(userRepo, groupRepo, eventBus)
	adminService.SetSessionStore(sessionStore)
	adminService.SetEmailSender(emailSender)
	modNoteRepo := postgres.NewModNoteRepo(db)
	adminService.SetModNoteRepo(modNoteRepo)
	adminService.SetTorrentRepo(torrentRepo)
	adminService.SetWarningRepo(warningRepo)
	adminService.SetMessageRepo(messageRepo)
	adminService.SetBanService(banService)

	reportService.SetWarningService(warningService)
	reportService.SetTorrentService(torrentService)

	categoryRepo := postgres.NewCategoryRepo(db)
	categoryService := service.NewCategoryService(categoryRepo)
	memberService := service.NewMemberService(userRepo, groupRepo)
	dashboardRepo := postgres.NewDashboardRepo(db)

	chatHub := handler.NewChatHub(chatService, sessionStore, siteSettingsService, eventBus, []string{cfg.Site.BaseURL})
	go chatHub.Run()

	// Wire PM notification listener — pushes real-time unread count via WebSocket.
	listener.RegisterPMNotificationListener(eventBus, messageRepo, chatHub.SendToUser)

	deps := &handler.Deps{
		DB:             db,
		StatsCache:     statsCache,
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
		WarningService:     warningService,
		NewsService:        newsService,
		RestrictionService: restrictionService,
		ChatService:        chatService,
		ChatHub:            chatHub,
		PeerRepo:            peerRepo,
		UserRepo:            userRepo,
		CategoryRepo:        categoryRepo,
		TransferHistoryRepo: transferHistoryRepo,
		DashboardRepo:       dashboardRepo,
		RSSConfig: &handler.RSSConfig{
			SiteName: cfg.Site.Name,
			BaseURL:  cfg.Site.BaseURL,
			ApiURL:   cfg.Site.ApiURL,
		},
	}

	// Start background worker (asynq server + scheduler)
	workerDeps := &worker.WorkerDeps{
		PeerRepo:        peerRepo,
		TorrentRepo:     torrentRepo,
		DB:              db,
		WarningSvc:      warningService,
		SiteSettingsSvc: siteSettingsService,
		EmailSender:     emailSender,
		StatsCache:      statsCache,
		ChatSvc:         chatService,
		RestrictionSvc:  restrictionService,
		AdminSvc:        adminService,
		SendToUser:      chatHub.SendToUser,
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
