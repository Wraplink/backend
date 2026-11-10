DROP INDEX IF EXISTS free_plan_claims_user_code_idx;
ALTER TABLE free_plan_claims DROP COLUMN IF EXISTS plan_code;
