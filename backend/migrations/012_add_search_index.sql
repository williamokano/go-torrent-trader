-- +goose Up
ALTER TABLE torrents ADD COLUMN IF NOT EXISTS search_vector tsvector;
CREATE INDEX idx_torrents_search ON torrents USING GIN (search_vector);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION torrents_search_vector_update() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('english', COALESCE(NEW.name, ''));
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER torrents_search_vector_trigger
  BEFORE INSERT OR UPDATE OF name ON torrents
  FOR EACH ROW EXECUTE FUNCTION torrents_search_vector_update();

-- Backfill existing rows
UPDATE torrents SET search_vector = to_tsvector('english', COALESCE(name, ''));

-- +goose Down
DROP TRIGGER IF EXISTS torrents_search_vector_trigger ON torrents;
DROP FUNCTION IF EXISTS torrents_search_vector_update();
DROP INDEX IF EXISTS idx_torrents_search;
ALTER TABLE torrents DROP COLUMN IF EXISTS search_vector;
