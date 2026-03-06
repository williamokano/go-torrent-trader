package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/config"
	"github.com/williamokano/go-torrent-trader/backend/internal/database"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository/postgres"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/storage"
)

func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		return 1
	}

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

	emailSender := service.NewSMTPSender(cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.From)
	authService := service.NewAuthServiceWithTTL(userRepo, sessionStore, emailSender, cfg.Site.BaseURL, cfg.Session.AccessTokenTTL, cfg.Session.RefreshTokenTTL)
	userService := service.NewUserService(userRepo, sessionStore)
	trackerService := service.NewTrackerService(userRepo, torrentRepo, peerRepo)

	// File storage
	fileStore, err := storage.New(cfg.Storage)
	if err != nil {
		slog.Error("failed to initialize file storage", "error", err)
		return 1
	}

	torrentService := service.NewTorrentService(torrentRepo, userRepo, fileStore, service.TorrentServiceConfig{
		AnnounceURL:      fmt.Sprintf("%s/announce", cfg.Site.ApiURL),
		TorrentComment:   cfg.Site.BaseURL,
		TorrentCreatedBy: cfg.Site.Name,
	})

	reportRepo := postgres.NewReportRepo(db)
	reportService := service.NewReportService(reportRepo)

	deps := &handler.Deps{
		DB:             db,
		AuthService:    authService,
		SessionStore:   sessionStore,
		UserService:    userService,
		TorrentService: torrentService,
		TrackerService: trackerService,
		ReportService:  reportService,
	}

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
		slog.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "error", err)
			return 1
		}
	}

	slog.Info("server stopped")
	return 0
}

func main() {
	os.Exit(run())
}
