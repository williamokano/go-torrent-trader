-- +goose Up
-- Private tracker: one peer per user per torrent. The peer_id changes across
-- client restarts, so it should not be part of the unique constraint.

-- Remove duplicate peers (keep the most recent per user+torrent)
DELETE FROM peers p1
USING peers p2
WHERE p1.torrent_id = p2.torrent_id
  AND p1.user_id = p2.user_id
  AND p1.last_announce < p2.last_announce;

-- Drop the old constraint and create the correct one
DROP INDEX IF EXISTS peers_torrent_id_user_id_peer_id_key;
ALTER TABLE peers DROP CONSTRAINT IF EXISTS peers_torrent_id_user_id_peer_id_key;
ALTER TABLE peers ADD CONSTRAINT peers_torrent_id_user_id_key UNIQUE (torrent_id, user_id);

-- +goose Down
ALTER TABLE peers DROP CONSTRAINT IF EXISTS peers_torrent_id_user_id_key;
ALTER TABLE peers ADD CONSTRAINT peers_torrent_id_user_id_peer_id_key UNIQUE (torrent_id, user_id, peer_id);
