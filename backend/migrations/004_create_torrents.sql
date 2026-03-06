-- +goose Up
CREATE TABLE torrents (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(255) NOT NULL,
    info_hash       BYTEA NOT NULL UNIQUE,
    size            BIGINT NOT NULL,
    description     TEXT,
    nfo             TEXT,
    category_id     BIGINT NOT NULL REFERENCES categories(id),
    uploader_id     BIGINT NOT NULL REFERENCES users(id),
    anonymous       BOOLEAN NOT NULL DEFAULT false,
    seeders         INT NOT NULL DEFAULT 0,
    leechers        INT NOT NULL DEFAULT 0,
    times_completed INT NOT NULL DEFAULT 0,
    comments_count  INT NOT NULL DEFAULT 0,
    visible         BOOLEAN NOT NULL DEFAULT true,
    banned          BOOLEAN NOT NULL DEFAULT false,
    free            BOOLEAN NOT NULL DEFAULT false,
    silver          BOOLEAN NOT NULL DEFAULT false,
    file_count      INT NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_torrents_category ON torrents (category_id);
CREATE INDEX idx_torrents_uploader ON torrents (uploader_id);
CREATE INDEX idx_torrents_created_at ON torrents (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS torrents;
