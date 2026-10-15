-- River job queue schema, version 008 (https://riverqueue.com).
--
-- Generated, not hand-written:
--   go run github.com/riverqueue/river/cmd/river@v0.49.0 migrate-get --version 008 --up / --down
--
-- One goose migration per River version, because goose runs each file in its
-- own transaction and River's versions are not safe to combine: 004 adds an
-- enum value that 006 uses, and Postgres refuses a new enum value in the same
-- transaction that added it. River's version 001 is skipped: it only creates
-- river_migration, River's own tracking table, and goose tracks migrations here.
-- A future River schema version is a new goose migration, never an edit here.

-- +goose Up
-- +goose StatementBegin
-- River main migration 008 [up]
-- No-op. PostgreSQL sequences already prevent automatically generated job IDs
-- from being reused.
SELECT 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- River main migration 008 [down]
-- No-op. PostgreSQL sequences already prevent automatically generated job IDs
-- from being reused.
SELECT 1;
-- +goose StatementEnd
