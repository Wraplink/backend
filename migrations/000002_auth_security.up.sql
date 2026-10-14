ALTER TABLE users
    ADD CONSTRAINT users_failed_login_attempts_check
        CHECK (failed_login_attempts >= 0);

CREATE INDEX idx_users_locked_until
    ON users(locked_until);

CREATE INDEX idx_users_email_verified
    ON users(email_verified);
