-- +goose Up
ALTER TABLE forum_posts ADD COLUMN IF NOT EXISTS search_vector tsvector;
ALTER TABLE forum_topics ADD COLUMN IF NOT EXISTS search_vector tsvector;

CREATE INDEX IF NOT EXISTS idx_forum_posts_search ON forum_posts USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_forum_topics_search ON forum_topics USING GIN (search_vector);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION forum_posts_search_vector_update() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('english', COALESCE(NEW.body, ''));
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS forum_posts_search_vector_trigger ON forum_posts;
CREATE TRIGGER forum_posts_search_vector_trigger
  BEFORE INSERT OR UPDATE OF body ON forum_posts
  FOR EACH ROW EXECUTE FUNCTION forum_posts_search_vector_update();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION forum_topics_search_vector_update() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('english', COALESCE(NEW.title, ''));
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS forum_topics_search_vector_trigger ON forum_topics;
CREATE TRIGGER forum_topics_search_vector_trigger
  BEFORE INSERT OR UPDATE OF title ON forum_topics
  FOR EACH ROW EXECUTE FUNCTION forum_topics_search_vector_update();

-- Backfill existing rows
UPDATE forum_posts SET search_vector = to_tsvector('english', COALESCE(body, ''));
UPDATE forum_topics SET search_vector = to_tsvector('english', COALESCE(title, ''));

-- +goose Down
DROP TRIGGER IF EXISTS forum_posts_search_vector_trigger ON forum_posts;
DROP FUNCTION IF EXISTS forum_posts_search_vector_update();
DROP TRIGGER IF EXISTS forum_topics_search_vector_trigger ON forum_topics;
DROP FUNCTION IF EXISTS forum_topics_search_vector_update();
DROP INDEX IF EXISTS idx_forum_posts_search;
DROP INDEX IF EXISTS idx_forum_topics_search;
ALTER TABLE forum_posts DROP COLUMN IF EXISTS search_vector;
ALTER TABLE forum_topics DROP COLUMN IF EXISTS search_vector;
