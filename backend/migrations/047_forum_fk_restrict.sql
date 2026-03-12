-- +goose Up

-- Change forums.category_id FK from CASCADE to RESTRICT
-- so deleting a category with forums is blocked by the DB itself.
ALTER TABLE forums DROP CONSTRAINT IF EXISTS forums_category_id_fkey;
ALTER TABLE forums ADD CONSTRAINT forums_category_id_fkey
    FOREIGN KEY (category_id) REFERENCES forum_categories(id) ON DELETE RESTRICT;

-- Change forum_topics.forum_id FK from CASCADE to RESTRICT
-- so deleting a forum with topics is blocked by the DB itself.
ALTER TABLE forum_topics DROP CONSTRAINT IF EXISTS forum_topics_forum_id_fkey;
ALTER TABLE forum_topics ADD CONSTRAINT forum_topics_forum_id_fkey
    FOREIGN KEY (forum_id) REFERENCES forums(id) ON DELETE RESTRICT;

-- +goose Down

-- Revert to CASCADE
ALTER TABLE forum_topics DROP CONSTRAINT IF EXISTS forum_topics_forum_id_fkey;
ALTER TABLE forum_topics ADD CONSTRAINT forum_topics_forum_id_fkey
    FOREIGN KEY (forum_id) REFERENCES forums(id) ON DELETE CASCADE;

ALTER TABLE forums DROP CONSTRAINT IF EXISTS forums_category_id_fkey;
ALTER TABLE forums ADD CONSTRAINT forums_category_id_fkey
    FOREIGN KEY (category_id) REFERENCES forum_categories(id) ON DELETE CASCADE;
