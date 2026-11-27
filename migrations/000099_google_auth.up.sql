CREATE TABLE google_auth_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    enabled boolean NOT NULL DEFAULT false
);

INSERT INTO google_auth_settings DEFAULT VALUES;

CREATE TABLE google_identities (
    subject text PRIMARY KEY,
    user_id uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE
);
