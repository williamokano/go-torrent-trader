package worker

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/hibiken/asynq"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// TaskRatioWarning is the task type for the ratio warning check job.
const TaskRatioWarning = "ratio:warning"

// NewRatioWarningTask creates a task for the ratio warning check.
func NewRatioWarningTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskRatioWarning, nil, asynq.MaxRetry(1), asynq.Unique(5*time.Hour)), nil
}

// NewRatioWarningHandler returns an asynq handler that checks user ratios and issues warnings/bans.
func NewRatioWarningHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		if deps.WarningSvc == nil || deps.SiteSettingsSvc == nil {
			slog.Warn("ratio warning: missing dependencies, skipping")
			return nil
		}

		// Load settings
		threshold := getFloatSetting(ctx, deps.SiteSettingsSvc, "ratio_warning_threshold", 0.3)
		minDownloaded := getIntSetting(ctx, deps.SiteSettingsSvc, "ratio_minimum_downloaded", 5368709120)
		warnDays := int(getIntSetting(ctx, deps.SiteSettingsSvc, "ratio_warn_days", 7))
		banDays := int(getIntSetting(ctx, deps.SiteSettingsSvc, "ratio_ban_days", 14))
		warnMsg := getStringSetting(ctx, deps.SiteSettingsSvc, "ratio_warning_message",
			"Dear {{username}}, your ratio ({{ratio}}) has been below the minimum threshold of {{threshold}} for {{days_elapsed}} days. You have {{days_remaining}} days to improve before your account is disabled.")
		banMsg := getStringSetting(ctx, deps.SiteSettingsSvc, "ratio_ban_message",
			"Dear {{username}}, your account has been disabled because your ratio ({{ratio}}) remained below the minimum threshold of {{threshold}} for more than {{days_elapsed}} days.")

		escalationDays := banDays - warnDays
		if escalationDays <= 0 {
			escalationDays = 7 // safety default
		}

		// Find users with low ratio
		users, err := deps.WarningSvc.GetUsersWithLowRatio(ctx, threshold, minDownloaded)
		if err != nil {
			return fmt.Errorf("get users with low ratio: %w", err)
		}

		slog.Info("ratio warning check", "users_below_threshold", len(users), "threshold", threshold)

		for _, u := range users {
			ratio := float64(0)
			if u.Downloaded > 0 {
				ratio = float64(u.Uploaded) / float64(u.Downloaded)
			}
			ratioStr := fmt.Sprintf("%.3f", ratio)
			thresholdStr := fmt.Sprintf("%.2f", threshold)

			// Check if user already has an active ratio warning
			existing, err := deps.WarningSvc.GetActiveRatioWarning(ctx, u.ID)
			if err != nil {
				slog.Error("ratio warning: failed to check existing warning", "user_id", u.ID, "error", err)
				continue
			}

			if existing == nil {
				// No existing warning: issue a soft warning
				daysSinceWarn := 0
				daysRemaining := escalationDays
				msg := service.ReplaceTemplateVars(warnMsg, map[string]string{
					"username":       u.Username,
					"ratio":          ratioStr,
					"threshold":      thresholdStr,
					"days_elapsed":   strconv.Itoa(daysSinceWarn),
					"days_remaining": strconv.Itoa(daysRemaining),
				})

				if _, err := deps.WarningSvc.IssueRatioWarning(ctx, u.ID, msg); err != nil {
					slog.Error("ratio warning: failed to issue warning", "user_id", u.ID, "error", err)
				} else {
					slog.Info("ratio warning: issued soft warning", "user_id", u.ID, "username", u.Username, "ratio", ratioStr)
				}
			} else {
				// Existing warning: check if we need to escalate
				daysSinceWarning := int(time.Since(existing.CreatedAt).Hours() / 24)
				if daysSinceWarning >= escalationDays {
					// Time to escalate — disable user
					msg := service.ReplaceTemplateVars(banMsg, map[string]string{
						"username":     u.Username,
						"ratio":        ratioStr,
						"threshold":    thresholdStr,
						"days_elapsed": strconv.Itoa(daysSinceWarning + warnDays),
					})

					if err := deps.WarningSvc.EscalateRatioWarning(ctx, existing.ID, msg); err != nil {
						slog.Error("ratio warning: failed to escalate", "user_id", u.ID, "error", err)
					} else {
						slog.Info("ratio warning: escalated to ban", "user_id", u.ID, "username", u.Username, "ratio", ratioStr)
					}
				}
				// Otherwise, just wait — warning already active
			}
		}

		// Check for users whose ratio has improved above threshold and resolve their warnings
		// We do this by checking all active ratio_soft warnings and seeing if the user's ratio is now OK
		if deps.DB != nil {
			if err := resolveImprovedRatios(ctx, deps, threshold, minDownloaded); err != nil {
				slog.Error("ratio warning: failed to resolve improved ratios", "error", err)
			}
		}

		return nil
	}
}

func resolveImprovedRatios(ctx context.Context, deps *WorkerDeps, threshold float64, minDownloaded int64) error {
	// Find all active ratio_soft warnings where the user's ratio has improved
	rows, err := deps.DB.QueryContext(ctx, `
		SELECT w.id, w.user_id
		FROM warnings w
		JOIN users u ON u.id = w.user_id
		WHERE w.type = 'ratio_soft' AND w.status = 'active'
		  AND u.enabled = true
		  AND (u.downloaded <= $1 OR (u.uploaded::float / GREATEST(u.downloaded::float, 1)) >= $2)
	`, minDownloaded, threshold)
	if err != nil {
		return fmt.Errorf("query improved ratios: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var resolved int
	for rows.Next() {
		var warningID, userID int64
		if err := rows.Scan(&warningID, &userID); err != nil {
			slog.Error("ratio warning: scan improved ratio", "error", err)
			continue
		}
		if err := deps.WarningSvc.ResolveWarning(ctx, warningID); err != nil {
			slog.Error("ratio warning: resolve warning", "warning_id", warningID, "error", err)
		} else {
			resolved++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate improved ratios: %w", err)
	}

	if resolved > 0 {
		slog.Info("ratio warning: resolved improved ratios", "count", resolved)
	}

	return nil
}

func getFloatSetting(ctx context.Context, svc *service.SiteSettingsService, key string, defaultVal float64) float64 {
	settings, err := svc.GetAll(ctx)
	if err != nil {
		return defaultVal
	}
	for _, s := range settings {
		if s.Key == key {
			v, err := strconv.ParseFloat(s.Value, 64)
			if err != nil {
				return defaultVal
			}
			return v
		}
	}
	return defaultVal
}

func getIntSetting(ctx context.Context, svc *service.SiteSettingsService, key string, defaultVal int64) int64 {
	settings, err := svc.GetAll(ctx)
	if err != nil {
		return defaultVal
	}
	for _, s := range settings {
		if s.Key == key {
			v, err := strconv.ParseInt(s.Value, 10, 64)
			if err != nil {
				return defaultVal
			}
			return v
		}
	}
	return defaultVal
}

func getStringSetting(ctx context.Context, svc *service.SiteSettingsService, key string, defaultVal string) string {
	settings, err := svc.GetAll(ctx)
	if err != nil {
		return defaultVal
	}
	for _, s := range settings {
		if s.Key == key {
			return s.Value
		}
	}
	return defaultVal
}
