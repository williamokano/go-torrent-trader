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

	// Publish event for registration mode changes
	if key == SettingRegistrationMode && oldValue != value {
		s.eventBus.Publish(ctx, &event.RegistrationModeChangedEvent{
			Base:    event.NewBase(event.RegistrationModeChanged, actor),
			OldMode: oldValue,
			NewMode: value,
		})
	}

	return nil
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
