-- +goose Up
-- Uploader class (BE-8.22c): trusted members staff promote so they can approve
-- their own uploads. Modeled as a capability flag on the group; a dedicated
-- "Uploader" group carries it. The flag is deliberately not exposed in the group
-- management UI — self-approval is granted by moving a user into this class.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS can_self_approve BOOLEAN NOT NULL DEFAULT false;

INSERT INTO groups (name, slug, level, color, can_upload, can_download, can_invite, can_comment, can_forum, is_admin, is_moderator, is_immune, can_self_approve)
VALUES ('Uploader', 'uploader', 50, '#8B5CF6', true, true, true, true, true, false, false, false, true)
ON CONFLICT (slug) DO NOTHING;

-- +goose Down
DELETE FROM groups WHERE slug = 'uploader';
ALTER TABLE groups DROP COLUMN IF EXISTS can_self_approve;
