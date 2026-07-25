// Package connector defines the seam between the site's domain events and the
// external destinations that announce them (shoutbox, webhooks, IRC, SSE,
// Discord, Telegram).
//
// The registry is built at compile time in cmd/server: there is no hot-loading
// and no plugin framework. Adding a destination means adding one package that
// implements Connector and one line of registration — everything else (filters,
// dedupe, retry, rate limiting, the delivery log, the admin UI) is shared
// pipeline that a new connector inherits for free.
package connector

import (
	"context"
	"encoding/json"
	"fmt"
)

// Instance is one configured destination row, reduced to what a connector needs.
// The ID is part of it because persistent connectors (IRC) have to map a
// delivery back to the specific long-lived client that owns that instance.
type Instance struct {
	ID     int64
	Name   string
	Config json.RawMessage
}

// Connector is a destination kind. Implementations are stateless with respect to
// a delivery: everything they need arrives in Instance.Config and the
// Announcement.
type Connector interface {
	// Kind is the stable identifier stored in notification_connectors.kind.
	Kind() string

	// Singleton reports whether at most one instance of this kind may exist.
	// True for destinations that are inherently site-wide (chat, sse), where a
	// second instance would just double-post to the same place.
	Singleton() bool

	// SecretFields lists the top-level config keys holding credentials. They are
	// stripped from every API response, kept on update when resubmitted empty,
	// and scrubbed out of delivery errors before those are logged or stored.
	SecretFields() []string

	// ValidateConfig rejects bad admin input at save time, so a broken config can
	// never reach the delivery path.
	ValidateConfig(cfg json.RawMessage) error

	// Deliver sends one announcement. It must not retry internally: the pipeline
	// owns backoff, dead-lettering and the delivery log. Returning an error marks
	// the attempt failed.
	Deliver(ctx context.Context, inst Instance, a Announcement) error
}

// SafeDeliver calls c.Deliver and converts a panic into an ordinary error.
//
// Connectors talk to third-party services through third-party code; a nil map
// or an index slip inside one of them must degrade to a failed delivery that
// retries, not take down the worker process (or the admin's test-send request)
// along with every other queued task.
func SafeDeliver(ctx context.Context, c Connector, inst Instance, a Announcement) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("connector %s panicked: %v", c.Kind(), r)
		}
	}()
	return c.Deliver(ctx, inst, a)
}

// PersistentConnector is a Connector that also maintains a long-lived connection
// (IRC). Start blocks until ctx is cancelled; the ConnectorManager owns the
// lifecycle and guarantees only one node runs Start for a given instance.
type PersistentConnector interface {
	Connector
	Start(ctx context.Context, inst Instance) error
}
