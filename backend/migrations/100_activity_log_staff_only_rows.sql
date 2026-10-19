-- +goose Up
-- Staff-only visibility was decided per event type, but whether an upload,
-- edit or delete may be shown depends on the torrent: one still awaiting
-- moderation is not public yet, and an anonymous uploader is not to be named.
-- Entries now carry their own flag.
ALTER TABLE activity_logs ADD COLUMN IF NOT EXISTS staff_only BOOLEAN NOT NULL DEFAULT false;

-- Existing upload, edit and delete entries about anonymous or not-public
-- torrents named their uploader or the torrent to every member; so did those
-- whose torrent is gone and can no longer be checked. Hide those, and keep
-- members' upload history for torrents that are public and named. (New
-- upload entries are staff's audit line; the public one comes from
-- torrent_published.)
UPDATE activity_logs al SET staff_only = true
WHERE al.event_type IN ('torrent_uploaded', 'torrent_edited', 'torrent_deleted')
  AND NOT EXISTS (
    SELECT 1 FROM torrents t
    WHERE t.id = (al.metadata ->> 'torrent_id')::bigint
      AND NOT t.anonymous AND t.moderation_status = 'approved' AND t.visible
  );

-- +goose Down
ALTER TABLE activity_logs DROP COLUMN IF EXISTS staff_only;
