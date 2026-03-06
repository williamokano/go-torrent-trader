package worker

import "github.com/hibiken/asynq"

// NewMux creates and returns an asynq.ServeMux with all task handlers registered.
func NewMux() *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskSendEmail, HandleSendEmail)
	mux.HandleFunc(TaskCleanupPeers, HandleCleanupPeers)
	mux.HandleFunc(TaskRecalcStats, HandleRecalcStats)
	return mux
}
