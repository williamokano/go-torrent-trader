package worker

import "github.com/riverqueue/river"

// SendEmailArgs is the job that sends one email. The JSON field names are the
// same ones the asynq payload used.
type SendEmailArgs struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (SendEmailArgs) Kind() string { return TaskSendEmail }

func (SendEmailArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 4} }

// CleanupPeersArgs is the job to clean stale peers.
type CleanupPeersArgs struct{}

func (CleanupPeersArgs) Kind() string { return TaskCleanupPeers }

func (CleanupPeersArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 2} }
