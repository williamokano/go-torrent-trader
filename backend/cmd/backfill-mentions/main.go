// Command backfill-mentions is a one-off tool that recomputes
// mentioned_usernames for existing torrent_comments, forum_posts, and
// messages rows created before the @mention feature (or, for messages,
// before PM mention linking) existed. Run once after deploying the
// corresponding migration:
//
//	go run ./cmd/backfill-mentions --dry-run   # preview, no writes
//	go run ./cmd/backfill-mentions              # apply
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/williamokano/go-torrent-trader/backend/internal/config"
	"github.com/williamokano/go-torrent-trader/backend/internal/database"
)

func run() int {
	dryRun := flag.Bool("dry-run", false, "compute and log what would change without writing")
	batchSize := flag.Int("batch-size", 500, "rows to process per batch, per table")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		return 1
	}

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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := checkSchema(ctx, db); err != nil {
		slog.Error("schema check failed — has the mentioned_usernames migration been applied to every table yet?", "error", err)
		return 1
	}

	opts := Options{DryRun: *dryRun, BatchSize: *batchSize}
	if opts.DryRun {
		slog.Info("dry-run mode: no writes will be made")
	}

	stats, err := runBackfill(ctx, db, opts)
	if err != nil {
		slog.Error("backfill failed", "error", err)
		return 1
	}

	var totalScanned, totalChanged int
	for _, ts := range stats.Tables {
		totalScanned += ts.Scanned
		totalChanged += ts.Changed
	}
	slog.Info("backfill finished", "tables", len(stats.Tables), "rows_scanned", totalScanned, "rows_changed", totalChanged, "dry_run", opts.DryRun)

	return 0
}

func main() {
	os.Exit(run())
}
