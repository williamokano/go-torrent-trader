-- +goose Up

-- NOTE ON EDITING A MERGED MIGRATION
-- This migration originally used CREATE TABLE IF NOT EXISTS for forums,
-- forum_topics and forum_posts. Those tables already exist from 007, so the
-- CREATEs silently skipped, the new columns (category_id, last_post_id,
-- view_count, reply_to_post_id) were never added, and the very next statement —
-- an index on forums(category_id) — failed. The chain could not apply to a
-- clean database at all, which broke every fresh install.
--
-- A follow-up migration cannot repair this: goose stops AT 039 and never
-- reaches a later file. Editing in place is the only fix that makes a fresh
-- database migrate. Databases that already recorded 039 as applied will not
-- re-run it and are unaffected.
--
-- The pattern below is CREATE IF NOT EXISTS (for a database that somehow lacks
-- the table) followed by ADD COLUMN IF NOT EXISTS (to bring 007's older shape
-- up to date). Both paths converge on the same schema.

-- Forum categories (display grouping) — genuinely new in this migration.
CREATE TABLE IF NOT EXISTS forum_categories (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed categories before backfilling forums.category_id, which points at them.
INSERT INTO forum_categories (name, sort_order) VALUES
    ('General', 1),
    ('Torrents', 2),
    ('Support', 3)
ON CONFLICT DO NOTHING;

-- Forums. Exists from 007 without category_id or last_post_id.
CREATE TABLE IF NOT EXISTS forums (
    id BIGSERIAL PRIMARY KEY,
    category_id BIGINT NOT NULL REFERENCES forum_categories(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    topic_count INT NOT NULL DEFAULT 0,
    post_count INT NOT NULL DEFAULT 0,
    last_post_id BIGINT,
    min_group_level INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Added nullable first: a NOT NULL column cannot be added to a table that
-- already has rows without a default. Backfilled and tightened below.
ALTER TABLE forums ADD COLUMN IF NOT EXISTS category_id BIGINT REFERENCES forum_categories(id) ON DELETE CASCADE;
ALTER TABLE forums ADD COLUMN IF NOT EXISTS last_post_id BIGINT;

-- 007 allowed a NULL description; the repository scans it into a plain string,
-- which a NULL would fail. Align with the intended NOT NULL DEFAULT ''.
UPDATE forums SET description = '' WHERE description IS NULL;
ALTER TABLE forums ALTER COLUMN description SET DEFAULT '';
ALTER TABLE forums ALTER COLUMN description SET NOT NULL;

-- Any forum predating categories belongs under General.
UPDATE forums SET category_id = (SELECT id FROM forum_categories WHERE name = 'General')
WHERE category_id IS NULL;

ALTER TABLE forums ALTER COLUMN category_id SET NOT NULL;

-- Topics. Exists from 007 without view_count or last_post_id.
CREATE TABLE IF NOT EXISTS forum_topics (
    id BIGSERIAL PRIMARY KEY,
    forum_id BIGINT NOT NULL REFERENCES forums(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    pinned BOOLEAN NOT NULL DEFAULT false,
    locked BOOLEAN NOT NULL DEFAULT false,
    post_count INT NOT NULL DEFAULT 0,
    view_count INT NOT NULL DEFAULT 0,
    last_post_id BIGINT,
    last_post_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE forum_topics ADD COLUMN IF NOT EXISTS view_count INT NOT NULL DEFAULT 0;
ALTER TABLE forum_topics ADD COLUMN IF NOT EXISTS last_post_id BIGINT;

-- Posts. Exists from 007 without reply_to_post_id.
CREATE TABLE IF NOT EXISTS forum_posts (
    id BIGSERIAL PRIMARY KEY,
    topic_id BIGINT NOT NULL REFERENCES forum_topics(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    reply_to_post_id BIGINT REFERENCES forum_posts(id) ON DELETE SET NULL,
    edited_at TIMESTAMPTZ,
    edited_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE forum_posts ADD COLUMN IF NOT EXISTS reply_to_post_id BIGINT REFERENCES forum_posts(id) ON DELETE SET NULL;

-- Per-user privilege flag.
ALTER TABLE users ADD COLUMN IF NOT EXISTS can_forum BOOLEAN NOT NULL DEFAULT true;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_forums_category ON forums(category_id);
CREATE INDEX IF NOT EXISTS idx_forum_topics_forum ON forum_topics(forum_id);
CREATE INDEX IF NOT EXISTS idx_forum_topics_last_post ON forum_topics(last_post_at DESC);
CREATE INDEX IF NOT EXISTS idx_forum_posts_topic ON forum_posts(topic_id);
CREATE INDEX IF NOT EXISTS idx_forum_posts_user ON forum_posts(user_id);

-- Seed the default forum structure.
INSERT INTO forums (category_id, name, description, sort_order)
SELECT c.id, v.name, v.description, v.sort_order
FROM (VALUES
    ('General', 'Announcements', 'Site news and announcements', 1),
    ('General', 'General Discussion', 'Off-topic chat and general discussion', 2),
    ('Torrents', 'Torrent Requests', 'Request torrents from other members', 1),
    ('Torrents', 'Torrent Talk', 'Discuss specific torrents and releases', 2),
    ('Support', 'Help & Support', 'Get help with the site or your client', 1),
    ('Support', 'Bug Reports', 'Report site bugs and issues', 2)
) AS v(cat_name, name, description, sort_order)
JOIN forum_categories c ON c.name = v.cat_name
WHERE NOT EXISTS (SELECT 1 FROM forums f WHERE f.name = v.name);

-- +goose Down

-- Reverses only what this migration added. The forums/forum_topics/forum_posts
-- tables belong to 007 and are dropped by 007's Down, not this one — dropping
-- them here would destroy data this migration did not create.
DROP INDEX IF EXISTS idx_forum_posts_user;
DROP INDEX IF EXISTS idx_forum_posts_topic;
DROP INDEX IF EXISTS idx_forum_topics_last_post;
DROP INDEX IF EXISTS idx_forum_topics_forum;
DROP INDEX IF EXISTS idx_forums_category;

ALTER TABLE users DROP COLUMN IF EXISTS can_forum;
ALTER TABLE forum_posts DROP COLUMN IF EXISTS reply_to_post_id;
ALTER TABLE forum_topics DROP COLUMN IF EXISTS last_post_id;
ALTER TABLE forum_topics DROP COLUMN IF EXISTS view_count;
ALTER TABLE forums DROP COLUMN IF EXISTS last_post_id;
ALTER TABLE forums DROP COLUMN IF EXISTS category_id;

DROP TABLE IF EXISTS forum_categories;
