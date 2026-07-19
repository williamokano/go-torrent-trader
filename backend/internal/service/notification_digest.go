package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// digestUnreadItemLimit caps how many individual notifications are fetched to
// build the type breakdown in a digest email. CountUnreadSince still reports
// the true total even when it exceeds this cap.
const digestUnreadItemLimit = 50

// Digest cadence intervals: the minimum elapsed time since a user's last
// digest before another one is due. Using elapsed time (rather than a fixed
// calendar day, e.g. "always Monday") means a missed scheduler run self-heals
// on the next tick instead of skipping a user's weekly digest entirely.
const (
	dailyDigestInterval  = 24 * time.Hour
	weeklyDigestInterval = 7 * 24 * time.Hour
)

// digestTypeLabels renders notification types for the email body, singular —
// buildDigestEmail appends the plural "s" itself based on the count.
var digestTypeLabels = map[string]string{
	model.NotifForumReply:     "forum reply",
	model.NotifMention:        "mention",
	model.NotifTopicReply:     "topic reply",
	model.NotifTorrentComment: "torrent comment",
	model.NotifPMReceived:     "private message",
	model.NotifSystem:         "system notification",
}

// NotificationDigestRunSummary is the outcome of a digest Run.
type NotificationDigestRunSummary struct {
	Recipients int // users evaluated across every cadence
	Emailed    int // users who actually received a digest email
}

// NotificationDigestService runs the periodic email digest job: for every
// user opted into a daily or weekly cadence, it summarizes their unread
// notifications since their last digest and enqueues an email.
type NotificationDigestService struct {
	digestPrefs   repository.NotificationDigestPreferenceRepository
	notifications repository.NotificationRepository
	taskEnqueuer  TaskEnqueuer
	siteBaseURL   string
}

// NewNotificationDigestService creates a new NotificationDigestService.
// taskEnqueuer reuses the same asynq-backed email pipeline as AuthService
// (see TaskEnqueuer) rather than sending mail inline from the scheduler.
func NewNotificationDigestService(
	digestPrefs repository.NotificationDigestPreferenceRepository,
	notifications repository.NotificationRepository,
	taskEnqueuer TaskEnqueuer,
	siteBaseURL string,
) *NotificationDigestService {
	return &NotificationDigestService{
		digestPrefs:   digestPrefs,
		notifications: notifications,
		taskEnqueuer:  taskEnqueuer,
		siteBaseURL:   siteBaseURL,
	}
}

// Run processes every user due for a digest across both cadences. It is safe
// to call once a day: each cadence gates itself independently via ListDue, so
// a single fixed daily schedule drives both "daily" and "weekly" users without
// the caller needing to know which day of the week it is.
//
// A failure for one recipient is logged and skipped rather than aborting the
// whole run — one bad row must not block digests for everyone else.
func (s *NotificationDigestService) Run(ctx context.Context, now time.Time) (NotificationDigestRunSummary, error) {
	var summary NotificationDigestRunSummary

	cadences := []struct {
		frequency string
		interval  time.Duration
	}{
		{model.DigestDaily, dailyDigestInterval},
		{model.DigestWeekly, weeklyDigestInterval},
	}

	for _, cadence := range cadences {
		recipients, err := s.digestPrefs.ListDue(ctx, cadence.frequency, now.Add(-cadence.interval))
		if err != nil {
			return summary, fmt.Errorf("list due %s digest recipients: %w", cadence.frequency, err)
		}

		for _, recipient := range recipients {
			summary.Recipients++
			sent, err := s.sendDigest(ctx, recipient, cadence.frequency, now)
			if err != nil {
				slog.Error("notification digest: failed to process recipient",
					"user_id", recipient.UserID, "frequency", cadence.frequency, "error", err)
				continue
			}
			if sent {
				summary.Emailed++
			}
		}
	}

	return summary, nil
}

