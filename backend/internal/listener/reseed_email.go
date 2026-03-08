package listener

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// RegisterReseedEmailListener subscribes a listener that sends an email to the
// torrent uploader when a reseed is requested.
func RegisterReseedEmailListener(bus event.Bus, email service.EmailSender, siteBaseURL string) {
	bus.Subscribe(event.ReseedRequested, func(ctx context.Context, evt event.Event) error {
		e := evt.(*event.ReseedRequestedEvent)

		if e.UploaderEmail == "" {
			return nil
		}

		subject := fmt.Sprintf("Reseed requested: %s", e.TorrentName)
		body := fmt.Sprintf(
			`<p>Hi,</p>
<p><strong>%s</strong> has requested a reseed for your torrent:</p>
<p><a href="%s/torrent/%d">%s</a></p>
<p>Please consider re-seeding this torrent if you still have the files.</p>
<p>Thanks,<br>TorrentTrader</p>`,
			e.Actor.Username, siteBaseURL, e.TorrentID, e.TorrentName,
		)

		if err := email.Send(ctx, e.UploaderEmail, subject, body); err != nil {
			slog.Error("failed to send reseed email",
				"torrent_id", e.TorrentID,
				"uploader_email", e.UploaderEmail,
				"error", err,
			)
			return fmt.Errorf("send reseed email: %w", err)
		}

		return nil
	})
}
