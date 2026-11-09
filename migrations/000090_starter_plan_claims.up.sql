-- Starter offers are reserved on verified onboarding and activated later.
-- Claims intentionally retain plan IDs without a cascading plan foreign key:
-- archived/deleted catalog entries must not reset lifetime eligibility.
CREATE TABLE starter_plan_policies (
    id UUID PRIMARY KEY,
    plan_id UUID NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT false,
    priority INTEGER NOT NULL DEFAULT 0,
    trial_duration_seconds BIGINT NOT NULL DEFAULT 3600 CHECK (trial_duration_seconds > 0),
    region TEXT NOT NULL DEFAULT '',
    assignment_trigger TEXT NOT NULL DEFAULT 'email_verified'
        CHECK (assignment_trigger IN ('email_verified','first_login')),
    activation_trigger TEXT NOT NULL DEFAULT 'manual'
        CHECK (activation_trigger IN ('manual','first_dns_use','immediate')),
    reservation_ttl_seconds BIGINT NOT NULL DEFAULT 604800
        CHECK (reservation_ttl_seconds > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX starter_plan_policies_eligible_idx
    ON starter_plan_policies (enabled, assignment_trigger, priority DESC, id);

CREATE TABLE free_plan_claims (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    plan_id UUID NOT NULL,
    starter_policy_id UUID REFERENCES starter_plan_policies(id) ON DELETE SET NULL,
    source TEXT NOT NULL CHECK (source IN ('market','starter','admin')),
    state TEXT NOT NULL DEFAULT 'reserved'
        CHECK (state IN ('reserved','activating','active','expired','cancelled','failed')),
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reservation_expires_at TIMESTAMPTZ,
    activated_at TIMESTAMPTZ,
    subscription_id UUID,
    UNIQUE (user_id, plan_id),
    CHECK (reservation_expires_at IS NULL OR reservation_expires_at > assigned_at)
);

-- Only one automatic starter offer can be reserved for a user,
-- regardless of how many starter policies match.
CREATE UNIQUE INDEX free_plan_claims_one_starter_per_user_idx
    ON free_plan_claims (user_id) WHERE source = 'starter';

CREATE INDEX free_plan_claims_user_state_idx
    ON free_plan_claims (user_id, state);
