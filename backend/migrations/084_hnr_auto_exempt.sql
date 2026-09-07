-- +goose Up
-- Automatic hit-and-run exemption: a scheduled rules pass that flags torrents
-- exempt (and un-flags them) by criteria, so staff do not have to tick
-- torrents.hnr_exempt by hand for "well seeded" or "too small to bother".
--
-- The problem this has to avoid is the one docs/PROPOSED_FEATURES.md PF-26
-- describes for auto-freeleech (#197): torrents.hnr_exempt is a bare boolean
-- with no record of who set it, so a rules pass that just writes the column
-- cannot tell its own writes from a staff decision — it would un-exempt a
-- torrent staff deliberately flagged, or re-exempt one staff deliberately
-- un-flagged. So a provenance column is added first.
--
-- hnr_exempt_source:
--   NULL     — never touched. The only rows an auto pass may exempt.
--   'manual' — a staff member set the checkbox (either direction). An auto
--              pass never touches these.
--   'auto'   — a rule set it. An auto pass may clear it back to (false, NULL)
--              when no rule matches any more.
ALTER TABLE torrents
    ADD COLUMN IF NOT EXISTS hnr_exempt_source TEXT
        CHECK (hnr_exempt_source IN ('manual', 'auto'));

-- Every existing *exemption* was set by hand (auto did not exist yet), so mark
-- those 'manual'. One case this cannot recover: a torrent staff deliberately
-- left un-exempt is indistinguishable from one never considered, so both stay
-- NULL and a matching rule may auto-exempt it on the first pass. Unavoidable
-- without an edit history; from here forward the provenance is exact.
UPDATE torrents SET hnr_exempt_source = 'manual' WHERE hnr_exempt = true;

-- The auto pass scans these two partitions of the column every run.
CREATE INDEX IF NOT EXISTS idx_torrents_hnr_exempt_unmanaged
    ON torrents (id) WHERE hnr_exempt_source IS NULL AND hnr_exempt = false;
CREATE INDEX IF NOT EXISTS idx_torrents_hnr_exempt_auto
    ON torrents (id) WHERE hnr_exempt_source = 'auto';

-- hnr_exempt_rules: the criteria, a flat list (not group-keyed like hnr_rules).
-- A torrent is auto-exempt if it matches ANY enabled rule.
--   min_seeders    — exempt once seeders >= threshold (well enough seeded that
--                    one member dropping it does not matter)
--   max_size_bytes — exempt when size <= threshold (the bookkeeping is not
--                    worth it for a tiny torrent)
-- A "dead / no seeders for N days" criterion is deliberately not here yet:
-- nothing in the schema records when a torrent last had a seeder, and #188 is
-- the open issue for agreeing what "dead" even means. It slots in as one more
-- CHECK value and one more predicate in ApplyExemptRules once that lands.
CREATE TABLE IF NOT EXISTS hnr_exempt_rules (
    id         BIGSERIAL PRIMARY KEY,
    criterion  TEXT NOT NULL CHECK (criterion IN ('min_seeders', 'max_size_bytes')),
    threshold  BIGINT NOT NULL CHECK (threshold >= 0),
    enabled    BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The auto pass runs inside the existing HnR daemon sweep; its outcome is
-- tallied on the same hnr_runs row as the breach/satisfy/purge counts.
ALTER TABLE hnr_runs
    ADD COLUMN IF NOT EXISTS torrents_exempted INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS torrents_released INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE hnr_runs
    DROP COLUMN IF EXISTS torrents_exempted,
    DROP COLUMN IF EXISTS torrents_released;
DROP TABLE IF EXISTS hnr_exempt_rules;
-- Undo the auto exemptions before the provenance that marks them disappears —
-- otherwise a rollback leaves them looking hand-set.
UPDATE torrents SET hnr_exempt = false WHERE hnr_exempt_source = 'auto';
DROP INDEX IF EXISTS idx_torrents_hnr_exempt_unmanaged;
DROP INDEX IF EXISTS idx_torrents_hnr_exempt_auto;
ALTER TABLE torrents DROP COLUMN IF EXISTS hnr_exempt_source;
