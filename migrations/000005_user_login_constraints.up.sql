ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_failed_login_attempts_key;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_last_login_ip_key;

ALTER TABLE users
    ALTER COLUMN last_login_at DROP NOT NULL,
    ALTER COLUMN last_login_at DROP DEFAULT,
    ALTER COLUMN last_login_ip DROP NOT NULL;
