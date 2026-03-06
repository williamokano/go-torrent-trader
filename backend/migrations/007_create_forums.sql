-- +goose Up
CREATE TABLE forums (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    sort_order      INT NOT NULL DEFAULT 0,
    min_group_level INT NOT NULL DEFAULT 0,
    topic_count     INT NOT NULL DEFAULT 0,
    post_count      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE forum_topics (
    id           BIGSERIAL PRIMARY KEY,
    forum_id     BIGINT NOT NULL REFERENCES forums(id),
    user_id      BIGINT NOT NULL REFERENCES users(id),
    title        VARCHAR(255) NOT NULL,
    pinned       BOOLEAN NOT NULL DEFAULT false,
    locked       BOOLEAN NOT NULL DEFAULT false,
    post_count   INT NOT NULL DEFAULT 0,
    last_post_at TIMESTAMPTZ,
    last_post_by BIGINT REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE forum_posts (
    id         BIGSERIAL PRIMARY KEY,
    topic_id   BIGINT NOT NULL REFERENCES forum_topics(id),
    user_id    BIGINT NOT NULL REFERENCES users(id),
    body       TEXT NOT NULL,
    edited_by  BIGINT REFERENCES users(id),
    edited_at  TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS forum_posts;
DROP TABLE IF EXISTS forum_topics;
DROP TABLE IF EXISTS forums;
