-- Preserve the claimed catalog code independently of plan deletion.
-- A deleted and later recreated plan with the same code must not reset eligibility.
ALTER TABLE free_plan_claims ADD COLUMN plan_code TEXT;
UPDATE free_plan_claims c SET plan_code = p.code
FROM plans p WHERE p.id = c.plan_id;
CREATE UNIQUE INDEX free_plan_claims_user_code_idx ON free_plan_claims (user_id, lower(plan_code))
WHERE plan_code IS NOT NULL;
