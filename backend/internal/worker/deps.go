package worker

import (
	"database/sql"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// SendToUserFunc is a callback for sending WebSocket messages to a specific user.
// Set by the HTTP server to bridge the worker with the WS hub.
type SendToUserFunc func(userID int64, payload []byte)

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
	SendToUser      SendToUserFunc
}
