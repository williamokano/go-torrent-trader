-- +goose Up
-- Hit-and-run (HnR) tracking. A member who snatches a torrent and does not
-- seed it to the required time or ratio is tracked per-snatch, escalated
-- leniently through a penalty ladder, and can pay off obligations with bonus
-- points. See docs/TRACKER_MODS.md's "Hit-and-Run (HnR) tracking" entry.
--
-- Nothing in the schema records how long a member has seeded a torrent:
-- peers rows are deleted on stop and reaped after 45 minutes of silence,
-- transfer_history is written only on the completed event (never refreshed
-- by a later announce), and announce_events is pruned on a 90-day default
-- retention. hnr_records is therefore its own accumulator, fed by the
-- announce path directly rather than derived after the fact.

-- hnr_rules: per-class policy. A group is subject to HnR if and only if it
-- has a row here, mirroring promotion_rules — a class with no rule (e.g. VIP)
-- is exempt without any special-case code.
CREATE TABLE IF NOT EXISTS hnr_rules (
    group_id               BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    required_seed_hours    INT NOT NULL DEFAULT 0,
    required_ratio         DOUBLE PRECISION NOT NULL DEFAULT 0,
    inactivity_grace_hours INT NOT NULL DEFAULT 48,
    max_days_to_satisfy    INT NOT NULL DEFAULT 0, -- 0 = no hard cap
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- hnr_records: one row per (user, torrent) snatch — the accumulator and the
-- state machine. state transitions:
--   active -> hnr        (daemon: inactivity grace or hard cap exceeded, unmet)
--   hnr -> active         (announce path: a seeding announce is proof of resumed seeding)
--   active|hnr -> satisfied (daemon or announce path: policy met)
--   active|hnr -> cleared   (member pays with bonus points)
--   active|hnr -> waived    (staff exempts the torrent after the snatch)
-- ON DELETE CASCADE on torrent_id (unlike transfer_history's SET NULL): an
-- obligation to seed a torrent that no longer exists is unsatisfiable, so it
-- must disappear rather than become permanently unclearable.
CREATE TABLE IF NOT EXISTS hnr_records (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    torrent_id     BIGINT NOT NULL REFERENCES torrents(id) ON DELETE CASCADE,
    state          TEXT NOT NULL DEFAULT 'active'
                   CHECK (state IN ('active', 'hnr', 'satisfied', 'cleared', 'waived')),
    completed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    seeded_seconds BIGINT NOT NULL DEFAULT 0,
    uploaded       BIGINT NOT NULL DEFAULT 0,
    breached_at    TIMESTAMPTZ,
    resolved_at    TIMESTAMPTZ,
    UNIQUE (user_id, torrent_id)
);

-- Partial index: the daemon and the announce-path accumulation UPDATE both
-- only ever touch open obligations, so resolved rows (the large majority over
-- time) fall out of the index and the scan stays bounded regardless of table
-- size.
CREATE INDEX IF NOT EXISTS idx_hnr_records_open
    ON hnr_records (state, last_seen_at) WHERE state IN ('active', 'hnr');

-- Staff aggregate ("how many active HnR", "top offenders") groups by user
-- filtered to state='hnr'.
CREATE INDEX IF NOT EXISTS idx_hnr_records_user_state ON hnr_records (user_id, state);

-- hnr_penalty_stages: the site-wide, ordered penalty ladder. De-escalates
-- automatically when a user's active-HnR count falls back below a stage's
-- threshold.
CREATE TABLE IF NOT EXISTS hnr_penalty_stages (
    stage             INT PRIMARY KEY,
    min_active_hnr    INT NOT NULL DEFAULT 1,
    min_days_in_prev  INT NOT NULL DEFAULT 0,
    action            TEXT NOT NULL CHECK (action IN ('notify', 'warn', 'restrict', 'final_notice', 'ban')),
    restriction_types TEXT[] NOT NULL DEFAULT '{}',
    restriction_days  INT NOT NULL DEFAULT 0, -- 0 = indefinite
    message_template  TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- hnr_user_state: per-user ladder position. The compare-and-swap target that
-- makes the daemon's escalation/de-escalation idempotent across instances.
CREATE TABLE IF NOT EXISTS hnr_user_state (
    user_id             BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    stage               INT NOT NULL DEFAULT 0, -- 0 = not on the ladder
    stage_entered_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_notified_stage INT NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- hnr_runs: the daemon's run log — status, trigger, and outcome counts, so
-- staff have visibility into when the job last ran and can force a run.
CREATE TABLE IF NOT EXISTS hnr_runs (
    id              BIGSERIAL PRIMARY KEY,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ,
    status          TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'failed')),
    trigger         TEXT NOT NULL DEFAULT 'schedule' CHECK (trigger IN ('schedule', 'manual')),
    triggered_by    BIGINT REFERENCES users(id) ON DELETE SET NULL,
    scanned         INT NOT NULL DEFAULT 0,
    breached        INT NOT NULL DEFAULT 0,
    satisfied       INT NOT NULL DEFAULT 0,
    stages_advanced INT NOT NULL DEFAULT 0,
    stages_decayed  INT NOT NULL DEFAULT 0,
    purged          INT NOT NULL DEFAULT 0,
    error           TEXT
);

CREATE INDEX IF NOT EXISTS idx_hnr_runs_started_at ON hnr_runs (started_at DESC);

-- torrents.hnr_exempt: staff-settable per-torrent exemption, the same shape
-- as free/silver. A torrent nobody should be punished for dropping.
ALTER TABLE torrents ADD COLUMN IF NOT EXISTS hnr_exempt BOOLEAN NOT NULL DEFAULT false;

-- Master switch (off by default — tracking starts only from here forward,
-- deliberately no backfill of existing transfer_history) plus the tunables.
INSERT INTO site_settings (key, value) VALUES
    ('hnr_enabled', 'false'),
    ('hnr_grace_after_complete_hours', '0'),
    ('hnr_seed_credit_cap_minutes', '45'),
    ('hnr_retention_days', '180'),
    ('hnr_exempt_donors', 'false'),
    ('hnr_clear_pricing_mode', 'fixed'),
    ('hnr_clear_base_points', '50'),
    ('hnr_clear_points_per_gib', '10'),
    ('hnr_clear_points_per_gib_deficit', '25')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM site_settings WHERE key IN (
    'hnr_enabled', 'hnr_grace_after_complete_hours', 'hnr_seed_credit_cap_minutes',
    'hnr_retention_days', 'hnr_exempt_donors', 'hnr_clear_pricing_mode',
    'hnr_clear_base_points', 'hnr_clear_points_per_gib', 'hnr_clear_points_per_gib_deficit'
);
ALTER TABLE torrents DROP COLUMN IF EXISTS hnr_exempt;
DROP TABLE IF EXISTS hnr_runs;
DROP TABLE IF EXISTS hnr_user_state;
DROP TABLE IF EXISTS hnr_penalty_stages;
DROP TABLE IF EXISTS hnr_records;
DROP TABLE IF EXISTS hnr_rules;