// sendDigest builds and enqueues one recipient's digest email, if they have
// anything unread to report. It always advances the send cursor (MarkSent) so
// the "since" window never grows unbounded, even when nothing was sent.
//
// "Since last digest" is deliberately narrower than "everything currently
// unread": it's unread notifications *created* after the last digest
// (CountUnreadSince/ListUnreadSince filter on created_at, not just is_read).
// A notification that was unread and already reported in an earlier digest
// will not resurface just because it's still unread — the digest is a "what's
// new" summary, not a mirror of the in-app unread badge (which uses plain
// CountUnread and can legitimately show a different number).
//
// now is the Run() start time, not the time this recipient's row is actually
// processed. A notification created in that gap satisfies created_at > since
// for this run and could also satisfy it again next run, so in the rare case
// of a notification landing mid-run it may appear in two consecutive digests.
// That's an accepted trade-off: never dropping a notification is worth more
// than never double-reporting one, and daily/weekly cadences make the overlap
// negligible in practice.
func (s *NotificationDigestService) sendDigest(ctx context.Context, recipient model.DigestRecipient, frequency string, now time.Time) (bool, error) {
	since := time.Time{}
	if recipient.LastDigestSentAt != nil {
		since = *recipient.LastDigestSentAt
	}

	total, err := s.notifications.CountUnreadSince(ctx, recipient.UserID, since)
	if err != nil {
		return false, fmt.Errorf("count unread since: %w", err)
	}
	if total == 0 {
		if err := s.digestPrefs.MarkSent(ctx, recipient.UserID, now); err != nil {
			return false, fmt.Errorf("mark sent: %w", err)
		}
		return false, nil
	}

	if recipient.Email == "" {
		slog.Warn("notification digest: recipient has no email on file, skipping send", "user_id", recipient.UserID)
		if err := s.digestPrefs.MarkSent(ctx, recipient.UserID, now); err != nil {
			return false, fmt.Errorf("mark sent: %w", err)
		}
		return false, nil
	}

	items, err := s.notifications.ListUnreadSince(ctx, recipient.UserID, since, digestUnreadItemLimit)
	if err != nil {
		return false, fmt.Errorf("list unread since: %w", err)
	}

	subject, body := buildDigestEmail(frequency, total, items, s.siteBaseURL)

	if s.taskEnqueuer == nil {
		slog.Warn("notification digest: no task enqueuer configured, skipping email", "user_id", recipient.UserID)
	} else if err := s.taskEnqueuer.EnqueueSendEmail(ctx, recipient.Email, subject, body); err != nil {
		return false, fmt.Errorf("enqueue digest email: %w", err)
	}

	if err := s.digestPrefs.MarkSent(ctx, recipient.UserID, now); err != nil {
		return false, fmt.Errorf("mark sent: %w", err)
	}
	return true, nil
}

// buildDigestEmail renders the subject and HTML body summarizing a user's
// unread notifications since their last digest, grouped by type.
func buildDigestEmail(frequency string, total int, items []model.Notification, siteBaseURL string) (subject, body string) {
	subject = fmt.Sprintf("Your %s digest: %d unread notification%s", frequency, total, plural(total))

	counts := make(map[string]int, len(digestTypeLabels))
	for _, n := range items {
		counts[n.Type]++
	}

	var b strings.Builder
	b.WriteString("<p>Hi,</p>")
	fmt.Fprintf(&b, "<p>You have <strong>%d</strong> unread notification%s since your last digest:</p><ul>", total, plural(total))
	for _, t := range model.AllNotificationTypes {
		if c := counts[t]; c > 0 {
			fmt.Fprintf(&b, "<li>%d %s%s</li>", c, digestLabel(t), plural(c))
		}
	}
	b.WriteString("</ul>")
	if len(items) < total {
		fmt.Fprintf(&b, "<p>Showing the %d most recent — view the rest on the site.</p>", len(items))
	}
	fmt.Fprintf(&b, `<p><a href="%s/notifications">View your notifications</a></p>`, siteBaseURL)
	fmt.Fprintf(&b, `<p>You're receiving this because your notification digest is set to %q. `+
		`You can change this anytime in your notification preferences.</p>`, frequency)
	b.WriteString("<p>Thanks,<br>TorrentTrader</p>")

	return subject, b.String()
}

// digestLabel returns a human-readable label for a notification type,
// falling back to the raw type string for anything unmapped.
func digestLabel(notifType string) string {
	if label, ok := digestTypeLabels[notifType]; ok {
		return label
	}
	return notifType
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
