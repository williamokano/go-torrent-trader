package worker

import (
	"context"
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
	// (backoff, rate-limit window, leftover batch) and the maintenance sweep
	// re-enqueue stranded work.
	ConnectorEnqueuer ConnectorDrainEnqueuer
	// ConnectorDeliveryRetention returns how long delivery-log rows are kept.
	// It is a func, not a value, because the retention is an admin-editable site
	// setting: reading it once at boot would make an edit appear to work and
	// silently do nothing until the next restart. Zero or negative disables
	// pruning; a nil func disables it too.
	ConnectorDeliveryRetention func() time.Duration
}

// ConnectorDrainEnqueuer queues connector drain tasks.
//
// The two methods differ only in whether they collapse duplicates: the
// dispatcher wants that (a burst of approvals should produce one drain), a drain
// scheduling its own follow-up must not have it (see NewConnectorDrainTask).
type ConnectorDrainEnqueuer interface {
	listener.DrainEnqueuer
	EnqueueConnectorDrainFollowUp(ctx context.Context, instanceID int64, delay time.Duration) error
}
