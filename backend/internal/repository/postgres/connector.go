package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

const connectorColumns = `id, kind, name, enabled, config, filters, created_at, updated_at`

// ConnectorRepo implements repository.ConnectorRepository using PostgreSQL.
type ConnectorRepo struct {
	db *sql.DB
}

// NewConnectorRepo returns a new PostgreSQL-backed ConnectorRepository.
func NewConnectorRepo(db *sql.DB) repository.ConnectorRepository {
	return &ConnectorRepo{db: db}
}

func (r *ConnectorRepo) Create(ctx context.Context, c *model.NotificationConnector) error {
	query := `INSERT INTO notification_connectors (kind, name, enabled, config, filters)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	if err := r.db.QueryRowContext(ctx, query,
		c.Kind, c.Name, c.Enabled, jsonbOrEmpty(c.Config), jsonbOrEmpty(c.Filters),
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return fmt.Errorf("create connector: %w", err)
	}
	return nil
}

func (r *ConnectorRepo) GetByID(ctx context.Context, id int64) (*model.NotificationConnector, error) {
	query := fmt.Sprintf(`SELECT %s FROM notification_connectors WHERE id = $1`, connectorColumns)
	return scanConnector(r.db.QueryRowContext(ctx, query, id))
}

func (r *ConnectorRepo) List(ctx context.Context) ([]model.NotificationConnector, error) {
	query := fmt.Sprintf(`SELECT %s FROM notification_connectors ORDER BY id`, connectorColumns)
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list connectors: %w", err)
	}
	return scanConnectors(rows)
}

func (r *ConnectorRepo) ListEnabled(ctx context.Context) ([]model.NotificationConnector, error) {
	query := fmt.Sprintf(`SELECT %s FROM notification_connectors WHERE enabled ORDER BY id`, connectorColumns)
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list enabled connectors: %w", err)
	}
	return scanConnectors(rows)
}

func (r *ConnectorRepo) Update(ctx context.Context, c *model.NotificationConnector) error {
	// kind is deliberately absent from the SET list: changing it would reinterpret
	// the stored config under a different connector. The service rejects it too.
	query := `UPDATE notification_connectors
		SET name = $1, enabled = $2, config = $3, filters = $4, updated_at = NOW()
		WHERE id = $5
		RETURNING updated_at`

	if err := r.db.QueryRowContext(ctx, query,
		c.Name, c.Enabled, jsonbOrEmpty(c.Config), jsonbOrEmpty(c.Filters), c.ID,
	).Scan(&c.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return sql.ErrNoRows
		}
		return fmt.Errorf("update connector: %w", err)
	}
	return nil
}

func (r *ConnectorRepo) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM notification_connectors WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete connector: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ConnectorRepo) CountByKind(ctx context.Context, kind string) (int64, error) {
	var count int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notification_connectors WHERE kind = $1`, kind,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count connectors by kind: %w", err)
	}
	return count, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanConnector(row rowScanner) (*model.NotificationConnector, error) {
	var c model.NotificationConnector
	if err := row.Scan(&c.ID, &c.Kind, &c.Name, &c.Enabled, &c.Config, &c.Filters,
		&c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("scan connector: %w", err)
	}
	return &c, nil
}

func scanConnectors(rows *sql.Rows) ([]model.NotificationConnector, error) {
	defer func() { _ = rows.Close() }()

	var connectors []model.NotificationConnector
	for rows.Next() {
		var c model.NotificationConnector
		if err := rows.Scan(&c.ID, &c.Kind, &c.Name, &c.Enabled, &c.Config, &c.Filters,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan connector: %w", err)
		}
		connectors = append(connectors, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connectors: %w", err)
	}
	return connectors, nil
}

// jsonbOrEmpty keeps a nil/empty RawMessage from reaching a NOT NULL JSONB
// column as SQL NULL.
func jsonbOrEmpty(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}
