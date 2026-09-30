ALTER TABLE users
    ADD COLUMN failed_login_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN locked_until TIMESTAMPTZ,
    ADD COLUMN last_login_at TIMESTAMPTZ,
    ADD COLUMN last_login_ip INET;

ALTER TABLE users
    ADD CONSTRAINT users_failed_login_attempts_check
        CHECK (failed_login_attempts >= 0);

CREATE INDEX idx_users_locked_until
    ON users(locked_until);

CREATE INDEX idx_users_email_verified
    ON users(email_verified);