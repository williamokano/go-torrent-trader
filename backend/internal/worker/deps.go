package worker

import (
	"database/sql"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// WorkerDeps holds the dependencies required by worker task handlers.
type WorkerDeps struct {
	PeerRepo        repository.PeerRepository
	TorrentRepo     repository.TorrentRepository
	DB              *sql.DB
	WarningSvc      *service.WarningService
	SiteSettingsSvc *service.SiteSettingsService
	EmailSender     service.EmailSender
	StatsCache      *service.StatsCache
	ChatSvc         *service.ChatService
}
