package service

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrInvalidPasskey      = errors.New("invalid passkey")
	ErrTorrentBanned       = errors.New("torrent is banned")
	ErrUserDisabled        = errors.New("user account is disabled")
	ErrDownloadSuspended   = errors.New("download privilege suspended")
)

const (
	DefaultInterval    = 1800 // seconds
	DefaultMinInterval = 900  // seconds
	MaxPeersPerRequest = 50
)

// AnnounceEvent represents the event parameter in an announce request.
type AnnounceEvent string

const (
	EventStarted   AnnounceEvent = "started"
	EventStopped   AnnounceEvent = "stopped"
	EventCompleted AnnounceEvent = "completed"
	EventEmpty     AnnounceEvent = ""
)

// AnnounceRequest holds the parsed parameters from an announce request.
type AnnounceRequest struct {
	Passkey    string
	InfoHash   []byte
	PeerID     []byte
	IP         string
	Port       int
	Uploaded   int64
	Downloaded int64
	Left       int64
	Event      AnnounceEvent
}

// AnnounceResponse holds the data to be bencoded and returned.
type AnnounceResponse struct {
	Interval    int    `bencode:"interval"`
	MinInterval int    `bencode:"min interval"`
	Complete    int    `bencode:"complete"`
	Incomplete  int    `bencode:"incomplete"`
	Peers       []byte `bencode:"peers"`
}

// ScrapeEntry holds the stats for a single torrent in a scrape response.
type ScrapeEntry struct {
	Complete   int `bencode:"complete"`
	Incomplete int `bencode:"incomplete"`
	Downloaded int `bencode:"downloaded"`
}

// TrackerService handles tracker-related business logic (announce, scrape).
type TrackerService struct {
	users           repository.UserRepository
	torrents        repository.TorrentRepository
	peers           repository.PeerRepository
	transferHistory repository.TransferHistoryRepository
}

// NewTrackerService creates a new TrackerService.
func NewTrackerService(
	users repository.UserRepository,
	torrents repository.TorrentRepository,
	peers repository.PeerRepository,
) *TrackerService {
	return &TrackerService{
		users:    users,
		torrents: torrents,
		peers:    peers,
	}
}

// SetTransferHistoryRepo sets the transfer history repository for recording completions.
func (s *TrackerService) SetTransferHistoryRepo(repo repository.TransferHistoryRepository) {
	s.transferHistory = repo
}

// Announce processes an announce request and returns the response.
func (s *TrackerService) Announce(ctx context.Context, req AnnounceRequest) (*AnnounceResponse, error) {
	// Validate passkey and get user.
	user, err := s.users.GetByPasskey(ctx, req.Passkey)
	if err != nil {
		return nil, ErrInvalidPasskey
	}
	if !user.Enabled {
		return nil, ErrUserDisabled
	}

	// Reject leeching if user's download privilege is suspended.
	if !user.CanDownload && req.Left > 0 {
		return nil, ErrDownloadSuspended
	}

	// Validate torrent exists and is not banned.
	torrent, err := s.torrents.GetByInfoHash(ctx, req.InfoHash)
	if err != nil {
		return nil, ErrTorrentNotFound
	}
	if torrent.Banned {
		return nil, ErrTorrentBanned
	}

	// Look up existing peer by the exact peer_id for delta calculation.
	// A user can have multiple peers (seedbox + home PC), each with a unique peer_id.
	existingPeer, _ := s.peers.GetByTorrentUserAndPeerID(ctx, torrent.ID, user.ID, req.PeerID)

	// Calculate upload/download deltas for user stats.
	var uploadDelta, downloadDelta int64
	if existingPeer != nil {
		uploadDelta = req.Uploaded - existingPeer.Uploaded
		downloadDelta = req.Downloaded - existingPeer.Downloaded
		// Protect against negative deltas (client reset).
		if uploadDelta < 0 {
			uploadDelta = 0
		}
		if downloadDelta < 0 {
			downloadDelta = 0
		}
	} else if req.Event != EventStarted {
		// First time seeing this peer without a started event; use reported values.
		uploadDelta = req.Uploaded
		downloadDelta = req.Downloaded
	}

	isSeeder := req.Left == 0

	switch req.Event {
	case EventStopped:
		if err := s.handleStopped(ctx, torrent, user, existingPeer, req); err != nil {
			slog.Error("failed to handle stopped event", "error", err)
		}
	case EventCompleted:
		if err := s.handleCompleted(ctx, torrent, user, existingPeer, req, isSeeder); err != nil {
			slog.Error("failed to handle completed event", "error", err)
		}
	default:
		// started or regular announce
		if err := s.handleAnnounce(ctx, torrent, user, existingPeer, req, isSeeder); err != nil {
			return nil, fmt.Errorf("handle announce: %w", err)
		}
	}

	// Update user stats if there are any deltas.
	if uploadDelta > 0 || downloadDelta > 0 {
		if err := s.users.IncrementStats(ctx, user.ID, uploadDelta, downloadDelta); err != nil {
			slog.Error("failed to update user stats", "user_id", user.ID, "error", err)
		}
	}

	// Build peer list (exclude the announcing peer).
	peers, err := s.peers.ListByTorrent(ctx, torrent.ID, MaxPeersPerRequest+1)
	if err != nil {
		return nil, fmt.Errorf("list peers: %w", err)
	}

	compactPeers := buildCompactPeerList(peers, req.PeerID, MaxPeersPerRequest)

	return &AnnounceResponse{
		Interval:    DefaultInterval,
		MinInterval: DefaultMinInterval,
		Complete:    torrent.Seeders,
		Incomplete:  torrent.Leechers,
		Peers:       compactPeers,
	}, nil
}

