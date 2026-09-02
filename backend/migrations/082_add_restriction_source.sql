-- +goose Up
-- user_restrictions.source: which system issued a restriction, so a lift can
-- target exactly its own cause instead of inferring "the" active restriction
-- of a type. HnR is about to become the second automated system writing
-- restrictions (warning_escalation is the first) — without this, a user
-- manually restricted for abuse who separately racks up an HnR restriction
-- of the same type could have the manual restriction's effect undone by an
-- HnR lift that only meant to undo its own.
--
-- Default 'manual' preserves the meaning of every historical and
-- staff-issued row unchanged; existing system-issued rows have no reliable
-- way to distinguish their origin after the fact, so they stay 'manual'
-- rather than being guessed at. HasActiveByType / restoreUserFlagIfNone stay
-- source-agnostic on purpose — the privilege flag must reflect "no active
-- restriction from anywhere" — only the lift operation becomes source-scoped.
ALTER TABLE user_restrictions ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual';

CREATE INDEX IF NOT EXISTS idx_user_restrictions_source
    ON user_restrictions (user_id, restriction_type, source) WHERE lifted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_user_restrictions_source;
ALTER TABLE user_restrictions DROP COLUMN IF EXISTS source;
