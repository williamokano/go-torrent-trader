-- +goose Up
-- External notification connectors (BE-10.1). One row per configured destination
-- (chat, webhook, irc, sse, discord, telegram); `kind` selects the compile-time
-- connector implementation and `config`/`filters` are that connector's admin input.
CREATE TABLE IF NOT EXISTS notification_connectors (
    id         BIGSERIAL PRIMARY KEY,
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT FALSE,
    config     JSONB NOT NULL DEFAULT '{}',
    filters    JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 'chat' and 'sse' broadcast to one shared destination, so a second instance would
-- double-post to the same shoutbox/feed. The service rejects it with a 409 first;
-- this index is the belt-and-braces that also holds against concurrent writers.
CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_connectors_singleton
    ON notification_connectors (kind) WHERE kind IN ('chat', 'sse');

CREATE TABLE IF NOT EXISTS connector_deliveries (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     BIGINT NOT NULL REFERENCES notification_connectors(id) ON DELETE CASCADE,
    -- 'torrent.published:<torrent_id>' for real announcements, 'test:<uuid>' for
    -- admin test-sends (which must never collide with a real event).
    event_key       TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    -- The marshaled Announcement: already anonymous-safe and never contains secrets.
    payload         JSONB NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    attempts        INT NOT NULL DEFAULT 0,
    last_error      TEXT,
    next_attempt_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Dedupe is enforced by the database, not by a check-then-insert in the
-- dispatcher: concurrent dispatches (retry, second node, double approve) race
-- and a read-first guard would let both through.
CREATE UNIQUE INDEX IF NOT EXISTS uq_connector_deliveries_dedupe
    ON connector_deliveries (instance_id, event_key);

-- The drain handler's hot query: due rows for one instance, oldest first. Also
-- serves the maintenance sweep's scan for instances with due work.
CREATE INDEX IF NOT EXISTS idx_connector_deliveries_due
    ON connector_deliveries (instance_id, status, next_attempt_at);

-- The admin delivery log pages by instance, newest first; without this it is a
-- full scan plus a sort on every page load once retention has built up.
CREATE INDEX IF NOT EXISTS idx_connector_deliveries_log
    ON connector_deliveries (instance_id, created_at DESC, id DESC);

-- Retention pruning runs every five minutes over the whole table and only ever
-- touches closed rows.
CREATE INDEX IF NOT EXISTS idx_connector_deliveries_prune
    ON connector_deliveries (created_at) WHERE status <> 'pending';

-- +goose Down
DROP INDEX IF EXISTS idx_connector_deliveries_prune;
DROP INDEX IF EXISTS idx_connector_deliveries_log;
DROP INDEX IF EXISTS idx_connector_deliveries_due;
DROP INDEX IF EXISTS uq_connector_deliveries_dedupe;
DROP TABLE IF EXISTS connector_deliveries;
DROP INDEX IF EXISTS uq_notification_connectors_singleton;
DROP TABLE IF EXISTS notification_connectors;
