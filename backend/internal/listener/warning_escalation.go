package listener

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// RegisterWarningEscalationListener subscribes to WarningIssued events and
// automatically escalates manual warnings based on site settings:
//   - At warning_count_restrict active manual warnings: apply a privilege restriction.
//   - At warning_count_ban active manual warnings: disable the user's account.
//
// Only manual warnings trigger escalation. Ratio warnings have their own flow.
func RegisterWarningEscalationListener(
	bus event.Bus,
	siteSettings *service.SiteSettingsService,
	warningRepo repository.WarningRepository,
	restrictionSvc *service.RestrictionService,
	userRepo repository.UserRepository,
	activityLogSvc *service.ActivityLogService,
) {
	bus.Subscribe(event.WarningIssued, func(ctx context.Context, evt event.Event) error {
		e := evt.(*event.WarningIssuedEvent)

		// Only escalate manual warnings — ratio warnings have their own escalation path.
		if e.WarningType != model.WarningTypeManual {
			return nil
		}

		// Check master toggle.
		if !siteSettings.GetBool(ctx, service.SettingWarningEscalationEnabled, false) {
			return nil
		}

		// Count active manual warnings for this user.
		activeCount, err := warningRepo.CountActiveManualByUser(ctx, e.UserID)
		if err != nil {
			slog.Error("warning_escalation: failed to count active manual warnings",
				"user_id", e.UserID,
				"error", err,
			)
			return nil
		}

		banThreshold := siteSettings.GetInt(ctx, service.SettingWarningCountBan, 3)
		restrictThreshold := siteSettings.GetInt(ctx, service.SettingWarningCountRestrict, 2)

		actor := event.Actor{ID: 0, Username: "System"}

		if activeCount >= banThreshold {
			// Disable the user's account.
			if err := disableUser(ctx, userRepo, e.UserID); err != nil {
				slog.Error("warning_escalation: failed to disable user",
					"user_id", e.UserID,
					"error", err,
				)
				return nil
			}

			// Log to activity log.
			logEntry := &model.ActivityLog{
				EventType: "warning_escalation_ban",
				Message:   fmt.Sprintf("System disabled %s after %d active manual warnings (threshold: %d)", e.Username, activeCount, banThreshold),
			}
			if err := activityLogSvc.Create(ctx, logEntry); err != nil {
				slog.Error("warning_escalation: failed to write activity log", "error", err)
			}

			slog.Info("warning_escalation: user account disabled",
				"user_id", e.UserID,
				"username", e.Username,
				"active_warnings", activeCount,
				"threshold", banThreshold,
			)

			// Publish a UserBanned event so other listeners (e.g. activity log) can react.
			bus.Publish(ctx, &event.UserBannedEvent{
				Base:     event.NewBase(event.UserBanned, actor),
				UserID:   e.UserID,
				Username: e.Username,
			})

			return nil
		}

		if activeCount >= restrictThreshold {
			restrictType := siteSettings.GetString(ctx, service.SettingWarningRestrictType, "download")
			restrictDays := siteSettings.GetInt(ctx, service.SettingWarningRestrictDays, 7)

			// For "all", apply each restriction type individually.
			types := []string{restrictType}
			if restrictType == "all" {
				types = []string{
					model.RestrictionTypeDownload,
					model.RestrictionTypeUpload,
					model.RestrictionTypeChat,
				}
			}

			expiresAt := time.Now().Add(time.Duration(restrictDays) * 24 * time.Hour)
			reason := fmt.Sprintf("Automatic restriction: %d active manual warnings (threshold: %d)", activeCount, restrictThreshold)

			for _, rt := range types {
				if _, err := restrictionSvc.ApplyRestriction(ctx, e.UserID, rt, reason, &expiresAt, nil); err != nil {
					slog.Error("warning_escalation: failed to apply restriction",
						"user_id", e.UserID,
						"restriction_type", rt,
						"error", err,
					)
				}
			}

			// Log to activity log.
			logEntry := &model.ActivityLog{
				EventType: "warning_escalation_restrict",
				Message:   fmt.Sprintf("System restricted %s (%s for %d days) after %d active manual warnings (threshold: %d)", e.Username, restrictType, restrictDays, activeCount, restrictThreshold),
			}
			if err := activityLogSvc.Create(ctx, logEntry); err != nil {
				slog.Error("warning_escalation: failed to write activity log", "error", err)
			}

			slog.Info("warning_escalation: privilege restriction applied",
				"user_id", e.UserID,
				"username", e.Username,
				"restriction_type", restrictType,
				"duration_days", restrictDays,
				"active_warnings", activeCount,
				"threshold", restrictThreshold,
			)
		}

		return nil
	})
}

// disableUser sets the user's enabled flag to false.
func disableUser(ctx context.Context, userRepo repository.UserRepository, userID int64) error {
	user, err := userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user %d: %w", userID, err)
	}
	user.Enabled = false
	if err := userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("disable user %d: %w", userID, err)
	}
	return nil
}
