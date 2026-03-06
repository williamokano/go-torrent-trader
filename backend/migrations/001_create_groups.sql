-- +goose Up
CREATE TABLE groups (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(255) NOT NULL UNIQUE,
    slug       VARCHAR(255) NOT NULL UNIQUE,
    level      INT NOT NULL DEFAULT 0,
    color      VARCHAR(7),
    can_upload   BOOLEAN NOT NULL DEFAULT true,
    can_download BOOLEAN NOT NULL DEFAULT true,
    can_invite   BOOLEAN NOT NULL DEFAULT true,
    can_comment  BOOLEAN NOT NULL DEFAULT true,
    can_forum    BOOLEAN NOT NULL DEFAULT true,
    is_admin     BOOLEAN NOT NULL DEFAULT false,
    is_moderator BOOLEAN NOT NULL DEFAULT false,
    is_immune    BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed default groups.
INSERT INTO groups (name, slug, level, color, can_upload, can_download, can_invite, can_comment, can_forum, is_admin, is_moderator, is_immune) VALUES
    ('Administrator', 'administrator', 100, '#FF0000', true, true, true, true, true, true, false, true),
    ('Moderator',     'moderator',      80, '#00AA00', true, true, true, true, true, false, true, true),
    ('VIP',           'vip',            60, '#FFA500', true, true, true, true, true, false, false, false),
    ('Power User',    'power-user',     40, '#0000FF', true, true, true, true, true, false, false, false),
    ('User',          'user',           20, '#555555', true, true, false, true, true, false, false, false),
    ('Validating',    'validating',     10, '#999999', false, true, false, false, false, false, false, false);

-- +goose Down
DROP TABLE IF EXISTS groups;
