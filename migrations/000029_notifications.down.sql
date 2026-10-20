ALTER TABLE support_tickets DROP COLUMN IF EXISTS customer_unread;
ALTER TABLE support_tickets DROP COLUMN IF EXISTS staff_unread;
DROP TABLE IF EXISTS notification_preferences;
DROP TABLE IF EXISTS user_notifications;
