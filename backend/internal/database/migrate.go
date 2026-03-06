package database

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/pressly/goose/v3"
)

// RunMigrations applies all pending SQL migrations from the given directory.
// It uses goose with the "postgres" dialect.
func RunMigrations(db *sql.DB, migrationsDir string) error {
	goose.SetBaseFS(os.DirFS(migrationsDir))

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("setting goose dialect: %w", err)
	}

	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
