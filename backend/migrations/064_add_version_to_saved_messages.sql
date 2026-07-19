-- +goose Up
-- Follow-up to BE-7.2 (063_create_saved_messages.sql), closing the gap its
-- own follow-up note in docs/IMPLEMENTATION_TASKS.md called out: Update was
-- last-write-wins with no conflict signal, so the same draft open in two
-- tabs (or two devices) could silently clobber whichever save landed second.
-- A monotonic per-row counter is enough to detect that: the client round-
-- trips the version it last saw, SavedMessageRepo.Update does a conditional
-- `UPDATE ... WHERE id = $1 AND version = $2`, and zero rows affected means
-- someone else's save landed first. No UUID/hash-based token needed — the
-- only concurrent writers are a single user's own sessions, and an integer
-- that increments by exactly 1 per successful update is simplest to reason
-- about and to compare.
ALTER TABLE saved_messages ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE saved_messages DROP COLUMN IF EXISTS version;
