CREATE TABLE contact_messages (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name varchar(120) NOT NULL, email varchar(254) NOT NULL, subject varchar(200) NOT NULL, message text NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX contact_messages_created_idx ON contact_messages(created_at DESC);
