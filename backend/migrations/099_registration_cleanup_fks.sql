-- +goose Up
-- The cleanup job deletes registrations that were never activated, but every
-- registration writes an activity log entry with itself as actor, and
-- activity_logs.actor_id refused the delete — one blocked row failed the
-- whole statement, so no expired registration was ever removed. These are the
-- rows a never-activated account can have; each now gives way to its delete.

-- The entry stays; it just no longer points at an account that is gone
-- (actor_id was made nullable for this in 032).
ALTER TABLE activity_logs DROP CONSTRAINT IF EXISTS activity_logs_actor_id_fkey;
ALTER TABLE activity_logs ADD CONSTRAINT activity_logs_actor_id_fkey
    FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE SET NULL;

-- The invite that registered it goes with it: already spent, so it cannot be
-- handed out again.
ALTER TABLE invites DROP CONSTRAINT IF EXISTS invites_used_by_id_fkey;
ALTER TABLE invites ADD CONSTRAINT invites_used_by_id_fkey
    FOREIGN KEY (used_by_id) REFERENCES users(id) ON DELETE CASCADE;

-- A password reset it asked for, and messages sent to it.
ALTER TABLE password_resets DROP CONSTRAINT IF EXISTS password_resets_user_id_fkey;
ALTER TABLE password_resets ADD CONSTRAINT password_resets_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_receiver_id_fkey;
ALTER TABLE messages ADD CONSTRAINT messages_receiver_id_fkey
    FOREIGN KEY (receiver_id) REFERENCES users(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_receiver_id_fkey;
ALTER TABLE messages ADD CONSTRAINT messages_receiver_id_fkey
    FOREIGN KEY (receiver_id) REFERENCES users(id);

ALTER TABLE password_resets DROP CONSTRAINT IF EXISTS password_resets_user_id_fkey;
ALTER TABLE password_resets ADD CONSTRAINT password_resets_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE invites DROP CONSTRAINT IF EXISTS invites_used_by_id_fkey;
ALTER TABLE invites ADD CONSTRAINT invites_used_by_id_fkey
    FOREIGN KEY (used_by_id) REFERENCES users(id);

ALTER TABLE activity_logs DROP CONSTRAINT IF EXISTS activity_logs_actor_id_fkey;
ALTER TABLE activity_logs ADD CONSTRAINT activity_logs_actor_id_fkey
    FOREIGN KEY (actor_id) REFERENCES users(id);
