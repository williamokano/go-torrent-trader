package listener

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// dispatchTimeout bounds the work this listener does. The event bus is
// synchronous, so everything here runs inside the HTTP request that approved the
// torrent — it must stay to fast DB inserts plus a Redis enqueue.
const dispatchTimeout = 5 * time.Second

// DrainEnqueuer queues the background job that actually talks to a destination.
// Declared here rather than imported from worker so the listener does not depend
// on the queue implementation.
type DrainEnqueuer interface {
	EnqueueConnectorDrain(ctx context.Context, instanceID int64, delay time.Duration) error
}

// RegisterConnectorDispatcher wires TorrentPublished to the connector pipeline.
//
// The listener never delivers anything itself: it records one pending delivery
// row per matching enabled instance and asks the worker to drain. That is what
// keeps a wedged webhook from ever slowing down — let alone failing — the
// approve request that triggered it.
func RegisterConnectorDispatcher(
	bus event.Bus,
	connectors repository.ConnectorRepository,
	deliveries repository.ConnectorDeliveryRepository,
	settings *service.SiteSettingsService,
	enq DrainEnqueuer,
	siteBaseURL string,
) {
	bus.Subscribe(event.TorrentPublished, func(_ context.Context, evt event.Event) error {
		// The bus dispatches synchronously and does not recover, so a panic here
		// would travel up through publishPublished into ApproveTorrent and 500 an
		// approval that has already been committed. Nothing this listener does is
		// worth that.
		defer func() {
			if r := recover(); r != nil {
				slog.Error("connector dispatch: panicked", "panic", r)
			}
		}()

		e, ok := evt.(*event.TorrentPublishedEvent)
		if !ok {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), dispatchTimeout)
		defer cancel()

		// One switch that mutes every kind, checked before anything is recorded.
		if settings != nil && !settings.GetBool(ctx, service.SettingConnectorsEnabled, true) {
			return nil
		}

		announcement := AnnouncementFromPublished(e, siteBaseURL)
		payload, err := json.Marshal(announcement)
		if err != nil {
			slog.Error("connector dispatch: failed to marshal announcement",
				"torrent_id", e.TorrentID, "error", err)
			return nil
		}

		instances, err := connectors.ListEnabled(ctx)
		if err != nil {
			slog.Error("connector dispatch: failed to list enabled connectors",
				"torrent_id", e.TorrentID, "error", err)
			return nil
		}

		eventKey := connector.EventKey(announcement)
		for _, inst := range instances {
			// Errors are logged per instance and never abort the loop: one
			// broken connector row must not silence the others, and none of them
			// may fail the approve.
			filters, err := connector.ParseFilters(inst.Filters)
			if err != nil {
				slog.Error("connector dispatch: invalid filters",
					"instance_id", inst.ID, "kind", inst.Kind, "error", err)
				continue
			}
			if !filters.Matches(announcement) {
				continue
			}

			row := &model.ConnectorDelivery{
				InstanceID: inst.ID,
				EventKey:   eventKey,
				EventType:  announcement.Event,
				Payload:    payload,
				Status:     model.DeliveryPending,
			}
			inserted, err := deliveries.InsertPending(ctx, row)
			if err != nil {
				slog.Error("connector dispatch: failed to record delivery",
					"instance_id", inst.ID, "event_key", eventKey, "error", err)
				continue
			}
			if !inserted {
				// A delivery for this event already exists — the unique index
				// caught a duplicate dispatch, so there is nothing new to drain.
				continue
			}
			if enq == nil {
				continue
			}
			if err := enq.EnqueueConnectorDrain(ctx, inst.ID, 0); err != nil {
				// The row is safely persisted; the maintenance sweep re-enqueues
				// instances with due rows, so a Redis blip delays delivery
				// rather than losing it.
				slog.Error("connector dispatch: failed to enqueue drain",
					"instance_id", inst.ID, "error", err)
			}
		}

		return nil
	})
}

// AnnouncementFromPublished projects the domain event onto the connector-facing
// announcement.
//
// The event already carries an empty UploaderName for an anonymous upload (the
// service drops it at the source), so this only has to supply the display name.
// It never reads a real username from anywhere else.
func AnnouncementFromPublished(e *event.TorrentPublishedEvent, siteBaseURL string) connector.Announcement {
	uploader := e.UploaderName
	if e.Anonymous || uploader == "" {
		uploader = connector.AnonymousUploader
	}

	a := connector.Announcement{
		Event:       connector.EventTorrentPublished,
		TorrentID:   e.TorrentID,
		Name:        e.Name,
		InfoHashHex: e.InfoHashHex,
		CategoryID:  e.CategoryID,
		Category:    e.CategoryName,
		Size:        e.Size,
		FileCount:   e.FileCount,
		Uploader:    uploader,
		Anonymous:   e.Anonymous,
		Freeleech:   e.Freeleech,
		Silver:      e.Silver,
		URL:         torrentURL(siteBaseURL, e.TorrentID),
		PublishedAt: e.PublishedAt,
	}
	a.Title = a.Name
	a.Body, _ = connector.RenderTemplate(connector.DefaultTemplate, a)
	return a
}

func torrentURL(baseURL string, torrentID int64) string {
	if baseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/torrent/%d", trimTrailingSlash(baseURL), torrentID)
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
