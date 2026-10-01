ALTER TABLE user_sessions
    ADD COLUMN family_id UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN replaced_by_session_id UUID,
    ADD COLUMN revoked_reason VARCHAR(40);

ALTER TABLE user_sessions
    ADD CONSTRAINT user_sessions_replaced_by_fk
        FOREIGN KEY (replaced_by_session_id)
            REFERENCES user_sessions(id)
            ON DELETE SET NULL;

ALTER TABLE user_sessions
    ADD CONSTRAINT user_sessions_revoked_reason_check
        CHECK (
            revoked_reason IS NULL
                OR revoked_reason IN (
                                      'logout',
                                      'rotated',
                                      'expired',
                                      'replay_detected',
                                      'admin_revoked'
                )
            );

CREATE INDEX idx_user_sessions_family
    ON user_sessions(family_id);

CREATE INDEX idx_user_sessions_replaced_by
    ON user_sessions(replaced_by_session_id);

CREATE INDEX idx_user_sessions_refresh_token_hash
    ON user_sessions(refresh_token_hash);