-- +goose Up
INSERT INTO site_settings (key, value) VALUES ('tracker_max_peers_per_torrent', '50')
    ON CONFLICT (key) DO NOTHING;
INSERT INTO site_settings (key, value) VALUES ('tracker_max_peers_per_user', '100')
    ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM site_settings WHERE key IN (
    'tracker_max_peers_per_torrent',
    'tracker_max_peers_per_user'
);
