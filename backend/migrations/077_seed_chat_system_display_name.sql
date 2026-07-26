-- +goose Up
-- chat_system_display_name: the author label shown on authorless shoutbox
-- announcements (the chat connector's system messages). Seeded with the value
-- that was previously compiled in, so this migration changes nothing on its own.
--
-- The row has to exist: the Site Settings admin page renders the rows the API
-- returns, so an unseeded key is invisible and therefore uneditable.
INSERT INTO site_settings (key, value) VALUES
    ('chat_system_display_name', 'System')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM site_settings WHERE key = 'chat_system_display_name';
