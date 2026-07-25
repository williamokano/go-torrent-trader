package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// ErrInvalidSetting marks a rejected value, as opposed to a storage failure.
// Without it the caller cannot tell "you sent a bad value" from "the database
// is down", and has to blanket both as one status.
var ErrInvalidSetting = errors.New("invalid setting value")

const (
	// SettingRegistrationMode controls whether registration is open or invite-only.
	SettingRegistrationMode = "registration_mode"

	// RegistrationModeOpen allows anyone to register.
	RegistrationModeOpen = "open"

	// RegistrationModeInviteOnly requires an invite code to register.
	RegistrationModeInviteOnly = "invite_only"

	// Chat anti-spam settings keys.
	SettingChatRateLimitWindow    = "chat_rate_limit_window"
	SettingChatRateLimitMax       = "chat_rate_limit_max"
	SettingChatSpamStrikeCount    = "chat_spam_strike_count"
	SettingChatSpamMuteMinutes    = "chat_spam_mute_minutes"
	SettingChatStrikeResetSeconds = "chat_strike_reset_seconds"
	SettingChatRateLimitMessage   = "chat_rate_limit_message"
	SettingChatSpamMuteMessage    = "chat_spam_mute_message"

	// Tracker connection limit settings keys.
	SettingTrackerMaxPeersPerTorrent = "tracker_max_peers_per_torrent"
	SettingTrackerMaxPeersPerUser    = "tracker_max_peers_per_user"

	// Warning escalation settings keys.
	SettingWarningEscalationEnabled = "warning_escalation_enabled"
	SettingWarningCountRestrict     = "warning_count_restrict"
	SettingWarningCountBan          = "warning_count_ban"
	SettingWarningRestrictType      = "warning_restrict_type"
	SettingWarningRestrictDays      = "warning_restrict_days"

	// Wait time settings keys.
	SettingWaitTimeEnabled     = "wait_time_enabled"
	SettingWaitTimeBypassRatio = "wait_time_bypass_ratio"
	SettingWaitTimeTiers       = "wait_time_tiers"

	// Cheat detection settings keys.
	SettingCheatDetectionEnabled         = "cheat_detection_enabled"
	SettingCheatMaxUploadSpeedMBs        = "cheat_max_upload_speed_mb_s"
	SettingCheatLeftMismatchTolerancePct = "cheat_left_mismatch_tolerance_pct"
	SettingCheatFlagCooldownHours        = "cheat_flag_cooldown_hours"

	// Announce event log settings keys.
	SettingAnnounceLogEnabled       = "announce_log_enabled"
	SettingAnnounceLogRetentionDays = "announce_log_retention_days"

	// Auto class promotion settings keys.
	SettingPromotionEnabled        = "promotion_enabled"
	SettingPromotionIntervalDays   = "promotion_interval_days"
	SettingPromotionSeedWindowDays = "promotion_seed_window_days"

	// Auto invite distribution settings keys.
	SettingInviteDistributionEnabled      = "invite_distribution_enabled"
	SettingInviteDistributionIntervalDays = "invite_distribution_interval_days"

	// Bonus point economy settings keys.
	SettingBonusEnabled                 = "bonus_enabled"
	SettingBonusPointsPerSeedingTorrent = "bonus_points_per_seeding_torrent"

	// Torrent submission moderation settings keys (BE-8.22).
	// SettingModerationEnabled is the master switch; when false, uploads
	// auto-approve (legacy behavior). SettingModerationPublicVisibility controls
	// whether a non-author/non-staff may view a pending torrent's detail page — it
	// never unlocks download.
	SettingModerationEnabled          = "moderation_enabled"
	SettingModerationPublicVisibility = "moderation_public_visibility"

	// External notification connector settings keys (BE-10).
	// SettingConnectorsEnabled is the global kill-switch: false silences every
	// connector kind without touching per-instance enabled flags.
	// SettingConnectorsAllowPrivateNetworks relaxes the outbound SSRF guard so a
	// self-hosted receiver on the LAN can be targeted.
	SettingConnectorsEnabled              = "connectors_enabled"
	SettingConnectorDeliveryRetentionDays = "connector_delivery_retention_days"
	SettingConnectorsAllowPrivateNetworks = "connectors_allow_private_networks"
)

// SiteSettingsService handles site settings business logic.
type SiteSettingsService struct {
	settings repository.SiteSettingsRepository
	eventBus event.Bus
}

