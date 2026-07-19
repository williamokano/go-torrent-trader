-- +goose Up
-- Backs BE-7.2 (PM drafts & templates). A draft is a specific in-progress
-- message (may carry a receiver/parent_id for a reply-in-progress) saved to
-- finish later; a template is a reusable pattern with no fixed recipient,
-- loaded to start a new compose. They share an identical shape, so one table
-- with a `kind` discriminator avoids duplicating the CRUD/list/delete
-- plumbing two near-identical tables would need.
-- The CHECK below keeps "a template is never addressed to anyone" true at the
-- schema level, not just in service-layer validation (backend/internal/
-- service/saved_message.go's build()) — so the invariant survives even a
-- future raw query or a bug in that validation.
CREATE TABLE IF NOT EXISTS saved_messages (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        VARCHAR(20) NOT NULL CHECK (kind IN ('draft', 'template')),
    receiver_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    subject     VARCHAR(255) NOT NULL DEFAULT '',
    body        TEXT NOT NULL DEFAULT '',
    parent_id   BIGINT REFERENCES messages(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (kind <> 'template' OR (receiver_id IS NULL AND parent_id IS NULL))
);

CREATE INDEX IF NOT EXISTS idx_saved_messages_user_kind ON saved_messages (user_id, kind, updated_at DESC);

-- Mirrors messages' idx_messages_receiver/idx_messages_sender (008_create_
-- messages.sql): ON DELETE SET NULL has to touch every row referencing the
-- deleted user/message, so these keep that maintenance an index scan instead
-- of a sequential one as the table grows.
CREATE INDEX IF NOT EXISTS idx_saved_messages_receiver ON saved_messages (receiver_id) WHERE receiver_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_saved_messages_parent ON saved_messages (parent_id) WHERE parent_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS saved_messages;
