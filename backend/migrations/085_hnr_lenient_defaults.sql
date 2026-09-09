-- +goose Up
-- Recalibrate the shipped hit-and-run penalty ladder against TorrentLeech's
-- published HnR rules (wiki.torrentleech.org/doku.php/hnr), the largest
-- private tracker with a public rule set and the benchmark this project should
-- not be harsher than. See #282.
--
-- What 081 shipped: every rung sat at min_active_hnr = 1, so a single torrent a
-- member stopped seeding walked them notify -> warn -> restrict (download,
-- forum and chat for 14 days) -> final notice -> ban in 31 days. TorrentLeech
-- would have shown that member a reminder and nothing else, ever: their ladder
-- only begins at 50 unmet obligations held for more than five consecutive
-- days, takes three account warnings a month apart to disable an account, and
-- removes a warning after a month of good behaviour.
--
-- The new ladder mirrors those numbers and then stops one rung short of them:
--
--   1  notify    >= 1  obligation,  immediately     a reminder, no penalty
--   2  notify    >= 50 obligations, immediately     the threshold is crossed;
--                                                   this rung starts the clock
--   3  warn      >= 50, 5 days at stage 2           TorrentLeech warning 1
--   4  warn      >= 50, 30 days at stage 3          TorrentLeech warning 2
--   5  restrict  >= 50, 30 days at stage 4          (TorrentLeech disables the
--                download, 14 days               account here; we suspend
--                                                   downloading instead)
--   6  ban       >= 50, 30 days at stage 5          30 days later than they
--                                                   would have
--
-- Stage 2 exists to make the five-day clock mean what TorrentLeech's does.
-- min_days_in_prev measures time spent in the *previous* stage, so hanging the
-- warning off stage 1 would fire it the instant a long-idle member crossed 50,
-- their stage-1 dwell having been running for months. A no-penalty rung at the
-- threshold itself resets stage_entered_at on the crossing, so the five days
-- are five days spent at 50+, and dropping back below 50 and returning starts
-- them again.
--
-- The restrict rung narrows to 'download' alone. Forum and chat access has
-- nothing to do with seeding, and taking them was punishment past the point of
-- correcting the behaviour; an operator who wants them can add them back.
--
-- Only applied when the ladder is still exactly what 081 seeded, field for
-- field, message included. An operator who has tuned any rung has made a
-- policy decision, and a migration that overwrote it would be changing site
-- policy behind their back.

-- +goose StatementBegin
DO $$
DECLARE
    intact INT;
    total  INT;
