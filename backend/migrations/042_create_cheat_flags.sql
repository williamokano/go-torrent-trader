-- +goose Up
CREATE TABLE IF NOT EXISTS cheat_flags (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    torrent_id    BIGINT REFERENCES torrents(id) ON DELETE SET NULL,
    flag_type     TEXT NOT NULL,
    details       JSONB NOT NULL DEFAULT '{}',
    dismissed     BOOLEAN NOT NULL DEFAULT FALSE,
    dismissed_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    dismissed_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cheat_flags_user_id ON cheat_flags(user_id);
CREATE INDEX IF NOT EXISTS idx_cheat_flags_dismissed ON cheat_flags(dismissed);
CREATE INDEX IF NOT EXISTS idx_cheat_flags_created_at ON cheat_flags(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_cheat_flags_flag_type ON cheat_flags(flag_type);
CREATE INDEX IF NOT EXISTS idx_cheat_flags_cooldown ON cheat_flags(user_id, torrent_id, flag_type, dismissed, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS cheat_flags;
