-- +goose Up
INSERT INTO site_settings (key, value) VALUES
    ('warning_escalation_enabled', 'false'),
    ('warning_count_restrict', '2'),
    ('warning_count_ban', '3'),
    ('warning_restrict_type', 'download'),
    ('warning_restrict_days', '7')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM site_settings WHERE key IN (
    'warning_escalation_enabled',
    'warning_count_restrict',
    'warning_count_ban',
    'warning_restrict_type',
    'warning_restrict_days'
);
