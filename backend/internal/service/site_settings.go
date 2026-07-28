package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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

	// SettingChatSystemDisplayName is the author label shown on authorless
	// shoutbox announcements. Read through SystemChatName, which caches it.
	SettingChatSystemDisplayName = "chat_system_display_name"

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

// maxSystemChatNameLength bounds the shoutbox author label. It shares a flex row
// with the timestamp and the message, so an essay here squeezes the text it is
// supposed to introduce.
const maxSystemChatNameLength = 32

// maxAnnounceLogRetentionDays is a century, which is not a real retention policy —
// it is the point past which the value stops meaning anything. The worker turns
// this setting into `days * 24h`, and int64 nanoseconds overflow above roughly
// 106,751 days, wrapping to a negative duration that the prune reads as "pruning
// disabled". A bound well inside that keeps the stored number and the behaviour in
// agreement; 0 is the supported way to keep announces forever.
const maxAnnounceLogRetentionDays = 36500

// cachedSettingTTL bounds how stale a cached setting may get. Set evicts
// immediately, which is exact within one process — this is the safety net for a
// replica that did not serve the write and never sees the event.
const cachedSettingTTL = 5 * time.Minute

// cachedSetting is one memoised value and the instant it stops being trusted.
type cachedSetting struct {
	value     string
	expiresAt time.Time
}

// SiteSettingsService handles site settings business logic.
type SiteSettingsService struct {
	settings repository.SiteSettingsRepository
	eventBus event.Bus

	// cacheTTL is a field rather than the bare constant so a test can shrink it;
	// production always gets cachedSettingTTL.
	cacheTTL time.Duration
	cacheMu  sync.RWMutex
	cache    map[string]cachedSetting
	// cacheGen is bumped on every eviction. A read that started before a write
	// landed carries the old generation and is discarded instead of being
	// written back — otherwise the evicted value would be resurrected for a
	// full TTL by whichever read happened to be mid-query. Global rather than
	// per key: a needless miss on an unrelated key costs one query.
	cacheGen uint64
	// now is swappable for the same reason. Nil is never stored — the
	// constructor fills it.
	now func() time.Time
}

// NewSiteSettingsService creates a new SiteSettingsService.
func NewSiteSettingsService(settings repository.SiteSettingsRepository, bus event.Bus) *SiteSettingsService {
	return &SiteSettingsService{
		settings: settings,
		eventBus: bus,
		cacheTTL: cachedSettingTTL,
		cache:    make(map[string]cachedSetting),
		now:      time.Now,
	}
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
	case SettingAnnounceLogRetentionDays:
		// Same shape as the connector retention above — zero disables pruning — but
		// with an upper bound, because this one is converted to a time.Duration.
		// Beyond roughly 106,751 days that multiplication overflows int64 into a
		// negative value, which the prune reads as "disabled": a fat-fingered entry
		// would turn pruning off while the admin page displayed the number they
		// typed. Rejecting it is how the setting stops lying.
		days, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%w: %s must be a whole number of days", ErrInvalidSetting, key)
		}
		if days > maxAnnounceLogRetentionDays {
			return fmt.Errorf("%w: %s cannot exceed %d days (use 0 to keep announces indefinitely)",
				ErrInvalidSetting, key, maxAnnounceLogRetentionDays)
		}
	case SettingPromotionSeedWindowDays:
		// Bounded for the same reason as the retention window above, and it became
		// the same kind of problem for a second reason: this value now also sets
		// the *floor* under retention, so it is reported to operators as the
		// effective window and to members as how long their announces are kept.
		// Past the int64 overflow the prune reads the wrapped duration as "no
		// floor" and keeps the short window, while both surfaces confidently
		// report the enormous number — the setting lying in two places instead of
		// one. A negative window is meaningless rather than merely large.
		days, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%w: %s must be a whole number of days", ErrInvalidSetting, key)
		}
		if days < 0 {
			return fmt.Errorf("%w: %s cannot be negative", ErrInvalidSetting, key)
		}
		if days > maxAnnounceLogRetentionDays {
			return fmt.Errorf("%w: %s cannot exceed %d days",
				ErrInvalidSetting, key, maxAnnounceLogRetentionDays)
		}
	case SettingChatSystemDisplayName:
		// Blank would render an announcement with no author at all, which reads
		// as a broken row rather than as a site notice. Counted in runes: the
		// limit is about how wide the label renders, not how many bytes it is.
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fmt.Errorf("%w: %s cannot be empty", ErrInvalidSetting, key)
		}
		if utf8.RuneCountInString(trimmed) > maxSystemChatNameLength {
			return fmt.Errorf("%w: %s cannot exceed %d characters",
				ErrInvalidSetting, key, maxSystemChatNameLength)
		}
		value = trimmed
	}

	// Get old value for event
	oldValue := ""
	if old, err := s.settings.Get(ctx, key); err == nil && old != nil {
		oldValue = old.Value
	}

	if err := s.settings.Set(ctx, key, value); err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}

	// Evict before anything else observable happens, so a listener reacting to
	// the event below cannot read the value this call just replaced.
	s.evict(key)

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

