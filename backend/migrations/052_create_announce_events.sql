-- +goose Up
-- announce_events is an append-only log of every tracker announce. The peers
-- table is overwritten on each announce and transfer_history keeps one
-- overwritten row per (user, torrent), so both destroy the per-announce deltas
-- at write time. Time-windowed questions — "how much did user X upload in June",
-- bonus reconciliation, per-user data export — are unanswerable from those
-- projections once the input is gone. This log preserves the raw stream so they
-- can be reconstructed after the fact.
CREATE TABLE announce_events (
    id                       BIGSERIAL PRIMARY KEY,
    user_id                  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    torrent_id               BIGINT REFERENCES torrents(id) ON DELETE SET NULL,
    peer_id                  BYTEA NOT NULL,
    ip                       INET NOT NULL,
    port                     INT NOT NULL,
    event                    VARCHAR(16) NOT NULL,
    uploaded                 BIGINT NOT NULL DEFAULT 0,
    downloaded               BIGINT NOT NULL DEFAULT 0,
    left_bytes               BIGINT NOT NULL DEFAULT 0,
    uploaded_delta           BIGINT NOT NULL DEFAULT 0,
    downloaded_delta         BIGINT NOT NULL DEFAULT 0,
    counted_downloaded_delta BIGINT NOT NULL DEFAULT 0,
    seeder                   BOOLEAN NOT NULL DEFAULT false,
    announced_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Per-user chronological access powers data export, reconciliation, and
-- monthly-window aggregates.
CREATE INDEX idx_announce_events_user ON announce_events (user_id, announced_at DESC);
-- Retention housekeeping scans by age. Cleanup is manual for now — see the
-- announce_log_retention_days setting.
CREATE INDEX idx_announce_events_announced_at ON announce_events (announced_at);

-- Capture toggle and retention window. Retention is advisory only: no automatic
-- deletion job runs yet, so the value records the intended house-cleaning
-- cadence for when one is added.
INSERT INTO site_settings (key, value) VALUES
    ('announce_log_enabled', 'true'),
    ('announce_log_retention_days', '90')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM site_settings WHERE key IN ('announce_log_enabled', 'announce_log_retention_days');
DROP TABLE IF EXISTS announce_events;
