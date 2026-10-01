DROP INDEX IF EXISTS idx_user_sessions_refresh_token_hash;
DROP INDEX IF EXISTS idx_user_sessions_replaced_by;
DROP INDEX IF EXISTS idx_user_sessions_family;

ALTER TABLE user_sessions
    DROP CONSTRAINT IF EXISTS user_sessions_revoked_reason_check;

ALTER TABLE user_sessions
    DROP CONSTRAINT IF EXISTS user_sessions_replaced_by_fk;

ALTER TABLE user_sessions
    DROP COLUMN IF EXISTS revoked_reason;

ALTER TABLE user_sessions
    DROP COLUMN IF EXISTS replaced_by_session_id;

ALTER TABLE user_sessions
    DROP COLUMN IF EXISTS family_id;