CREATE TABLE user_notifications (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 type varchar(24) NOT NULL CHECK(type IN ('order','payment','support','security','system')),
 title varchar(200) NOT NULL,
 message text NOT NULL,
 link text,
 read_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX user_notifications_feed_idx ON user_notifications(user_id,created_at DESC);
CREATE INDEX user_notifications_unread_idx ON user_notifications(user_id) WHERE read_at IS NULL;
CREATE TABLE notification_preferences (
 user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 orders boolean NOT NULL DEFAULT true,
 payments boolean NOT NULL DEFAULT true,
 support boolean NOT NULL DEFAULT true,
 security boolean NOT NULL DEFAULT true,
 system boolean NOT NULL DEFAULT true
);
ALTER TABLE support_tickets ADD COLUMN customer_unread boolean NOT NULL DEFAULT false;
ALTER TABLE support_tickets ADD COLUMN staff_unread boolean NOT NULL DEFAULT true;
CREATE INDEX support_tickets_staff_unread_idx ON support_tickets(updated_at DESC) WHERE staff_unread;
