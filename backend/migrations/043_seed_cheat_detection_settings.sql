-- +goose Up
INSERT INTO site_settings (key, value) VALUES
    ('cheat_detection_enabled', 'true'),
    ('cheat_max_upload_speed_mb_s', '100'),
    ('cheat_left_mismatch_tolerance_pct', '10'),
    ('cheat_flag_cooldown_hours', '6')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM site_settings WHERE key IN (
    'cheat_detection_enabled',
    'cheat_max_upload_speed_mb_s',
    'cheat_left_mismatch_tolerance_pct',
    'cheat_flag_cooldown_hours'
);
