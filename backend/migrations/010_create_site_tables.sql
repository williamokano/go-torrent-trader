-- +goose Up
CREATE TABLE news (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT REFERENCES users(id),
    title      VARCHAR(255) NOT NULL,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE reports (
    id          BIGSERIAL PRIMARY KEY,
    reporter_id BIGINT NOT NULL REFERENCES users(id),
    torrent_id  BIGINT REFERENCES torrents(id),
    reason      TEXT NOT NULL,
    resolved    BOOLEAN NOT NULL DEFAULT false,
    resolved_by BIGINT REFERENCES users(id),
    resolved_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE torrent_comments (
    id         BIGSERIAL PRIMARY KEY,
    torrent_id BIGINT NOT NULL REFERENCES torrents(id),
    user_id    BIGINT NOT NULL REFERENCES users(id),
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE countries (
    id   BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(2) NOT NULL UNIQUE
);

CREATE TABLE languages (
    id   BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(5) NOT NULL UNIQUE
);

-- Seed common countries.
INSERT INTO countries (name, code) VALUES
    ('United States', 'US'),
    ('United Kingdom', 'GB'),
    ('Germany', 'DE'),
    ('France', 'FR'),
    ('Spain', 'ES'),
    ('Italy', 'IT'),
    ('Portugal', 'PT'),
    ('Brazil', 'BR'),
    ('Canada', 'CA'),
    ('Australia', 'AU'),
    ('Japan', 'JP'),
    ('South Korea', 'KR'),
    ('China', 'CN'),
    ('India', 'IN'),
    ('Russia', 'RU'),
    ('Netherlands', 'NL'),
    ('Sweden', 'SE'),
    ('Norway', 'NO'),
    ('Denmark', 'DK'),
    ('Finland', 'FI'),
    ('Poland', 'PL'),
    ('Romania', 'RO'),
    ('Turkey', 'TR'),
    ('Mexico', 'MX'),
    ('Argentina', 'AR');

-- Seed common languages.
INSERT INTO languages (name, code) VALUES
    ('English', 'en'),
    ('German', 'de'),
    ('French', 'fr'),
    ('Spanish', 'es'),
    ('Portuguese', 'pt'),
    ('Italian', 'it'),
    ('Japanese', 'ja'),
    ('Korean', 'ko'),
    ('Chinese', 'zh'),
    ('Russian', 'ru'),
    ('Dutch', 'nl'),
    ('Swedish', 'sv'),
    ('Norwegian', 'no'),
    ('Danish', 'da'),
    ('Finnish', 'fi'),
    ('Polish', 'pl'),
    ('Romanian', 'ro'),
    ('Turkish', 'tr'),
    ('Hindi', 'hi'),
    ('Arabic', 'ar');

-- +goose Down
DROP TABLE IF EXISTS torrent_comments;
DROP TABLE IF EXISTS reports;
DROP TABLE IF EXISTS news;
DROP TABLE IF EXISTS languages;
DROP TABLE IF EXISTS countries;
