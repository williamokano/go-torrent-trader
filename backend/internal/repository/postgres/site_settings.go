package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// SiteSettingsRepo implements repository.SiteSettingsRepository using PostgreSQL.
type SiteSettingsRepo struct {
	db *sql.DB
}

// NewSiteSettingsRepo returns a new PostgreSQL-backed SiteSettingsRepository.
func NewSiteSettingsRepo(db *sql.DB) repository.SiteSettingsRepository {
	return &SiteSettingsRepo{db: db}
}

func (r *SiteSettingsRepo) Get(ctx context.Context, key string) (*model.SiteSetting, error) {
	var s model.SiteSetting
	query := `SELECT key, value, updated_at FROM site_settings WHERE key = $1`
	err := r.db.QueryRowContext(ctx, query, key).Scan(&s.Key, &s.Value, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get site setting %q: %w", key, err)
	}
	return &s, nil
}

func (r *SiteSettingsRepo) Set(ctx context.Context, key, value string) error {
	query := `INSERT INTO site_settings (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
	_, err := r.db.ExecContext(ctx, query, key, value)
	if err != nil {
		return fmt.Errorf("set site setting %q: %w", key, err)
	}
	return nil
}

func (r *SiteSettingsRepo) GetAll(ctx context.Context) ([]model.SiteSetting, error) {
	query := `SELECT key, value, updated_at FROM site_settings ORDER BY key`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list site settings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var settings []model.SiteSetting
	for rows.Next() {
		var s model.SiteSetting
		if err := rows.Scan(&s.Key, &s.Value, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan site setting: %w", err)
		}
		settings = append(settings, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate site settings: %w", err)
	}
	return settings, nil
}
