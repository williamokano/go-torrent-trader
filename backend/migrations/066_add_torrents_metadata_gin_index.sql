-- +goose Up
-- Metadata browse/search filtering (BE-3.13a).
-- A GIN index on torrents.metadata accelerates the JSONB containment predicate
-- (t.metadata @> '{...}') used by equality metadata filters. The default
-- jsonb_ops operator class supports @> (as well as key-existence operators),
-- keeping the index usable if further metadata predicates are added later.
CREATE INDEX IF NOT EXISTS idx_torrents_metadata ON torrents USING GIN (metadata);

-- +goose Down
DROP INDEX IF EXISTS idx_torrents_metadata;
