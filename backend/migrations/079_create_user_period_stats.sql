-- +goose Up
-- 052 created announce_events as an append-only log and left retention advisory:
-- announce_log_retention_days was seeded and validated, and nothing read it. The
-- table therefore grew without bound, with two indexes compounding the write
-- amplification, on the busiest write path the site has.
--
-- Pruning the raw log destroys the only record of what a member transferred in a
-- given month, so the prune cannot ship alone. These two tables are what make it
-- safe: user_period_stats keeps the monthly aggregate forever, and
-- announce_rollup_state records how far the aggregation has got so the prune can
-- refuse to delete anything not yet counted.

-- Monthly per-user transfer aggregates. Kept indefinitely: they are derived from
-- personal data but are not themselves personal beyond the user link, and they
-- answer the time-windowed questions (leaderboards, goals, site health, "how much
-- did I upload in June") that would otherwise have to scan the raw log at page
-- speed.
CREATE TABLE IF NOT EXISTS user_period_stats (
    user_id            BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- 'YYYY-MM'. A fixed-width text month sorts chronologically, displays without
    -- conversion, and cannot be mistaken for a day the way a truncated DATE can.
    year_month         CHAR(7)     NOT NULL CHECK (year_month ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    -- Sums of the per-announce deltas, not client-reported cumulative totals.
    uploaded           BIGINT      NOT NULL DEFAULT 0,
    downloaded         BIGINT      NOT NULL DEFAULT 0,
    -- After freeleech discount: what actually counted toward ratio. Kept separate
    -- because "what you moved" and "what you were charged for" diverge whenever a
    -- torrent is free, and a ratio recomputed from the raw figure would be wrong.
    counted_downloaded BIGINT      NOT NULL DEFAULT 0,
    announces          BIGINT      NOT NULL DEFAULT 0,
    seed_announces     BIGINT      NOT NULL DEFAULT 0,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, year_month)
);

-- The primary key serves "one member across months". Leaderboards and site-health
-- pages ask the transposed question — one month across all members — which would
-- otherwise be a full scan.
CREATE INDEX IF NOT EXISTS idx_user_period_stats_month ON user_period_stats (year_month);

-- How far the rollup has aggregated, as an exclusive upper date bound: every
-- announce strictly before this date is already counted in user_period_stats.
-- Deliberately one row — the watermark is global, and a table with a key that can
-- only hold one value says so in a way a bare row count does not.
CREATE TABLE IF NOT EXISTS announce_rollup_state (
    id             BOOLEAN     PRIMARY KEY DEFAULT true CHECK (id),
    rolled_through DATE        NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed at the oldest announce still on disk so a deployment with history
-- aggregates all of it before the first prune can touch anything. A fresh install
-- has no rows and starts from today, which is correct: there is nothing behind it.
INSERT INTO announce_rollup_state (id, rolled_through)
SELECT true, (COALESCE(MIN(announced_at), NOW()) AT TIME ZONE 'UTC')::date
FROM announce_events
ON CONFLICT (id) DO NOTHING;

-- +goose Down
-- Rolling this back is lossy in a way most rollbacks are not: once the prune has
-- run, the monthly totals are the only surviving record of the announces it
-- deleted, and dropping them cannot be undone by re-applying. A re-apply reseeds
-- the watermark from the oldest announce still on disk and re-aggregates from
-- there, so only the pruned months are gone — permanently. Take a backup first.
DROP TABLE IF EXISTS announce_rollup_state;
DROP TABLE IF EXISTS user_period_stats;
