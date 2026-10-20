-- Normalize account-action revocations before restoring the previous constraint.
UPDATE user_sessions SET revoked_reason='admin_revoked' WHERE revoked_reason='admin_account_action';
ALTER TABLE user_sessions DROP CONSTRAINT IF EXISTS user_sessions_revoked_reason_check;
ALTER TABLE user_sessions ADD CONSTRAINT user_sessions_revoked_reason_check CHECK (
 revoked_reason IS NULL OR revoked_reason IN (
  'logout', 'rotated', 'expired', 'replay_detected', 'admin_revoked'
 )
);
