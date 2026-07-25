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

// unresolvedCategoryPath is what an admin reads in the delivery log when an
// announcement was withheld because the category tree could not be read.
const unresolvedCategoryPath = "withheld: the torrent's category tree could not be read, " +
	"so this instance's exclude filter could not be applied safely"

// DrainEnqueuer queues the background job that actually talks to a destination.
// Declared here rather than imported from worker so the listener does not depend
// on the queue implementation.
type DrainEnqueuer interface {
	EnqueueConnectorDrain(ctx context.Context, instanceID int64, delay time.Duration) error
}

// CategoryPathResolver returns a category's ancestor chain, root first. It is a
// function rather than an interface because there is exactly one implementation
// (service.CategoryAncestorIDs) and a test wants a two-line stub.
type CategoryPathResolver func(ctx context.Context, categoryID int64) ([]int64, error)

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
	resolveCategoryPath CategoryPathResolver,
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

		instances, err := connectors.ListEnabled(ctx)
		if err != nil {
			slog.Error("connector dispatch: failed to list enabled connectors",
				"torrent_id", e.TorrentID, "error", err)
			return nil
		}

		// Filters are parsed before anything else is done, because whether the
		// category chain has to be resolved at all depends on them. A broken
		// filter row is logged and skipped rather than aborting the loop: one
		// bad connector must not silence the others, and none of them may fail
		// the approve this runs inside.
		type target struct {
			inst    model.NotificationConnector
			filters connector.Filters
		}
		targets := make([]target, 0, len(instances))
		for _, inst := range instances {
			filters, err := connector.ParseFilters(inst.Filters)
			if err != nil {
				slog.Error("connector dispatch: invalid filters",
					"instance_id", inst.ID, "kind", inst.Kind, "error", err)
				continue
			}
			targets = append(targets, target{inst: inst, filters: filters})
		}
		if len(targets) == 0 {
			// Nothing to announce to, so nothing to resolve: the whole feature
			// costs a disabled site nothing.
			return nil
		}

		// The ancestor chain is what lets a category filter stand for a whole
		// subtree. Resolved once per event, and unconditionally rather than only
		// when some instance filters on category: it is marshaled into the stored
		// payload, which IRC re-reads at delivery time to route its per-channel
		// categories the same way. Resolving it only sometimes would make the
		// payload's shape depend on unrelated instances' filters, and would leave
		// IRC routing quietly leaf-only whenever nothing else needed the chain.
		// The cost is a handful of primary-key lookups per approval.
		categoryPathKnown := true
		if announcement.CategoryID > 0 {
			if resolveCategoryPath == nil {
				// Reporting the chain as known would quietly downgrade every
				// exclude filter to leaf-only matching, which is the leak this
				// whole path exists to prevent.
				categoryPathKnown = false
				slog.Error("connector dispatch: no category path resolver configured",
					"torrent_id", e.TorrentID)
			} else if path, err := resolveCategoryPath(ctx, announcement.CategoryID); err != nil {
				categoryPathKnown = false
				slog.Error("connector dispatch: failed to resolve category path",
					"torrent_id", e.TorrentID, "category_id", announcement.CategoryID, "error", err)
			} else {
				announcement.CategoryPath = path
			}
		}

		payload, err := json.Marshal(announcement)
		if err != nil {
			slog.Error("connector dispatch: failed to marshal announcement",
				"torrent_id", e.TorrentID, "error", err)
			return nil
		}

		eventKey := connector.EventKey(announcement)
		for _, t := range targets {
			inst, filters := t.inst, t.filters

			row := &model.ConnectorDelivery{
				InstanceID: inst.ID,
				EventKey:   eventKey,
				EventType:  announcement.Event,
				Payload:    payload,
				Status:     model.DeliveryPending,
			}

			excludesCategories := filters.CategoryMode == connector.CategoryModeExclude &&
				len(filters.CategoryIDs) > 0
			if !categoryPathKnown && excludesCategories {
				// Fail closed, but only here. Without the chain an exclude filter
				// would let through every sub-category of the thing it exists to
				// keep out. An include filter cannot over-deliver on the leaf
				// fallback — at worst it delivers nothing — so it is left alone
				// rather than losing announcements for no safety gain.
				//
				// The row is still written, failed and with the reason, because
				// an announcement that silently never happened is the one thing
				// the delivery log must not hide.
				reason := unresolvedCategoryPath
				row.Status = model.DeliveryFailed
				row.LastError = &reason
				if _, err := deliveries.InsertPending(ctx, row); err != nil {
					slog.Error("connector dispatch: failed to record withheld delivery",
						"instance_id", inst.ID, "event_key", eventKey, "error", err)
				}
				slog.Warn("connector dispatch: withheld from an exclude-filtered instance, category path unknown",
					"instance_id", inst.ID, "kind", inst.Kind, "torrent_id", e.TorrentID)
				continue
			}
			if !filters.Matches(announcement) {
				continue
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