// Scrape returns stats for the given info hashes. Unknown hashes are silently
// omitted from the result map. The map keys are the raw 20-byte info hash strings.
func (s *TrackerService) Scrape(ctx context.Context, infoHashes [][]byte) (map[string]ScrapeEntry, error) {
	result := make(map[string]ScrapeEntry, len(infoHashes))

	for _, ih := range infoHashes {
		torrent, err := s.torrents.GetByInfoHash(ctx, ih)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}

		result[string(ih)] = ScrapeEntry{
			Complete:   torrent.Seeders,
			Incomplete: torrent.Leechers,
			Downloaded: torrent.TimesCompleted,
		}
	}

	return result, nil
}

func (s *TrackerService) handleStopped(
	ctx context.Context,
	torrent *model.Torrent,
	user *model.User,
	existingPeer *model.Peer,
	req AnnounceRequest,
) error {
	if existingPeer == nil {
		return nil
	}

	if err := s.peers.Delete(ctx, torrent.ID, user.ID, req.PeerID); err != nil {
		return fmt.Errorf("delete peer: %w", err)
	}

	if existingPeer.Seeder {
		if err := s.torrents.IncrementSeeders(ctx, torrent.ID, -1); err != nil {
			slog.Error("failed to decrement seeders", "torrent_id", torrent.ID, "error", err)
		}
	} else {
		if err := s.torrents.IncrementLeechers(ctx, torrent.ID, -1); err != nil {
			slog.Error("failed to decrement leechers", "torrent_id", torrent.ID, "error", err)
		}
	}

	return nil
}

func (s *TrackerService) handleCompleted(
	ctx context.Context,
	torrent *model.Torrent,
	user *model.User,
	existingPeer *model.Peer,
	req AnnounceRequest,
	isSeeder bool,
) error {
	clientName := ParsePeerIDClient(req.PeerID)
	now := time.Now()
	peer := &model.Peer{
		TorrentID:    torrent.ID,
		UserID:       user.ID,
		PeerID:       req.PeerID,
		IP:           req.IP,
		Port:         req.Port,
		Uploaded:     req.Uploaded,
		Downloaded:   req.Downloaded,
		LeftBytes:    req.Left,
		Seeder:       isSeeder,
		Agent:        &clientName,
		StartedAt:    now,
		LastAnnounce: now,
	}

	if err := s.peers.Upsert(ctx, peer); err != nil {
		return fmt.Errorf("upsert peer: %w", err)
	}

	if err := s.torrents.IncrementTimesCompleted(ctx, torrent.ID); err != nil {
		slog.Error("failed to increment times_completed", "torrent_id", torrent.ID, "error", err)
	}

	// Record transfer history
	if s.transferHistory != nil {
		torrentID := torrent.ID
		th := &model.TransferHistory{
			UserID:       user.ID,
			TorrentID:    &torrentID,
			Uploaded:     req.Uploaded,
			Downloaded:   req.Downloaded,
			Seeder:       isSeeder,
			CompletedAt:  now,
			LastAnnounce: now,
		}
		if err := s.transferHistory.Upsert(ctx, th); err != nil {
			slog.Error("failed to record transfer history", "torrent_id", torrent.ID, "user_id", user.ID, "error", err)
		}
	}

	// Transition from leecher to seeder if applicable.
	if existingPeer != nil && !existingPeer.Seeder && isSeeder {
		if err := s.torrents.IncrementLeechers(ctx, torrent.ID, -1); err != nil {
			slog.Error("failed to decrement leechers", "torrent_id", torrent.ID, "error", err)
		}
		if err := s.torrents.IncrementSeeders(ctx, torrent.ID, 1); err != nil {
			slog.Error("failed to increment seeders", "torrent_id", torrent.ID, "error", err)
		}
	} else if existingPeer == nil {
		if isSeeder {
			if err := s.torrents.IncrementSeeders(ctx, torrent.ID, 1); err != nil {
				slog.Error("failed to increment seeders", "torrent_id", torrent.ID, "error", err)
			}
		} else {
			if err := s.torrents.IncrementLeechers(ctx, torrent.ID, 1); err != nil {
				slog.Error("failed to increment leechers", "torrent_id", torrent.ID, "error", err)
			}
		}
	}

	return nil
}

