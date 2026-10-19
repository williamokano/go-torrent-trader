-- +goose Up
-- Migration 039 meant forum_posts.topic_id to cascade, but only inside a
-- CREATE TABLE IF NOT EXISTS that never ran (the table exists from 007), so
-- 007's no-action keys survived and every topic delete failed: a topic always
-- has at least its first post. Same for the two user references that should
-- go to NULL when the user is removed.
--
-- The author keys (forum_topics.user_id, forum_posts.user_id) are left as they
-- are on purpose: 039 also had them cascade, which would let removing one
-- member erase whole threads, other members' replies included.
ALTER TABLE forum_posts DROP CONSTRAINT IF EXISTS forum_posts_topic_id_fkey;
ALTER TABLE forum_posts ADD CONSTRAINT forum_posts_topic_id_fkey
    FOREIGN KEY (topic_id) REFERENCES forum_topics(id) ON DELETE CASCADE;

ALTER TABLE forum_posts DROP CONSTRAINT IF EXISTS forum_posts_edited_by_fkey;
ALTER TABLE forum_posts ADD CONSTRAINT forum_posts_edited_by_fkey
    FOREIGN KEY (edited_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE forum_topics DROP CONSTRAINT IF EXISTS forum_topics_last_post_by_fkey;
ALTER TABLE forum_topics ADD CONSTRAINT forum_topics_last_post_by_fkey
    FOREIGN KEY (last_post_by) REFERENCES users(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE forum_topics DROP CONSTRAINT IF EXISTS forum_topics_last_post_by_fkey;
ALTER TABLE forum_topics ADD CONSTRAINT forum_topics_last_post_by_fkey
    FOREIGN KEY (last_post_by) REFERENCES users(id);

ALTER TABLE forum_posts DROP CONSTRAINT IF EXISTS forum_posts_edited_by_fkey;
ALTER TABLE forum_posts ADD CONSTRAINT forum_posts_edited_by_fkey
    FOREIGN KEY (edited_by) REFERENCES users(id);

ALTER TABLE forum_posts DROP CONSTRAINT IF EXISTS forum_posts_topic_id_fkey;
ALTER TABLE forum_posts ADD CONSTRAINT forum_posts_topic_id_fkey
    FOREIGN KEY (topic_id) REFERENCES forum_topics(id);
