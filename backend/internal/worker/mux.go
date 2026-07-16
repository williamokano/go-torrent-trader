package worker

import "github.com/hibiken/asynq"

// NewMux creates and returns an asynq.ServeMux with all task handlers registered.
// deps provides the repositories and database connection needed by handlers that
// perform real work (e.g. peer cleanup).
func NewMux(deps *WorkerDeps) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskSendEmail, NewSendEmailHandler(deps))
	mux.HandleFunc(TaskCleanupPeers, NewCleanupHandler(deps))
	mux.HandleFunc(TaskRecalcStats, NewRecalcStatsHandler(deps))
	mux.HandleFunc(TaskRatioWarning, NewRatioWarningHandler(deps))
	mux.HandleFunc(TaskMaintenance, NewMaintenanceHandler(deps))
	mux.HandleFunc(TaskBackupDatabase, NewBackupHandler(deps))
	mux.HandleFunc(TaskPromotion, NewPromotionHandler(deps))
	mux.HandleFunc(TaskBonusAward, NewBonusAwardHandler(deps))
	mux.HandleFunc(TaskInviteDistribution, NewInviteDistributionHandler(deps))
	return mux
}
