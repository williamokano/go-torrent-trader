package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/config"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/schema"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/source"
)

// withSource is the shared opening move of every command that reads the legacy
// database: load config, insist on a DSN, connect, and close afterwards.
func withSource(cmd *cobra.Command, fn func(context.Context, *source.DB) error) (err error) {
	cfg, err := config.LoadFromFlags(cmd)
	if err != nil {
		return err
	}
	dsn, err := cfg.RequireSource()
	if err != nil {
		return err
	}

	db, err := source.Open(cmd.Context(), dsn)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing source database: %w", closeErr))
		}
	}()

	return fn(cmd.Context(), db)
}

// withSchema runs fn against the source database's schema.
func withSchema(cmd *cobra.Command, fn func(schema.Schema, string) error) error {
	return withSource(cmd, func(ctx context.Context, db *source.DB) error {
		s, err := db.Schema(ctx)
		if err != nil {
			return err
		}
		return fn(s, db.String())
	})
}
