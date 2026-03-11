package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// AnnounceCheckInput holds the data needed by CheatDetectionService to evaluate an announce.
type AnnounceCheckInput struct {
	UserID        int64
	Username      string
	TorrentID     int64
	TorrentName   string
	ExistingPeer  *model.Peer
	UploadDelta   int64
	DownloadDelta int64
	ReqLeft       int64
	Leechers      int
	Now           time.Time
}

// CheatDetectionService checks announce data for suspicious patterns and creates flags.
type CheatDetectionService struct {
	flags        repository.CheatFlagRepository
	siteSettings *SiteSettingsService
	eventBus     event.Bus
}

// NewCheatDetectionService creates a new CheatDetectionService.
func NewCheatDetectionService(
	flags repository.CheatFlagRepository,
	siteSettings *SiteSettingsService,
	eventBus event.Bus,
) *CheatDetectionService {
	return &CheatDetectionService{
		flags:        flags,
		siteSettings: siteSettings,
		eventBus:     eventBus,
	}
}

// CheckAnnounce runs all cheat detection checks asynchronously.
// Errors are logged but never block the announce path.
func (s *CheatDetectionService) CheckAnnounce(ctx context.Context, input AnnounceCheckInput) {
	if !s.siteSettings.GetBool(ctx, SettingCheatDetectionEnabled, true) {
		return
	}

	// Run checks in a goroutine to avoid adding latency to the announce path.
	// Use background context since the request context may be cancelled.
	go s.runChecks(context.Background(), input)
}

// runChecks executes all detection checks synchronously.
func (s *CheatDetectionService) runChecks(ctx context.Context, input AnnounceCheckInput) {
	s.checkImpossibleUploadSpeed(ctx, input)
	s.checkUploadNoDownloaders(ctx, input)
	s.checkLeftMismatch(ctx, input)
}

// checkImpossibleUploadSpeed flags upload speeds exceeding the configured maximum.
func (s *CheatDetectionService) checkImpossibleUploadSpeed(ctx context.Context, input AnnounceCheckInput) {
	if input.ExistingPeer == nil || input.UploadDelta <= 0 {
		return
	}

	timeDelta := input.Now.Sub(input.ExistingPeer.LastAnnounce)
	if timeDelta.Seconds() < 30 {
		return
	}

	speedMBs := float64(input.UploadDelta) / timeDelta.Seconds() / (1024 * 1024)
	maxSpeed := s.siteSettings.GetFloat64(ctx, SettingCheatMaxUploadSpeedMBs, 100)

	if speedMBs <= maxSpeed {
		return
	}

	details := map[string]interface{}{
		"upload_delta_bytes": input.UploadDelta,
		"time_delta_secs":   timeDelta.Seconds(),
		"speed_mb_s":        speedMBs,
		"threshold_mb_s":    maxSpeed,
	}

	s.createFlag(ctx, input, model.CheatFlagImpossibleUploadSpeed, details)
}

// checkUploadNoDownloaders flags upload data when the seeder has no leechers.
func (s *CheatDetectionService) checkUploadNoDownloaders(ctx context.Context, input AnnounceCheckInput) {
	if input.ExistingPeer == nil || !input.ExistingPeer.Seeder {
		return
	}

	const minUploadBytes = 1024 * 1024 // 1MB
	if input.UploadDelta < minUploadBytes {
		return
	}

	if input.Leechers > 0 {
		return
	}

	details := map[string]interface{}{
		"upload_delta_bytes": input.UploadDelta,
		"leechers":          input.Leechers,
	}

	s.createFlag(ctx, input, model.CheatFlagUploadNoDownloaders, details)
}

// checkLeftMismatch flags download data that doesn't correspond to a decrease in "left" bytes.
func (s *CheatDetectionService) checkLeftMismatch(ctx context.Context, input AnnounceCheckInput) {
	if input.ExistingPeer == nil || input.DownloadDelta <= 0 {
		return
	}

	const minDownloadBytes = 1024 * 1024 // 1MB
	if input.DownloadDelta < minDownloadBytes {
		return
	}

	// Completed download — left is 0, no mismatch possible.
	if input.ReqLeft == 0 {
		return
	}

	leftDecrease := input.ExistingPeer.LeftBytes - input.ReqLeft
	// Client reset (left went up) — skip to avoid false positives.
	if leftDecrease < 0 {
		return
	}

	tolerancePct := s.siteSettings.GetFloat64(ctx, SettingCheatLeftMismatchTolerancePct, 10)
	// Clamp tolerance to [0, 99] to prevent disabling the check via misconfiguration.
	if tolerancePct < 0 {
		tolerancePct = 0
	}
	if tolerancePct > 99 {
		tolerancePct = 99
	}
	threshold := float64(input.DownloadDelta) * (1 - tolerancePct/100)

	if float64(leftDecrease) >= threshold {
		return
	}

	details := map[string]interface{}{
		"download_delta_bytes": input.DownloadDelta,
		"left_decrease_bytes":  leftDecrease,
		"previous_left":        input.ExistingPeer.LeftBytes,
		"current_left":         input.ReqLeft,
		"tolerance_pct":        tolerancePct,
	}

	s.createFlag(ctx, input, model.CheatFlagLeftMismatch, details)
}

// createFlag persists a cheat flag after checking the cooldown window.
func (s *CheatDetectionService) createFlag(ctx context.Context, input AnnounceCheckInput, flagType string, details map[string]interface{}) {
	cooldownHours := s.siteSettings.GetInt(ctx, SettingCheatFlagCooldownHours, 6)

	recent, err := s.flags.HasRecentUndismissed(ctx, input.UserID, input.TorrentID, flagType, cooldownHours)
	if err != nil {
		slog.Error("cheat detection: failed to check cooldown", "error", err, "user_id", input.UserID, "flag_type", flagType)
		return
	}
	if recent {
		return
	}

	detailsJSON, err := json.Marshal(details)
	if err != nil {
		slog.Error("cheat detection: failed to marshal details", "error", err)
		return
	}

	torrentID := &input.TorrentID
	flag := &model.CheatFlag{
		UserID:    input.UserID,
		TorrentID: torrentID,
		FlagType:  flagType,
		Details:   string(detailsJSON),
	}

	if err := s.flags.Create(ctx, flag); err != nil {
		slog.Error("cheat detection: failed to create flag", "error", err, "user_id", input.UserID, "flag_type", flagType)
		return
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, &event.CheatFlaggedEvent{
			Base:        event.NewBase(event.CheatFlagged, event.Actor{Username: "system"}),
			UserID:      input.UserID,
			Username:    input.Username,
			TorrentID:   torrentID,
			TorrentName: input.TorrentName,
			FlagType:    flagType,
		})
	}
}
