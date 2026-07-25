-- +goose Up
-- connectors_enabled: global kill-switch. When 'false' the dispatcher drops every
-- announcement without touching per-instance `enabled`, so one toggle silences all
-- external notifications and nothing has to be reconfigured to bring them back.
-- connector_delivery_retention_days: how long delivery-log rows are kept; a
-- non-positive value disables pruning.
-- connectors_allow_private_networks: escape hatch for homelab receivers. Default
-- 'false' so the SSRF guard rejects loopback/RFC1918/link-local targets.
INSERT INTO site_settings (key, value) VALUES
    ('connectors_enabled', 'true'),
    ('connector_delivery_retention_days', '30'),
    ('connectors_allow_private_networks', 'false')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM site_settings WHERE key IN (
    'connectors_enabled',
    'connector_delivery_retention_days',
    'connectors_allow_private_networks'
);
