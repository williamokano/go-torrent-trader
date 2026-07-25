-- +goose Up
-- 071 made 'chat' a singleton on the assumption that a second instance could only
-- double-post to the one shoutbox. With per-instance templates and category
-- filters that is no longer true: a site can want a differently worded line for
-- Anime than for Movies, which is two instances with non-overlapping filters.
-- Overlapping filters do post twice — that is now the admin's choice to make.
--
-- 'sse' stays a singleton here. There is one live feed and a second instance
-- would deliver the same announcement to the same subscribers twice.
DROP INDEX IF EXISTS uq_notification_connectors_singleton;

CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_connectors_singleton
    ON notification_connectors (kind) WHERE kind IN ('sse');

-- +goose Down
DROP INDEX IF EXISTS uq_notification_connectors_singleton;

-- Rolling back re-imposes one chat instance, which a database that already has
-- several cannot satisfy. Rather than fail the whole rollback, fall back to the
-- sse-only index: sse's guarantee predates this migration and has nothing to do
-- with chat, so losing it as collateral would be a silent regression. The
-- service-level check still refuses a second chat instance once the old binary
-- is back.
-- +goose StatementBegin
DO $$
BEGIN
    CREATE UNIQUE INDEX uq_notification_connectors_singleton
        ON notification_connectors (kind) WHERE kind IN ('chat', 'sse');
EXCEPTION WHEN unique_violation THEN
    RAISE WARNING 'multiple chat connectors exist; restoring the sse-only singleton index instead';
    CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_connectors_singleton
        ON notification_connectors (kind) WHERE kind IN ('sse');
END
$$;
-- +goose StatementEnd
