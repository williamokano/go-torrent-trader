package worker

import (
	"database/sql"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// WorkerDeps holds the dependencies required by worker task handlers.
type WorkerDeps struct {
	PeerRepo    repository.PeerRepository
	TorrentRepo repository.TorrentRepository
	DB          *sql.DB
}
