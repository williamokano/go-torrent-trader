-- The stats:recalc job is gone (#289): it only warmed the Redis stats cache,
-- and there is no cache to warm. A job of that kind already queued — inserted
-- by the previous release's schedule, or waiting on a retry — would otherwise
-- sit in river_job with no worker for it, logged as an unknown kind on every
-- attempt until River gave up on it.

-- +goose Up
DELETE FROM river_job
WHERE kind = 'stats:recalc'
  AND state NOT IN ('completed', 'cancelled', 'discarded');

-- +goose Down
-- Nothing to restore: the deleted jobs had no worker left to run them.
SELECT 1;
