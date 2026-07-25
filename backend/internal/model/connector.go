package model

import (
	"encoding/json"
	"time"
)

// NotificationConnector is one configured external announcement destination
// (BE-10). Kind selects the compile-time implementation; Config and Filters are
// that implementation's admin input, stored as JSONB and validated on write.
type NotificationConnector struct {
	ID      int64
	Kind    string
	Name    string
	Enabled bool
	// Config may contain credentials. It is never returned by the API as-is —
	// see connector.RedactConfig and the connector's SecretFields.
	Config    json.RawMessage
	Filters   json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ConnectorDelivery statuses.
//
// A row starts pending and ends sent, coalesced or failed. "coalesced" means the
// announcement was represented by a "+N more" summary because the instance's
// rate budget was exhausted — it was accounted for, not dropped.
const (
	DeliveryPending   = "pending"
	DeliverySent      = "sent"
	DeliveryCoalesced = "coalesced"
	DeliveryFailed    = "failed"
)

// ConnectorDelivery is one announcement's journey to one instance: the audit
// trail behind the admin delivery log, and the queue the drain worker reads.
type ConnectorDelivery struct {
	ID         int64
	InstanceID int64
	// EventKey is unique per instance and is what makes dedupe race-safe:
	// "torrent.published:<torrent_id>" for real events, "test:<uuid>" for
	// admin test-sends.
	EventKey  string
	EventType string
	// Payload is the marshaled connector.Announcement. It never contains a
	// secret, and never the real uploader behind an anonymous upload.
	Payload       json.RawMessage
	Status        string
	Attempts      int
	LastError     *string
	NextAttemptAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
