-- +goose Up
-- Staff-only visibility was decided per event type, but whether an upload,
-- edit or delete may be shown depends on the torrent: one still awaiting
-- moderation is not public yet, and an anonymous uploader is not to be named.
-- Entries now carry their own flag.
ALTER TABLE activity_logs ADD COLUMN IF NOT EXISTS staff_only BOOLEAN NOT NULL DEFAULT false;

-- Existing entries about anonymous or not-yet-approved torrents named their
-- uploader or the torrent to every member; hide them.
UPDATE activity_logs al SET staff_only = true
FROM torrents t
WHERE al.event_type IN ('torrent_uploaded', 'torrent_edited', 'torrent_deleted')
  AND al.metadata IS NOT NULL
  AND (al.metadata::jsonb ->> 'torrent_id')::bigint = t.id
  AND (t.anonymous OR t.moderation_status <> 'approved');

-- +goose Down
ALTER TABLE activity_logs DROP COLUMN IF EXISTS staff_only;
