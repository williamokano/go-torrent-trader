-- +goose Up
-- invite_distribution_rules defines the auto-invite-grant policy per group. A
-- group is eligible for the auto-distribution job if and only if it has a row
-- here. Unlike promotion_rules there is no ladder/ordering between groups —
-- each rule is independently evaluated eligibility criteria plus a per-user
-- invite ceiling. min_downloaded_bytes/max_downloaded_bytes follow
-- promotion_rules' "0 = no constraint" convention for max_downloaded_bytes
-- (unbounded); max_invites is a ceiling, not a threshold, so its default of 0
-- deliberately means "grant nothing" until an admin sets a real cap — the
-- safe default for a freshly added rule row.
CREATE TABLE IF NOT EXISTS invite_distribution_rules (
    group_id             BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    min_ratio            DOUBLE PRECISION NOT NULL DEFAULT 0,
    min_downloaded_bytes BIGINT NOT NULL DEFAULT 0,
    max_downloaded_bytes BIGINT NOT NULL DEFAULT 0, -- 0 = unbounded
    max_invites          INT NOT NULL DEFAULT 0,     -- per-user ceiling; 0 = grant nothing
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- invite_distribution_runs is an audit trail of each engine run and the
-- source of the "last run" timestamp used to honour the configurable
-- interval, mirroring promotion_runs.
CREATE TABLE IF NOT EXISTS invite_distribution_runs (
    id      BIGSERIAL PRIMARY KEY,
    ran_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    granted INT NOT NULL DEFAULT 0
);

-- Master switch (off by default) and the evaluation cycle, mirroring
-- promotion_enabled/promotion_interval_days.
INSERT INTO site_settings (key, value) VALUES
    ('invite_distribution_enabled', 'false'),
    ('invite_distribution_interval_days', '7')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM site_settings WHERE key IN ('invite_distribution_enabled', 'invite_distribution_interval_days');
DROP TABLE IF EXISTS invite_distribution_runs;
DROP TABLE IF EXISTS invite_distribution_rules;
