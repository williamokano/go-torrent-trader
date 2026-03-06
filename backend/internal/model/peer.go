package model

import "time"

// Peer represents an active peer in a torrent swarm.
type Peer struct {
	ID           int64
	TorrentID    int64
	UserID       int64
	PeerID       []byte
	IP           string
	Port         int
	Uploaded     int64
	Downloaded   int64
	LeftBytes    int64
	Seeder       bool
	Agent        *string
	StartedAt    time.Time
	LastAnnounce time.Time
}
