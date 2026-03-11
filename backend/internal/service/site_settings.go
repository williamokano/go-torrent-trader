package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

const (
	// SettingRegistrationMode controls whether registration is open or invite-only.
	SettingRegistrationMode = "registration_mode"

	// RegistrationModeOpen allows anyone to register.
	RegistrationModeOpen = "open"

	// RegistrationModeInviteOnly requires an invite code to register.
	RegistrationModeInviteOnly = "invite_only"

	// Chat anti-spam settings keys.
	SettingChatRateLimitWindow  = "chat_rate_limit_window"
	SettingChatRateLimitMax     = "chat_rate_limit_max"
	SettingChatSpamStrikeCount  = "chat_spam_strike_count"
	SettingChatSpamMuteMinutes  = "chat_spam_mute_minutes"
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
			return fmt.Errorf("invalid registration mode: must be %q or %q", RegistrationModeOpen, RegistrationModeInviteOnly)
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
