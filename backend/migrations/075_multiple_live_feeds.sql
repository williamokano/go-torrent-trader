-- +goose Up
-- 071 made 'sse' a singleton because there was one live feed. There can now be
-- several: each instance carries a slug that becomes its stream URL, and its own
-- filters decide what appears in it — "everything", "everything except 18+",
-- "just anime". Two instances no longer double-deliver to the same subscribers,
-- because a subscriber picks one feed by slug.
DROP INDEX IF EXISTS uq_notification_connectors_singleton;

-- Any feed that predates this change keeps working under the name the unslugged
-- route still resolves.
UPDATE notification_connectors
   SET config = jsonb_set(COALESCE(config, '{}'::jsonb), '{slug}', '"default"', true)
 WHERE kind = 'sse'
   AND COALESCE(config->>'slug', '') = '';

-- The slug is the URL, so two feeds sharing one would silently serve each other's
-- subscribers. The service rejects a duplicate with a 409 first; this is the
-- backstop that also holds against two concurrent writers.
--
-- Indexed on the *effective* slug, matching what the application reads: a row
-- with no slug key would otherwise index as NULL, and Postgres treats NULLs as
-- distinct — so any number of them could coexist while the code resolved every
-- one of them to "default". Trimmed for the same reason: two rows differing only
-- in padding are one feed as far as the URL is concerned.
CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_connectors_sse_slug
    ON notification_connectors ((COALESCE(NULLIF(btrim(config->>'slug'), ''), 'default')))
    WHERE kind = 'sse';

-- +goose Down
DROP INDEX IF EXISTS uq_notification_connectors_sse_slug;

-- Strip the slug the Up added. The pre-075 binary's ValidateConfig rejects every
-- key but rate_per_min, and the admin form posts the stored config back verbatim
-- — so leaving it would make the surviving feed unsaveable, with no UI to remove
-- the offending key.
UPDATE notification_connectors SET config = config - 'slug' WHERE kind = 'sse';

-- Rolling back re-imposes one live feed, which a database that already has
-- several cannot satisfy. Restore the sse-only index 074 left in place, and
-- leave it off rather than failing the rollback when the data will not fit.
-- +goose StatementBegin
DO $$
BEGIN
    CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_connectors_singleton
        ON notification_connectors (kind) WHERE kind IN ('sse');
EXCEPTION WHEN unique_violation THEN
    RAISE WARNING 'multiple live feeds exist; singleton index not restored';
END
$$;
-- +goose StatementEnd
