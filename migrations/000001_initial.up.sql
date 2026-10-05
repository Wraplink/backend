CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
                       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

                       failed_login_attempts int NOT NULL UNIQUE default 0,
                       locked_until TIMESTAMPTZ NULL,
                       last_login_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                       last_login_ip INET NOT NULL UNIQUE,
                       email VARCHAR(254) NOT NULL UNIQUE,
                       full_name VARCHAR(200) NOT NULL,

                       email_verified BOOLEAN NOT NULL DEFAULT FALSE,
                       account_status VARCHAR(30) NOT NULL DEFAULT 'pending',

                       created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                       updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_credentials (
                                  user_id UUID PRIMARY KEY
                                      REFERENCES users(id) ON DELETE CASCADE,

                                  password_hash TEXT NOT NULL,

                                  password_changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_sessions (
                               id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

                               user_id UUID NOT NULL
                                   REFERENCES users(id) ON DELETE CASCADE,

                               refresh_token_hash TEXT NOT NULL,

                               ip_address INET,
                               user_agent TEXT,

                               expires_at TIMESTAMPTZ NOT NULL,
                               revoked_at TIMESTAMPTZ,

                               created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_user_sessions_user
    ON user_sessions(user_id);

CREATE TABLE email_verifications (
                                     id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

                                     user_id UUID NOT NULL
                                         REFERENCES users(id) ON DELETE CASCADE,

                                     token_hash TEXT NOT NULL UNIQUE,

                                     expires_at TIMESTAMPTZ NOT NULL,
                                     used_at TIMESTAMPTZ,

                                     created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE password_resets (
                                 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

                                 user_id UUID NOT NULL
                                     REFERENCES users(id) ON DELETE CASCADE,

                                 token_hash TEXT NOT NULL UNIQUE,

                                 expires_at TIMESTAMPTZ NOT NULL,
                                 used_at TIMESTAMPTZ,

                                 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE legal_acceptances (
                                   id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

                                   user_id UUID NOT NULL
                                       REFERENCES users(id) ON DELETE CASCADE,

                                   terms_version VARCHAR(30) NOT NULL,
                                   privacy_version VARCHAR(30) NOT NULL,

                                   marketing_consent BOOLEAN NOT NULL DEFAULT FALSE,

                                   ip_address INET,
                                   accepted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE security_events (
                                 id BIGSERIAL PRIMARY KEY,

                                 user_id UUID
                                                        REFERENCES users(id) ON DELETE SET NULL,

                                 event_type VARCHAR(60) NOT NULL,

                                 ip_address INET,
                                 user_agent TEXT,

                                 metadata JSONB NOT NULL DEFAULT '{}',

                                 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_security_events_user
    ON security_events(user_id);

CREATE INDEX idx_security_events_type
    ON security_events(event_type);