BEGIN
    SELECT count(*) INTO total FROM hnr_penalty_stages;

    SELECT count(*) INTO intact FROM hnr_penalty_stages WHERE
        (stage = 1 AND min_active_hnr = 1 AND min_days_in_prev = 0
         AND action = 'notify' AND restriction_types = '[]' AND restriction_days = 0
         AND message_template =
             'Hi {{username}}, our records show you have not finished seeding a torrent you downloaded ' ||
             'to the required time or ratio. Please resume seeding to avoid further action on your account.')
     OR (stage = 2 AND min_active_hnr = 1 AND min_days_in_prev = 3
         AND action = 'warn' AND restriction_types = '[]' AND restriction_days = 0
         AND message_template =
             'Hi {{username}}, this is a formal warning: you still have an unresolved hit-and-run obligation. ' ||
             'Continued non-compliance will lead to privilege restrictions.')
     OR (stage = 3 AND min_active_hnr = 1 AND min_days_in_prev = 7
         AND action = 'restrict' AND restriction_types = '["download", "forum", "chat"]' AND restriction_days = 14
         AND message_template =
             'Hi {{username}}, your download, forum, and chat privileges have been restricted because of an ' ||
             'unresolved hit-and-run obligation. Resume seeding or clear it to restore your privileges.')
     OR (stage = 4 AND min_active_hnr = 1 AND min_days_in_prev = 14
         AND action = 'final_notice' AND restriction_types = '[]' AND restriction_days = 0
         AND message_template =
             'Hi {{username}}, this is a final notice: your account will be banned if this hit-and-run ' ||
             'obligation remains unresolved.')
     OR (stage = 5 AND min_active_hnr = 1 AND min_days_in_prev = 7
         AND action = 'ban' AND restriction_types = '[]' AND restriction_days = 0
         AND message_template = 'Account banned for an unresolved hit-and-run obligation.');

    IF total <> 5 OR intact <> 5 THEN
        RAISE NOTICE 'hnr_penalty_stages has been customised (% of % rungs at their shipped values); leaving the ladder alone', intact, total;
        RETURN;
    END IF;

    UPDATE hnr_penalty_stages SET
        min_active_hnr = 1, min_days_in_prev = 0, action = 'notify',
        restriction_types = '[]', restriction_days = 0,
        message_template =
            'Hi {{username}}, one of your downloads has not met its seeding requirement yet. This is a ' ||
            'reminder, not a penalty — resume seeding and it clears itself. Nothing further happens unless ' ||
            'unmet obligations pile up.',
        updated_at = NOW()
    WHERE stage = 1;

    UPDATE hnr_penalty_stages SET
        min_active_hnr = 50, min_days_in_prev = 0, action = 'notify',
        restriction_types = '[]', restriction_days = 0,
        message_template =
            'Hi {{username}}, you now have {{count}} unmet seeding obligations. At this level the account is ' ||
            'five days away from a formal warning — resume seeding, or clear obligations with bonus points, ' ||
            'to drop back below the threshold.',
        updated_at = NOW()
    WHERE stage = 2;

    UPDATE hnr_penalty_stages SET
        min_active_hnr = 50, min_days_in_prev = 5, action = 'warn',
        restriction_types = '[]', restriction_days = 0,
        message_template =
            'Hi {{username}}, this is a formal warning: you have kept {{count}} unmet seeding obligations for ' ||
            'more than five days. Bring the count down to avoid a second warning.',
        updated_at = NOW()
    WHERE stage = 3;

    UPDATE hnr_penalty_stages SET
        min_active_hnr = 50, min_days_in_prev = 30, action = 'warn',
        restriction_types = '[]', restriction_days = 0,
        message_template =
            'Hi {{username}}, this is a second formal warning: a month on, you still have {{count}} unmet ' ||
            'seeding obligations. The next step suspends your ability to download.',
        updated_at = NOW()
    WHERE stage = 4;

    UPDATE hnr_penalty_stages SET
        min_active_hnr = 50, min_days_in_prev = 30, action = 'restrict',
        restriction_types = '["download"]', restriction_days = 14,
        message_template =
            'Hi {{username}}, your download privileges have been suspended for {{restriction_days}} days: ' ||
            '{{count}} seeding obligations remain unmet. Seed what you already have, or clear the obligations ' ||
            'with bonus points, and the suspension lifts.',
        updated_at = NOW()
    WHERE stage = 5;

    INSERT INTO hnr_penalty_stages
        (stage, min_active_hnr, min_days_in_prev, action, restriction_types, restriction_days, message_template)
    VALUES
        (6, 50, 30, 'ban', '[]', 0,
         'Account disabled: {{count}} seeding obligations were left unmet through two warnings and a ' ||
         'download suspension.')
    ON CONFLICT (stage) DO NOTHING;
END $$;
-- +goose StatementEnd

-- hnr_warning_expiry_days: how long a hit-and-run warning stays active before
-- the maintenance sweep resolves it. TorrentLeech removes one HnR warning per
-- month of good behaviour; before this, an HnR warning here was permanent and
-- left users.warned set for good, which is strictly harsher than the tracker
-- we are calibrating against. 0 keeps them permanent for an operator who wants
-- that.
INSERT INTO site_settings (key, value) VALUES ('hnr_warning_expiry_days', '30')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
-- Only what this migration added comes back out. The recalibrated rungs are
-- left as they are: by the time a down-migration runs there is no way to tell
-- a value this migration wrote from one an operator has since tuned, and
-- restoring 081's ladder would silently reinstate a one-obligation ban path.
DELETE FROM site_settings WHERE key = 'hnr_warning_expiry_days';
DELETE FROM hnr_penalty_stages WHERE stage = 6;
