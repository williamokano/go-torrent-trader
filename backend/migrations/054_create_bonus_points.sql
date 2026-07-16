-- +goose Up
-- Bonus point economy foundation. The balance lives on users for fast reads;
-- every movement (seeding award, store purchase, admin adjustment) also writes
-- an append-only bonus_transactions row — same lesson as the announce log:
-- a cumulative balance can never be decomposed into history after the fact.
ALTER TABLE users ADD COLUMN IF NOT EXISTS bonus_points BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS bonus_transactions (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    delta      BIGINT NOT NULL,  -- positive = award, negative = spend
    reason     TEXT NOT NULL,    -- 'seeding' | 'purchase' | 'admin_adjust'
    ref_id     BIGINT,           -- purchase: store item id; admin_adjust: acting admin's user id
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bonus_transactions_user ON bonus_transactions (user_id, created_at DESC);

-- The store catalogue. freeleech_ticket and double_upload are foundation-only
-- kinds: seeded disabled, and the purchase path rejects them until their
-- effects are implemented.
CREATE TABLE IF NOT EXISTS bonus_store_items (
    id            BIGSERIAL PRIMARY KEY,
    slug          TEXT NOT NULL UNIQUE,
    kind          TEXT NOT NULL CHECK (kind IN ('invite', 'upload_credit', 'freeleech_ticket', 'double_upload')),
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    price         BIGINT NOT NULL CHECK (price > 0),
    reward_amount BIGINT NOT NULL DEFAULT 0,
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    sort          INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO bonus_store_items (slug, kind, name, description, price, reward_amount, enabled, sort) VALUES
    ('invite-1',         'invite',           '1 Invite',            'One invitation to bring a friend aboard.', 5000, 1,          TRUE,  10),
    ('upload-5gib',      'upload_credit',    '5 GiB Upload Credit', 'Adds 5 GiB to your uploaded total.',       3000, 5368709120, TRUE,  20),
    ('freeleech-ticket', 'freeleech_ticket', 'Freeleech Ticket',    'Coming soon.',                             2000, 1,          FALSE, 30),
    ('double-upload',    'double_upload',    'Double Upload (24h)', 'Coming soon.',                             4000, 24,         FALSE, 40)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO site_settings (key, value) VALUES
    ('bonus_enabled', 'false'),
    ('bonus_points_per_seeding_torrent', '1')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM site_settings WHERE key IN ('bonus_enabled', 'bonus_points_per_seeding_torrent');
DROP TABLE IF EXISTS bonus_store_items;
DROP TABLE IF EXISTS bonus_transactions;
ALTER TABLE users DROP COLUMN IF EXISTS bonus_points;
