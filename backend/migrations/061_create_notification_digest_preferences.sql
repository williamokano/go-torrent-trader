-- +goose Up
-- BE-9.13: per-user email digest frequency preference.
--
-- Kept separate from notification_preferences (keyed on user_id + notification_type,
-- one boolean per type) rather than folding into it: digest frequency is a single
-- per-user tri-state value that summarizes across every type, and it needs its own
-- last_digest_sent_at cursor to compute "unread since last digest" -- neither fits
-- the (user_id, notification_type) grain of the existing table.
CREATE TABLE IF NOT EXISTS notification_digest_preferences (
    user_id             BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    frequency           TEXT NOT NULL DEFAULT 'off' CHECK (frequency IN ('off', 'daily', 'weekly')),
    last_digest_sent_at TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Speeds up the periodic digest job's "who is due" scan, which filters on
-- frequency; rows left at the default 'off' never match that scan.
CREATE INDEX IF NOT EXISTS idx_notification_digest_preferences_frequency
    ON notification_digest_preferences(frequency) WHERE frequency != 'off';

-- +goose Down
DROP TABLE IF EXISTS notification_digest_preferences;
