ALTER TABLE user_subscriptions ADD COLUMN plan_description TEXT NOT NULL DEFAULT '';

-- Existing purchases predate this snapshot; do not infer historical terms from the mutable catalog.
