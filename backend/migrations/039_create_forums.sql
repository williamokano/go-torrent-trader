-- +goose Up

-- Forum categories (display grouping)
CREATE TABLE forum_categories (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Forums (where topics live)
CREATE TABLE forums (
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

-- Topics
CREATE TABLE forum_topics (
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

-- Posts (flat, not threaded)
CREATE TABLE forum_posts (
    id BIGSERIAL PRIMARY KEY,
    topic_id BIGINT NOT NULL REFERENCES forum_topics(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    reply_to_post_id BIGINT REFERENCES forum_posts(id) ON DELETE SET NULL,
    edited_at TIMESTAMPTZ,
    edited_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add can_forum to users table (per-user privilege flag)
ALTER TABLE users ADD COLUMN IF NOT EXISTS can_forum BOOLEAN NOT NULL DEFAULT true;

-- Indexes
CREATE INDEX idx_forums_category ON forums(category_id);
CREATE INDEX idx_forum_topics_forum ON forum_topics(forum_id);
CREATE INDEX idx_forum_topics_last_post ON forum_topics(last_post_at DESC);
CREATE INDEX idx_forum_posts_topic ON forum_posts(topic_id);
CREATE INDEX idx_forum_posts_user ON forum_posts(user_id);

-- Seed default forum structure
INSERT INTO forum_categories (name, sort_order) VALUES
    ('General', 1),
    ('Torrents', 2),
    ('Support', 3);

INSERT INTO forums (category_id, name, description, sort_order) VALUES
    (1, 'Announcements', 'Site news and announcements', 1),
    (1, 'General Discussion', 'Off-topic chat and general discussion', 2),
    (2, 'Torrent Requests', 'Request torrents from other members', 1),
    (2, 'Torrent Talk', 'Discuss specific torrents and releases', 2),
    (3, 'Help & Support', 'Get help with the site or your client', 1),
    (3, 'Bug Reports', 'Report site bugs and issues', 2);

-- +goose Down
DROP INDEX IF EXISTS idx_forum_posts_user;
DROP INDEX IF EXISTS idx_forum_posts_topic;
DROP INDEX IF EXISTS idx_forum_topics_last_post;
DROP INDEX IF EXISTS idx_forum_topics_forum;
DROP INDEX IF EXISTS idx_forums_category;
ALTER TABLE users DROP COLUMN IF EXISTS can_forum;
DROP TABLE IF EXISTS forum_posts;
DROP TABLE IF EXISTS forum_topics;
DROP TABLE IF EXISTS forums;
DROP TABLE IF EXISTS forum_categories;
