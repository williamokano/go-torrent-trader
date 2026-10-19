-- +goose Up
-- The per-torrent peer cap counted every peer row, a member's own included,
-- and the per-member cap was larger: one member announcing under fifty
-- peer_ids filled a torrent and turned everyone else away. Peers are now also
-- capped per member per torrent, and the per-torrent cap no longer turns away
-- seeders. Its seeded default of 50 also refused the 51st peer of any popular
-- torrent, so a value still at that default becomes 0 (no cap); an operator's
-- own value is kept.
INSERT INTO site_settings (key, value) VALUES ('tracker_max_peers_per_user_torrent', '3')
    ON CONFLICT (key) DO NOTHING;
UPDATE site_settings SET value = '0'
    WHERE key = 'tracker_max_peers_per_torrent' AND value = '50';

-- +goose Down
-- Restores 50 wherever the cap is 0, including an operator's own 0.
UPDATE site_settings SET value = '50'
    WHERE key = 'tracker_max_peers_per_torrent' AND value = '0';
DELETE FROM site_settings WHERE key = 'tracker_max_peers_per_user_torrent';
