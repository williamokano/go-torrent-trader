-- +goose Up
-- moderation_enabled: master switch. When 'false', uploads auto-approve (legacy).
-- moderation_public_visibility: whether a non-author/non-staff may VIEW a pending
-- torrent's detail page. Never unlocks download — pending torrents are never
-- downloadable by anyone but the author and staff.
INSERT INTO site_settings (key, value) VALUES
    ('moderation_enabled', 'true'),
    ('moderation_public_visibility', 'false')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM site_settings WHERE key IN (
    'moderation_enabled',
    'moderation_public_visibility'
);
