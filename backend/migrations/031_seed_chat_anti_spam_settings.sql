-- +goose Up
INSERT INTO site_settings (key, value) VALUES ('chat_rate_limit_window', '10')
    ON CONFLICT (key) DO NOTHING;
INSERT INTO site_settings (key, value) VALUES ('chat_rate_limit_max', '10')
    ON CONFLICT (key) DO NOTHING;
INSERT INTO site_settings (key, value) VALUES ('chat_spam_strike_count', '3')
    ON CONFLICT (key) DO NOTHING;
INSERT INTO site_settings (key, value) VALUES ('chat_spam_mute_minutes', '5')
    ON CONFLICT (key) DO NOTHING;
INSERT INTO site_settings (key, value) VALUES ('chat_strike_reset_seconds', '60')
    ON CONFLICT (key) DO NOTHING;
INSERT INTO site_settings (key, value) VALUES ('chat_rate_limit_message', 'Slow down! You are sending messages too fast.')
    ON CONFLICT (key) DO NOTHING;
INSERT INTO site_settings (key, value) VALUES ('chat_spam_mute_message', 'You have been automatically muted for flooding the chat.')
    ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM site_settings WHERE key IN (
    'chat_rate_limit_window',
    'chat_rate_limit_max',
    'chat_spam_strike_count',
    'chat_spam_mute_minutes',
    'chat_strike_reset_seconds',
    'chat_rate_limit_message',
    'chat_spam_mute_message'
);
