CREATE SEQUENCE support_ticket_number_seq START 10001;
CREATE TABLE support_tickets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 ticket_number bigint NOT NULL UNIQUE DEFAULT nextval('support_ticket_number_seq'),
 user_id uuid NOT NULL REFERENCES users(id),
 order_id uuid REFERENCES user_subscriptions(id),
 subject varchar(200) NOT NULL,
 category varchar(40) NOT NULL,
 priority varchar(12) NOT NULL DEFAULT 'medium' CHECK(priority IN ('low','medium','high')),
 status varchar(20) NOT NULL DEFAULT 'open' CHECK(status IN ('open','pending','resolved','closed')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE support_messages (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 ticket_id uuid NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
 sender_id uuid NOT NULL REFERENCES users(id),
 sender_role varchar(12) NOT NULL CHECK(sender_role IN ('user','support')),
 message text NOT NULL CHECK(length(message) BETWEEN 1 AND 10000),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX support_tickets_user_idx ON support_tickets(user_id,created_at DESC);
CREATE INDEX support_tickets_status_idx ON support_tickets(status,updated_at DESC);
CREATE INDEX support_messages_ticket_idx ON support_messages(ticket_id,created_at);
