-- +goose Up
-- Torrent submission moderation (BE-8.22a). New uploads land as 'pending' and stay
-- invisible/undownloadable to everyone but the author and staff until approved.
ALTER TABLE torrents
    ADD COLUMN IF NOT EXISTS moderation_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (moderation_status IN ('pending', 'approved', 'rejected')),
    ADD COLUMN IF NOT EXISTS assigned_moderator_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS approved_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;

-- Existing torrents are already public: grandfather them in as approved so the new
-- approved-only public filter doesn't hide the entire catalogue.
UPDATE torrents SET moderation_status = 'approved' WHERE moderation_status = 'pending';

-- Queue lookups filter by status; the public list filter also references it.
CREATE INDEX IF NOT EXISTS idx_torrents_moderation_status ON torrents (moderation_status);
CREATE INDEX IF NOT EXISTS idx_torrents_assigned_moderator ON torrents (assigned_moderator_id)
    WHERE assigned_moderator_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_torrents_assigned_moderator;
DROP INDEX IF EXISTS idx_torrents_moderation_status;
ALTER TABLE torrents
    DROP COLUMN IF EXISTS approved_at,
    DROP COLUMN IF EXISTS approved_by,
    DROP COLUMN IF EXISTS assigned_moderator_id,
    DROP COLUMN IF EXISTS moderation_status;
