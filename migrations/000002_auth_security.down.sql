DROP INDEX IF EXISTS idx_users_email_verified;

DROP INDEX IF EXISTS idx_users_locked_until;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_failed_login_attempts_check;

ALTER TABLE users
    DROP COLUMN IF EXISTS last_login_ip,
    DROP COLUMN IF EXISTS last_login_at,
    DROP COLUMN IF EXISTS locked_until,
    DROP COLUMN IF EXISTS failed_login_attempts;