CREATE TABLE access_admins (user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE, granted_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE admin_permission_events (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), actor_id UUID NOT NULL REFERENCES users(id), target_id UUID NOT NULL REFERENCES users(id), permission TEXT NOT NULL CHECK(permission IN ('payment_review','discount_admin')), enabled BOOLEAN NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE INDEX idx_admin_permission_events_target ON admin_permission_events(target_id,created_at DESC);
