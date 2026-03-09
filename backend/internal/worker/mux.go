package worker

import "github.com/hibiken/asynq"

// NewMux creates and returns an asynq.ServeMux with all task handlers registered.
// deps provides the repositories and database connection needed by handlers that
// perform real work (e.g. peer cleanup).
func NewMux(deps *WorkerDeps) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskSendEmail, HandleSendEmail)
	mux.HandleFunc(TaskCleanupPeers, NewCleanupHandler(deps))
	mux.HandleFunc(TaskRecalcStats, HandleRecalcStats)
	mux.HandleFunc(TaskRatioWarning, NewRatioWarningHandler(deps))
	mux.HandleFunc(TaskMaintenance, NewMaintenanceHandler(deps))
	return mux
}
