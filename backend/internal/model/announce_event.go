package model

import "time"

// AnnounceEvent is an immutable, append-only record of a single tracker
// announce. Unlike the peers table (overwritten each announce) and
// transfer_history (one overwritten row per user+torrent), this log preserves
// every announce so that time-windowed aggregates — monthly upload totals and
// bonus reconciliation — can be computed after the fact. Those are impossible to
// recover from the overwriting projections because the per-announce deltas are
// destroyed at write time.
//
// There is deliberately no IP address here. The tracker needs a peer's address to
// build the peer list, and the peers table holds it for exactly as long as the peer
// is in the swarm; keeping a second copy for months bought nothing, because nothing
// ever read it. See migration 080.
type AnnounceEvent struct {
	ID        int64
	UserID    int64
	TorrentID *int64 // nullable — set to NULL when the torrent is deleted
	PeerID    []byte
	Port      int
	Event     string // started, stopped, completed, or announce

	// Client-reported cumulative totals for the current session.
	Uploaded   int64
	Downloaded int64
	LeftBytes  int64

	// Deltas since this peer's previous announce.
	UploadedDelta          int64 // bytes uploaded since last announce
	DownloadedDelta        int64 // raw, before any freeleech discount
	CountedDownloadedDelta int64 // after freeleech discount — what counted toward ratio

	Seeder      bool
	AnnouncedAt time.Time
}
