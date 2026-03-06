-- +goose Up
CREATE TABLE categories (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    parent_id  BIGINT REFERENCES categories(id),
    sort_order INT NOT NULL DEFAULT 0,
    image      VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed top-level categories with explicit IDs.
INSERT INTO categories (id, name, sort_order) VALUES
    (1, 'Movies',   1),
    (2, 'TV',       2),
    (3, 'Music',    3),
    (4, 'Games',    4),
    (5, 'Software', 5),
    (6, 'Anime',    6),
    (7, 'Books',    7),
    (8, 'Other',    8);

-- Reset sequence past the explicit IDs before inserting subcategories.
SELECT setval('categories_id_seq', (SELECT MAX(id) FROM categories));

-- Seed subcategories (auto-increment from 9+).
INSERT INTO categories (name, parent_id, sort_order) VALUES
    ('HD',  1, 1),
    ('SD',  1, 2),
    ('4K',  1, 3),
    ('HD',  2, 1),
    ('SD',  2, 2),
    ('MP3', 3, 1),
    ('FLAC', 3, 2),
    ('PC',  4, 1),
    ('Console', 4, 2),
    ('Windows', 5, 1),
    ('macOS', 5, 2),
    ('Linux', 5, 3);

-- +goose Down
DROP TABLE IF EXISTS categories;
