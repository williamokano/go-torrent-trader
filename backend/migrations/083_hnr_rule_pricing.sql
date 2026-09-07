-- +goose Up
-- Per-class overrides for the bonus-point price of clearing an open
-- hit-and-run obligation. Until now every class paid the one site-wide
-- formula (hnr_clear_* settings); a stricter class could not be charged more,
-- nor a VIP-adjacent one less.
--
-- Every column is nullable and NULL means "fall back to the site-wide
-- setting", so an all-NULL row — which is what every existing rule and every
-- rule created without touching these fields is — changes nothing. The column
-- CHECKs allow NULL (an unknown compares as neither in nor out of the set).
ALTER TABLE hnr_rules
    ADD COLUMN IF NOT EXISTS clear_pricing_mode TEXT
        CHECK (clear_pricing_mode IN ('fixed', 'deficit')),
    ADD COLUMN IF NOT EXISTS clear_base_points INT
        CHECK (clear_base_points >= 0),
    ADD COLUMN IF NOT EXISTS clear_points_per_gib INT
        CHECK (clear_points_per_gib >= 0),
    ADD COLUMN IF NOT EXISTS clear_points_per_gib_deficit INT
        CHECK (clear_points_per_gib_deficit >= 0);

-- +goose Down
ALTER TABLE hnr_rules
    DROP COLUMN IF EXISTS clear_pricing_mode,
    DROP COLUMN IF EXISTS clear_base_points,
    DROP COLUMN IF EXISTS clear_points_per_gib,
    DROP COLUMN IF EXISTS clear_points_per_gib_deficit;
