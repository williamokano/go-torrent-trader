-- +goose Up
-- Audit trail for admin edits of user profile fields. Uploaded, downloaded and
-- invites are the sensitive ones (they move the economy), but every profile
-- field an admin can change is recorded: one row per changed field, holding
-- the old value, the new value and who made the change. Append-only.
--
-- user_id deliberately has NO foreign key: users can be hard-deleted (the
-- cleanup worker prunes enabled=false accounts), and audit rows must survive
-- their subject — otherwise deleting an account destroys the evidence of the
-- edits made to it. changed_by keeps a SET NULL FK plus a username snapshot
-- taken at write time, so the trail also survives the acting admin's deletion.
CREATE TABLE IF NOT EXISTS user_edit_history (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT NOT NULL,
    changed_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    changed_by_username TEXT NOT NULL DEFAULT '',
    field               TEXT NOT NULL,
    old_value           TEXT NOT NULL DEFAULT '',
    new_value           TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_edit_history_user ON user_edit_history (user_id, created_at DESC);
-- Without this, every user deletion seq-scans the (ever-growing) table to
-- resolve the SET NULL.
CREATE INDEX IF NOT EXISTS idx_user_edit_history_changed_by ON user_edit_history (changed_by);

-- +goose Down
DROP TABLE IF EXISTS user_edit_history;
