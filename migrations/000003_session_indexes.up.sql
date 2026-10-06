CREATE INDEX idx_user_sessions_expires_at
    ON user_sessions(expires_at);

CREATE INDEX idx_user_sessions_active
    ON user_sessions(user_id, revoked_at)
    WHERE revoked_at IS NULL;