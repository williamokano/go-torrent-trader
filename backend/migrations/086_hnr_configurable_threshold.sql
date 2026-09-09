-- +goose Up
-- Make the ladder's penalty threshold one number an operator can change,
-- instead of the same figure repeated across five rungs that all have to
-- agree. See #282's follow-up.
--
-- 085 wrote 50 — TorrentLeech's threshold — onto every penalising rung. That
-- is the right default for a site the size of TorrentLeech and far too high
-- for a tracker with a few hundred members, and lowering it meant editing five
-- rows to the same value by hand. Worse, nothing made them agree: the ladder
-- targets the highest rung whose threshold the member's count clears, so a
-- half-finished edit leaving rung 2 at 50 and rung 3 at 10 quietly lands a
-- member with ten obligations on the warning rung.
--
-- min_active_hnr becomes nullable, and NULL means "use the site-wide
-- hnr_penalty_threshold". This is the same shape the per-class clear-pricing
-- overrides already use on hnr_rules (NULL falls back to the hnr_clear_*
-- setting), so a rung that wants its own figure still just sets one.
--
-- Rung 1 keeps an explicit 1: the opening reminder fires on a member's first
-- unmet obligation by design and must not move when an operator retunes what
-- counts as a penalty-worthy pile-up.
ALTER TABLE hnr_penalty_stages ALTER COLUMN min_active_hnr DROP NOT NULL;
ALTER TABLE hnr_penalty_stages ALTER COLUMN min_active_hnr DROP DEFAULT;

INSERT INTO site_settings (key, value) VALUES ('hnr_penalty_threshold', '50')
ON CONFLICT (key) DO NOTHING;

-- Only rungs still carrying 085's 50 defer to the setting. A rung an operator
-- has since given its own figure keeps it — that number is now an explicit
-- override, which is exactly what they meant by typing it.
UPDATE hnr_penalty_stages SET min_active_hnr = NULL, updated_at = NOW()
WHERE min_active_hnr = 50;

-- +goose Down
-- Rungs that were deferring to the setting are pinned back to its current
-- value before the column stops accepting NULL, so a down-migration cannot
-- leave the ladder unrepresentable (or silently reset rungs to 1).
-- +goose StatementBegin
DO $$
DECLARE
    threshold INT;
BEGIN
    SELECT COALESCE(NULLIF(value, '')::INT, 50) INTO threshold
    FROM site_settings WHERE key = 'hnr_penalty_threshold';
    IF threshold IS NULL THEN
        threshold := 50;
    END IF;
    UPDATE hnr_penalty_stages SET min_active_hnr = threshold WHERE min_active_hnr IS NULL;
END $$;
-- +goose StatementEnd

DELETE FROM site_settings WHERE key = 'hnr_penalty_threshold';
ALTER TABLE hnr_penalty_stages ALTER COLUMN min_active_hnr SET DEFAULT 1;
ALTER TABLE hnr_penalty_stages ALTER COLUMN min_active_hnr SET NOT NULL;