// NewSiteSettingsService creates a new SiteSettingsService.
func NewSiteSettingsService(settings repository.SiteSettingsRepository, bus event.Bus) *SiteSettingsService {
	return &SiteSettingsService{settings: settings, eventBus: bus}
}

// GetRegistrationMode returns the current registration mode (defaults to invite_only).
func (s *SiteSettingsService) GetRegistrationMode(ctx context.Context) string {
	setting, err := s.settings.Get(ctx, SettingRegistrationMode)
	if err != nil || setting == nil {
		return RegistrationModeInviteOnly
	}
	if setting.Value != RegistrationModeOpen && setting.Value != RegistrationModeInviteOnly {
		return RegistrationModeInviteOnly
	}
	return setting.Value
}

// GetAll returns all site settings.
func (s *SiteSettingsService) GetAll(ctx context.Context) ([]model.SiteSetting, error) {
	return s.settings.GetAll(ctx)
}

// Set updates a site setting and publishes appropriate events.
func (s *SiteSettingsService) Set(ctx context.Context, key, value string, actor event.Actor) error {
	// Validate known keys
	switch key {
	case SettingRegistrationMode:
		if value != RegistrationModeOpen && value != RegistrationModeInviteOnly {
			return fmt.Errorf("%w: registration mode must be %q or %q",
				ErrInvalidSetting, RegistrationModeOpen, RegistrationModeInviteOnly)
		}
	case SettingModerationEnabled, SettingModerationPublicVisibility,
		SettingConnectorsEnabled, SettingConnectorsAllowPrivateNetworks:
		if value != "true" && value != "false" {
			return fmt.Errorf("%w: %s must be %q or %q", ErrInvalidSetting, key, "true", "false")
		}
	case SettingConnectorDeliveryRetentionDays:
		// Zero or negative is meaningful (it disables pruning), so only
		// non-numeric input is rejected here.
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("%w: %s must be a whole number of days", ErrInvalidSetting, key)
		}
	}

	// Get old value for event
	oldValue := ""
	if old, err := s.settings.Get(ctx, key); err == nil && old != nil {
		oldValue = old.Value
	}

	if err := s.settings.Set(ctx, key, value); err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}

	// Publish generic setting changed event for all consumers
	if oldValue != value {
		s.eventBus.Publish(ctx, &event.SiteSettingChangedEvent{
			Base:     event.NewBase(event.SiteSettingChanged, actor),
			Key:      key,
			OldValue: oldValue,
			NewValue: value,
		})
	}

	// Publish specific event for registration mode changes (backward compat)
	if key == SettingRegistrationMode && oldValue != value {
		s.eventBus.Publish(ctx, &event.RegistrationModeChangedEvent{
			Base:    event.NewBase(event.RegistrationModeChanged, actor),
			OldMode: oldValue,
			NewMode: value,
		})
	}

	return nil
}

// GetString returns a site setting as a string, or the fallback if not found.
func (s *SiteSettingsService) GetString(ctx context.Context, key string, fallback string) string {
	setting, err := s.settings.Get(ctx, key)
	if err != nil || setting == nil || setting.Value == "" {
		return fallback
	}
	return setting.Value
}

// GetBool returns a site setting parsed as a boolean, or the fallback if not found.
// Truthy values: "true", "1", "yes". Everything else is falsy.
func (s *SiteSettingsService) GetBool(ctx context.Context, key string, fallback bool) bool {
	setting, err := s.settings.Get(ctx, key)
	if err != nil || setting == nil || setting.Value == "" {
		return fallback
	}
	switch setting.Value {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	}
	return fallback
}

// GetInt returns a site setting parsed as an integer, or the fallback if not found or not a valid int.
func (s *SiteSettingsService) GetInt(ctx context.Context, key string, fallback int) int {
	setting, err := s.settings.Get(ctx, key)
	if err != nil || setting == nil {
		return fallback
	}
	v, err := strconv.Atoi(setting.Value)
	if err != nil {
		return fallback
	}
	return v
}

// GetFloat64 returns a site setting parsed as a float64, or the fallback if not found or not a valid float.
func (s *SiteSettingsService) GetFloat64(ctx context.Context, key string, fallback float64) float64 {
	setting, err := s.settings.Get(ctx, key)
	if err != nil || setting == nil {
		return fallback
	}
	v, err := strconv.ParseFloat(setting.Value, 64)
	if err != nil {
		return fallback
	}
	return v
}
