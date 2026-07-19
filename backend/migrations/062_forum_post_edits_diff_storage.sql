-- +goose Up
-- BE-9.23: forum_post_edits moves from storing a full old_body snapshot per
-- edit to storing a reversible diff against the version immediately after it.
-- forum_posts.body is always the latest version; history is reconstructed by
-- walking edits backward from the latest, applying each stored diff in
-- reverse. old_body/new_body become nullable rather than dropped: rows
-- written before this migration keep their full snapshots as-is (no lossy
-- backfill), while every new row populates diff instead. The check
-- constraint enforces that every row is one format or the other.
ALTER TABLE forum_post_edits ADD COLUMN IF NOT EXISTS diff TEXT;
ALTER TABLE forum_post_edits ADD COLUMN IF NOT EXISTS reason TEXT;
ALTER TABLE forum_post_edits ALTER COLUMN old_body DROP NOT NULL;
ALTER TABLE forum_post_edits ALTER COLUMN new_body DROP NOT NULL;

-- Postgres has no ADD CONSTRAINT IF NOT EXISTS, so drop-then-add keeps this
-- resilient to a partial re-run (crash after the constraint lands but before
-- goose records the version) instead of failing with "already exists".
ALTER TABLE forum_post_edits DROP CONSTRAINT IF EXISTS forum_post_edits_diff_or_snapshot;
ALTER TABLE forum_post_edits ADD CONSTRAINT forum_post_edits_diff_or_snapshot
    CHECK (diff IS NOT NULL OR old_body IS NOT NULL);

-- +goose Down
-- Only safe if no diff-only rows exist (i.e. nothing was written after this
-- migration shipped) — mirrors the caveat already noted in migration 049.
ALTER TABLE forum_post_edits DROP CONSTRAINT IF EXISTS forum_post_edits_diff_or_snapshot;
ALTER TABLE forum_post_edits ALTER COLUMN new_body SET NOT NULL;
ALTER TABLE forum_post_edits ALTER COLUMN old_body SET NOT NULL;
ALTER TABLE forum_post_edits DROP COLUMN IF EXISTS reason;
ALTER TABLE forum_post_edits DROP COLUMN IF EXISTS diff;
