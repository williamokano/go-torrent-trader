-- +goose Up
-- Fix: restore (torrent_id, user_id, peer_id) uniqueness.
-- A user CAN have multiple peers per torrent (seedbox + home PC).
-- The previous migration incorrectly changed this to (torrent_id, user_id).
-- This migration handles both cases: fresh installs (already correct) and
-- upgrades from the broken constraint.
ALTER TABLE peers DROP CONSTRAINT IF EXISTS peers_torrent_id_user_id_key;

-- Re-add the correct constraint (idempotent — will no-op if it already exists
-- on a fresh install where migration 017 never ran with the old version).
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'peers_torrent_id_user_id_peer_id_key'
    ) THEN
        ALTER TABLE peers ADD CONSTRAINT peers_torrent_id_user_id_peer_id_key UNIQUE (torrent_id, user_id, peer_id);
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE peers DROP CONSTRAINT IF EXISTS peers_torrent_id_user_id_peer_id_key;
ALTER TABLE peers ADD CONSTRAINT peers_torrent_id_user_id_key UNIQUE (torrent_id, user_id);