// SystemChatName returns the author label for authorless shoutbox messages,
// falling back to model.SystemChatUsername when unset or unreadable.
//
// This is the only cached read on the service, and deliberately so: it is hit on
// every shoutbox backfill and every announcement, it changes about never, and
// being a display name it cannot do any harm by being a few minutes stale. A
// general-purpose cached getter would invite the same treatment for something
// like the connectors kill-switch, where an operator flipping it expects the
// site to obey now.
func (s *SiteSettingsService) SystemChatName(ctx context.Context) string {
	return s.cachedString(ctx, SettingChatSystemDisplayName, model.SystemChatUsername)
}

// cachedString memoises one setting for cacheTTL.
//
// "No such row" is a stable answer and is cached as the fallback; any other
// error is a storage failure, which returns the fallback but is *not* cached —
// a connection blip must not pin the default for the whole TTL.
func (s *SiteSettingsService) cachedString(ctx context.Context, key, fallback string) string {
	now := s.now()
	value, gen, ok := s.cacheLookup(key, now)
	if ok {
		return value
	}

	setting, err := s.settings.Get(ctx, key)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// Not cached, so the next announcement retries — but say so, because
		// otherwise an outage quietly renames the announcer on every line.
		slog.Warn("site setting read failed, using fallback",
			"key", key, "fallback", fallback, "error", err)
		return fallback
	}

	value = fallback
	if err == nil && setting != nil {
		// Trimmed on read as well as on write: Set cannot reach a row an
		// operator edited straight through psql, and a whitespace-only value
		// would render an announcement with a blank author.
		if trimmed := strings.TrimSpace(setting.Value); trimmed != "" {
			value = trimmed
		}
	}

	s.cacheStore(key, value, now.Add(s.cacheTTL), gen)
	return value
}

// cacheLookup returns a live cache entry and the generation it was read at. Its
// own method so the read lock is released by defer and cannot still be held
// when cachedString goes on to take the write lock — an RWMutex is not
// reentrant, and that would deadlock.
func (s *SiteSettingsService) cacheLookup(key string, now time.Time) (string, uint64, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	entry, ok := s.cache[key]
	if !ok || !now.Before(entry.expiresAt) {
		return "", s.cacheGen, false
	}
	return entry.value, s.cacheGen, true
}

// cacheStore writes back a value read from storage, unless something was
// evicted while that read was in flight — in which case the value is already
// known to be stale and must not be cached.
func (s *SiteSettingsService) cacheStore(key, value string, expiresAt time.Time, gen uint64) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	if s.cacheGen != gen {
		return
	}
	s.cache[key] = cachedSetting{value: value, expiresAt: expiresAt}
}

// evict drops a cached key. Called from Set, which is the only writer inside
// this process; another process's write is only picked up when the entry
// expires, which is what cachedSettingTTL is for.
func (s *SiteSettingsService) evict(key string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.cacheGen++
	delete(s.cache, key)
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
