package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel, AddSource: true}))
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
	trackerService.SetGroupRepo(groupRepo)

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
	trackerService.SetSiteSettings(siteSettingsService)

	cheatFlagRepo := postgres.NewCheatFlagRepo(db)
	cheatDetectionService := service.NewCheatDetectionService(cheatFlagRepo, siteSettingsService, eventBus)
	trackerService.SetCheatDetection(cheatDetectionService)

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

	// Warning escalation listener — auto-restrict or ban based on manual warning count.
	listener.RegisterWarningEscalationListener(eventBus, siteSettingsService, warningRepo, restrictionService, userRepo, activityLogService, sessionStore, messageRepo)

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

	forumCategoryRepo := postgres.NewForumCategoryRepo(db)
	forumRepo := postgres.NewForumRepo(db)
	forumTopicRepo := postgres.NewForumTopicRepo(db)
	forumPostRepo := postgres.NewForumPostRepo(db)
	forumService := service.NewForumService(db, forumCategoryRepo, forumRepo, forumTopicRepo, forumPostRepo, userRepo, groupRepo, eventBus)

	// Database backups (pg_dump) — admin-triggered and optionally scheduled.
	// A misconfigured backup directory (or a DSN pg_dump cannot be pointed at,
	// e.g. libpq keyword form) disables backups; it must not take the site down.
	// The router and worker both skip the feature when the service is nil.
	backupService, err := service.NewBackupService(service.BackupServiceConfig{
		DatabaseURL: cfg.Database.URL,
		Dir:         cfg.Backup.Dir,
		PgDumpPath:  cfg.Backup.PgDumpPath,
		Timeout:     cfg.Backup.Timeout,
		Retention:   cfg.Backup.Retention,
	}, eventBus)
	if err != nil {
		slog.Warn("database backups disabled", "error", err)
		backupService = nil
	} else {
		slog.Info("backup service initialized", "dir", backupService.Dir(), "retention", cfg.Backup.Retention)
	}

	chatHub := handler.NewChatHub(chatService, sessionStore, siteSettingsService, eventBus, []string{cfg.Site.BaseURL})
	go chatHub.Run()

	// Wire PM notification listener — pushes real-time unread count via WebSocket.
	listener.RegisterPMNotificationListener(eventBus, messageRepo, chatHub.SendToUser)

	// Notification system
	notificationRepo := postgres.NewNotificationRepo(db)
	notificationPrefRepo := postgres.NewNotificationPreferenceRepo(db)
	topicSubscriptionRepo := postgres.NewTopicSubscriptionRepo(db)
	notificationService := service.NewNotificationService(notificationRepo, notificationPrefRepo, topicSubscriptionRepo, forumTopicRepo, forumRepo, chatHub.SendToUser)
	listener.RegisterNotificationListeners(eventBus, notificationService, userRepo, forumPostRepo, topicSubscriptionRepo)

	deps := &handler.Deps{
		DB:                  db,
		StatsCache:          statsCache,
		AuthService:         authService,
		SessionStore:        sessionStore,
		UserService:         userService,
		MemberService:       memberService,
		TorrentService:      torrentService,
		TrackerService:      trackerService,
		ReportService:       reportService,
		CommentService:      commentService,
		InviteService:       inviteService,
		AdminService:        adminService,
		CategoryService:     categoryService,
		ActivityLogService:  activityLogService,
		SiteSettingsService: siteSettingsService,
		BanService:          banService,
		MessageService:      messageService,
		WarningService:      warningService,
		NewsService:         newsService,
		RestrictionService:  restrictionService,
		ForumService:        forumService,
		ChatService:         chatService,
		ChatHub:             chatHub,
		PeerRepo:            peerRepo,
		UserRepo:            userRepo,
		CategoryRepo:        categoryRepo,
		TransferHistoryRepo: transferHistoryRepo,
		DashboardRepo:       dashboardRepo,
		CheatFlagRepo:       cheatFlagRepo,
		NotificationService: notificationService,
		RSSConfig: &handler.RSSConfig{
			SiteName: cfg.Site.Name,
			BaseURL:  cfg.Site.BaseURL,
			ApiURL:   cfg.Site.ApiURL,
		},
	}

	// Assigned only when backups are available: putting a typed nil pointer in
	// the BackupManager interface would make the router's nil check pass and the
	// handlers panic on first use.
	if backupService != nil {
		deps.BackupService = backupService
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
		BackupSvc:       backupService,
		SendToUser:      chatHub.SendToUser,

		NotificationRepo:      notificationRepo,
		NotificationRetention: cfg.Worker.NotificationRetention,
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

		if cfg.Backup.ScheduleCron != "" && backupService != nil {
			if err := worker.RegisterBackupTask(scheduler, cfg.Backup.ScheduleCron); err != nil {
				slog.Error("failed to register scheduled backup", "error", err)
				return 1
			}
			slog.Info("scheduled backups enabled", "cron", cfg.Backup.ScheduleCron)
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
