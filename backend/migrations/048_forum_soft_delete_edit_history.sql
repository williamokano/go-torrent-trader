-- +goose Up
ALTER TABLE forum_posts ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE forum_posts ADD COLUMN IF NOT EXISTS deleted_by BIGINT REFERENCES users(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS forum_post_edits (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES forum_posts(id) ON DELETE CASCADE,
    edited_by BIGINT NOT NULL REFERENCES users(id) ON DELETE SET NULL,
    old_body TEXT NOT NULL,
    new_body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_forum_post_edits_post_id ON forum_post_edits(post_id);

-- +goose Down
DROP TABLE IF EXISTS forum_post_edits;
ALTER TABLE forum_posts DROP COLUMN IF EXISTS deleted_by;
ALTER TABLE forum_posts DROP COLUMN IF EXISTS deleted_at;
