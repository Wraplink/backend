CREATE TABLE admin_section_permissions (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission TEXT NOT NULL CHECK (permission IN (
        'users', 'orders', 'payment_methods', 'plans', 'starter_plans',
        'support', 'contact_messages', 'network_monitor', 'network_manage', 'domain_policies'
    )),
    PRIMARY KEY (user_id, permission)
);

-- Preserve capabilities that previously came with the payment reviewer role.
INSERT INTO admin_section_permissions (user_id, permission)
SELECT user_id, permission
FROM payment_review_admins
CROSS JOIN (VALUES ('payment_methods'), ('support'), ('contact_messages')) AS existing(permission);

ALTER TABLE admin_permission_events DROP CONSTRAINT admin_permission_events_permission_check;
ALTER TABLE admin_permission_events ADD CONSTRAINT admin_permission_events_permission_check
CHECK (permission IN (
    'payment_review', 'discount_admin', 'users', 'orders', 'payment_methods', 'plans',
    'starter_plans', 'support', 'contact_messages', 'network_monitor', 'network_manage', 'domain_policies'
));
