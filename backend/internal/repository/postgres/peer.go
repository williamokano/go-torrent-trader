package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

const peerColumns = `id, torrent_id, user_id, peer_id, ip, port,
	uploaded, downloaded, left_bytes, seeder, agent, started_at, last_announce`

// PeerRepo implements repository.PeerRepository using PostgreSQL.
type PeerRepo struct {
	db *sql.DB
}

// NewPeerRepo returns a new PostgreSQL-backed PeerRepository.
func NewPeerRepo(db *sql.DB) repository.PeerRepository {
	return &PeerRepo{db: db}
}

func scanPeer(row interface{ Scan(...any) error }) (*model.Peer, error) {
	var p model.Peer
	err := row.Scan(
		&p.ID, &p.TorrentID, &p.UserID, &p.PeerID, &p.IP, &p.Port,
		&p.Uploaded, &p.Downloaded, &p.LeftBytes, &p.Seeder, &p.Agent,
		&p.StartedAt, &p.LastAnnounce,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PeerRepo) GetByTorrentAndUser(ctx context.Context, torrentID, userID int64) (*model.Peer, error) {
	query := fmt.Sprintf("SELECT %s FROM peers WHERE torrent_id = $1 AND user_id = $2", peerColumns)
	return scanPeer(r.db.QueryRowContext(ctx, query, torrentID, userID))
}

func (r *PeerRepo) ListByTorrent(ctx context.Context, torrentID int64, limit int) ([]model.Peer, error) {
	query := fmt.Sprintf("SELECT %s FROM peers WHERE torrent_id = $1 ORDER BY last_announce DESC LIMIT $2", peerColumns)
	rows, err := r.db.QueryContext(ctx, query, torrentID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying peers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var peers []model.Peer
	for rows.Next() {
		p, err := scanPeer(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning peer: %w", err)
		}
		peers = append(peers, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating peers: %w", err)
	}

	return peers, nil
}

func (r *PeerRepo) Upsert(ctx context.Context, peer *model.Peer) error {
	query := `INSERT INTO peers (
		torrent_id, user_id, peer_id, ip, port, uploaded, downloaded,
		left_bytes, seeder, agent, started_at, last_announce
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	ON CONFLICT (torrent_id, user_id, peer_id) DO UPDATE SET
		ip = EXCLUDED.ip,
		port = EXCLUDED.port,
		uploaded = EXCLUDED.uploaded,
		downloaded = EXCLUDED.downloaded,
		left_bytes = EXCLUDED.left_bytes,
		seeder = EXCLUDED.seeder,
		agent = EXCLUDED.agent,
		last_announce = EXCLUDED.last_announce
	RETURNING id, started_at`

	return r.db.QueryRowContext(ctx, query,
		peer.TorrentID, peer.UserID, peer.PeerID, peer.IP, peer.Port,
		peer.Uploaded, peer.Downloaded, peer.LeftBytes, peer.Seeder,
		peer.Agent, peer.StartedAt, peer.LastAnnounce,
	).Scan(&peer.ID, &peer.StartedAt)
}

func (r *PeerRepo) Delete(ctx context.Context, torrentID, userID int64, peerID []byte) error {
	query := `DELETE FROM peers WHERE torrent_id = $1 AND user_id = $2 AND peer_id = $3`
	result, err := r.db.ExecContext(ctx, query, torrentID, userID, peerID)
	if err != nil {
		return fmt.Errorf("deleting peer: %w", err)
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

func (r *PeerRepo) DeleteStale(ctx context.Context, before time.Time) (int64, error) {
	query := `DELETE FROM peers WHERE last_announce < $1`
	result, err := r.db.ExecContext(ctx, query, before)
	if err != nil {
		return 0, fmt.Errorf("deleting stale peers: %w", err)
	}
	return result.RowsAffected()
}
