package model

import "time"

// Backup describes a database backup file stored on disk. Backups are not
// tracked in the database — the backup directory is the source of truth, so a
// restored or manually-copied file shows up automatically.
type Backup struct {
	Name      string    `json:"name"`       // server-generated file name, e.g. backup-20260714T031500Z-a1b2c3d4.dump
	Size      int64     `json:"size"`       // file size in bytes
	CreatedAt time.Time `json:"created_at"` // file modification time (UTC)
}
