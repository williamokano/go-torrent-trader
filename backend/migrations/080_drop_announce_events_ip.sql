-- +goose Up
-- Stop keeping a per-announce history of members' IP addresses.
--
-- Migration 052 added announce_events.ip on the theory that the log doubled as a
-- "seen at this address/time" trail. Nothing was ever built on it: the peer list is
-- served from the peers table, which carries its own ip and expires with the swarm;
-- IP bans are enforced at login and registration against the bans table; quick-ban
-- reads users.ip. Across the whole codebase the column was written on every announce
-- and read by nothing.
--
-- That made it the worst combination available — the highest-write column on the
-- busiest table, retained for months, valuable only to someone who should not have
-- the database. Dropping it removes that, and shrinks the rows the 079 prune has to
-- walk. Members' addresses are still known to the tracker for as long as answering
-- an announce requires, and no longer.
--
-- DROP COLUMN is metadata-only in PostgreSQL: it does not rewrite the table, so this
-- is fast even on a log with millions of rows. The stored values become unreachable
-- immediately and are reclaimed as those pages are next rewritten.
ALTER TABLE announce_events DROP COLUMN IF EXISTS ip;

-- +goose Down
-- Restored nullable, and empty. The addresses that were here are gone: the Up
-- dropped them rather than parking them somewhere, which is the point of it. There
-- is no source to backfill from, and the tracker no longer writes the column, so a
-- rollback leaves the log without addresses either way.
ALTER TABLE announce_events ADD COLUMN IF NOT EXISTS ip INET;
