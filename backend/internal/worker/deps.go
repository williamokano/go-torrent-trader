package worker

import (
	"database/sql"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/listener"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// SendToUserFunc is a callback for sending WebSocket messages to a specific user.
// Set by the HTTP server to bridge the worker with the WS hub.
type SendToUserFunc func(userID int64, payload []byte)

// WorkerDeps holds the dependencies required by worker task handlers.
type WorkerDeps struct {
	PeerRepo              repository.PeerRepository
	TorrentRepo           repository.TorrentRepository
	DB                    *sql.DB
	WarningSvc            *service.WarningService
	SiteSettingsSvc       *service.SiteSettingsService
	EmailSender           service.EmailSender
	StatsCache            *service.StatsCache
	ChatSvc               *service.ChatService
	RestrictionSvc        *service.RestrictionService
	AdminSvc              *service.AdminService
	BackupSvc             *service.BackupService
	PromotionSvc          *service.PromotionService
	BonusSvc              *service.BonusService
	InviteDistributionSvc *service.InviteDistributionService
	DigestSvc             *service.NotificationDigestService
	SendToUser            SendToUserFunc

	NotificationRepo repository.NotificationRepository
	// NotificationRetention is how long read notifications are kept before the
	// maintenance job purges them. Zero or negative disables the purge.
	NotificationRetention time.Duration

	// External notification connectors (BE-10). All nil-checked: a deployment
	// without them simply has no drain handler work to do.
	ConnectorRegistry     *connector.Registry
	ConnectorRepo         repository.ConnectorRepository
	ConnectorDeliveryRepo repository.ConnectorDeliveryRepository
	// ConnectorEnqueuer lets the drain handler schedule its own follow-up run
	// (backoff, rate-limit window, leftover batch).
	ConnectorEnqueuer listener.DrainEnqueuer
	// ConnectorDeliveryRetention is how long delivery-log rows are kept.
	// Zero or negative disables pruning.
	ConnectorDeliveryRetention time.Duration
}
