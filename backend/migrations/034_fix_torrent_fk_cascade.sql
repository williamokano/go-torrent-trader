-- +goose Up
-- Fix FK constraints on torrent-related tables to CASCADE on delete.
-- Previously these were NO ACTION, causing torrent deletion to fail
-- when the torrent had comments, reports, or active peers.

ALTER TABLE torrent_comments DROP CONSTRAINT IF EXISTS torrent_comments_torrent_id_fkey;
ALTER TABLE torrent_comments ADD CONSTRAINT torrent_comments_torrent_id_fkey
    FOREIGN KEY (torrent_id) REFERENCES torrents(id) ON DELETE CASCADE;

ALTER TABLE reports DROP CONSTRAINT IF EXISTS reports_torrent_id_fkey;
ALTER TABLE reports ADD CONSTRAINT reports_torrent_id_fkey
    FOREIGN KEY (torrent_id) REFERENCES torrents(id) ON DELETE CASCADE;

ALTER TABLE peers DROP CONSTRAINT IF EXISTS peers_torrent_id_fkey;
ALTER TABLE peers ADD CONSTRAINT peers_torrent_id_fkey
    FOREIGN KEY (torrent_id) REFERENCES torrents(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE torrent_comments DROP CONSTRAINT IF EXISTS torrent_comments_torrent_id_fkey;
ALTER TABLE torrent_comments ADD CONSTRAINT torrent_comments_torrent_id_fkey
    FOREIGN KEY (torrent_id) REFERENCES torrents(id);

ALTER TABLE reports DROP CONSTRAINT IF EXISTS reports_torrent_id_fkey;
ALTER TABLE reports ADD CONSTRAINT reports_torrent_id_fkey
    FOREIGN KEY (torrent_id) REFERENCES torrents(id);

ALTER TABLE peers DROP CONSTRAINT IF EXISTS peers_torrent_id_fkey;
ALTER TABLE peers ADD CONSTRAINT peers_torrent_id_fkey
    FOREIGN KEY (torrent_id) REFERENCES torrents(id);
