-- +goose Up
CREATE TABLE IF NOT EXISTS news (
    id         BIGSERIAL PRIMARY KEY,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL,
    author_id  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    published  BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_news_published_created ON news (published, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS news;
