DROP INDEX IF EXISTS idx_users_email_verified;
DROP INDEX IF EXISTS idx_users_locked_until;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_failed_login_attempts_check;
