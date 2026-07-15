-- +goose Up
-- promotion_rules defines the auto class-promotion ladder. A group is a rung on
-- the ladder if and only if it has a row here; the rung's row holds the
-- thresholds a user must meet to reach (and hold) that class. Ladder order is by
-- the group's level. Staff groups (is_admin / is_moderator) must never get a
-- rule — the service rejects that — so auto-promotion can never land a user in
-- a staff class.
CREATE TABLE promotion_rules (
    group_id       BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    min_ratio      DOUBLE PRECISION NOT NULL DEFAULT 0,
    min_uploaded   BIGINT NOT NULL DEFAULT 0, -- bytes
    min_age_days   INT NOT NULL DEFAULT 0,
    min_torrents   INT NOT NULL DEFAULT 0, -- uploaded torrents
    min_seed_hours INT NOT NULL DEFAULT 0, -- estimated seeding hours within the window
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- promotion_runs is an audit trail of each engine run and the source of the
-- "last run" timestamp used to honour the configurable promotion interval.
CREATE TABLE promotion_runs (
    id       BIGSERIAL PRIMARY KEY,
    ran_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    promoted INT NOT NULL DEFAULT 0,
    demoted  INT NOT NULL DEFAULT 0
);

-- Master switch (off by default), the evaluation cycle, and the window over
-- which seeding time is measured from the announce log.
INSERT INTO site_settings (key, value) VALUES
    ('promotion_enabled', 'false'),
    ('promotion_interval_days', '7'),
    ('promotion_seed_window_days', '30')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM site_settings WHERE key IN ('promotion_enabled', 'promotion_interval_days', 'promotion_seed_window_days');
DROP TABLE IF EXISTS promotion_runs;
DROP TABLE IF EXISTS promotion_rules;