func (s *TrackerService) handleAnnounce(
	ctx context.Context,
	torrent *model.Torrent,
	user *model.User,
	existingPeer *model.Peer,
	req AnnounceRequest,
	isSeeder bool,
) error {
	clientName := ParsePeerIDClient(req.PeerID)
	now := time.Now()
	peer := &model.Peer{
		TorrentID:    torrent.ID,
		UserID:       user.ID,
		PeerID:       req.PeerID,
		IP:           req.IP,
		Port:         req.Port,
		Uploaded:     req.Uploaded,
		Downloaded:   req.Downloaded,
		LeftBytes:    req.Left,
		Seeder:       isSeeder,
		Agent:        &clientName,
		StartedAt:    now,
		LastAnnounce: now,
	}

	if err := s.peers.Upsert(ctx, peer); err != nil {
		return fmt.Errorf("upsert peer: %w", err)
	}

	if existingPeer == nil {
		if isSeeder {
			if err := s.torrents.IncrementSeeders(ctx, torrent.ID, 1); err != nil {
				slog.Error("failed to increment seeders", "torrent_id", torrent.ID, "error", err)
			}
		} else {
			if err := s.torrents.IncrementLeechers(ctx, torrent.ID, 1); err != nil {
				slog.Error("failed to increment leechers", "torrent_id", torrent.ID, "error", err)
			}
		}
	} else if existingPeer.Seeder != isSeeder {
		if isSeeder {
			if err := s.torrents.IncrementLeechers(ctx, torrent.ID, -1); err != nil {
				slog.Error("failed to decrement leechers", "torrent_id", torrent.ID, "error", err)
			}
			if err := s.torrents.IncrementSeeders(ctx, torrent.ID, 1); err != nil {
				slog.Error("failed to increment seeders", "torrent_id", torrent.ID, "error", err)
			}
		} else {
			if err := s.torrents.IncrementSeeders(ctx, torrent.ID, -1); err != nil {
				slog.Error("failed to decrement seeders", "torrent_id", torrent.ID, "error", err)
			}
			if err := s.torrents.IncrementLeechers(ctx, torrent.ID, 1); err != nil {
				slog.Error("failed to increment leechers", "torrent_id", torrent.ID, "error", err)
			}
		}
	}

	return nil
}

// buildCompactPeerList creates a BEP 23 compact peer list.
// Each peer is encoded as 6 bytes: 4 bytes IPv4 + 2 bytes port (big-endian).
// The announcing peer is excluded and the list is shuffled for fairness.
func buildCompactPeerList(peers []model.Peer, excludePeerID []byte, maxPeers int) []byte {
	var eligible []model.Peer
	for _, p := range peers {
		if bytesEqual(p.PeerID, excludePeerID) {
			continue
		}
		ip := net.ParseIP(p.IP)
		if ip == nil {
			continue
		}
		if ip.To4() == nil {
			continue
		}
		eligible = append(eligible, p)
	}

	rand.Shuffle(len(eligible), func(i, j int) {
		eligible[i], eligible[j] = eligible[j], eligible[i]
	})

	if len(eligible) > maxPeers {
		eligible = eligible[:maxPeers]
	}

	buf := make([]byte, len(eligible)*6)
	for i, p := range eligible {
		ip := net.ParseIP(p.IP).To4()
		copy(buf[i*6:i*6+4], ip)
		binary.BigEndian.PutUint16(buf[i*6+4:i*6+6], uint16(p.Port))
	}

	return buf
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
