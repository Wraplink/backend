-- Allow session revocation when an administrator suspends or deletes an account.
ALTER TABLE user_sessions DROP CONSTRAINT IF EXISTS user_sessions_revoked_reason_check;
ALTER TABLE user_sessions ADD CONSTRAINT user_sessions_revoked_reason_check CHECK (
 revoked_reason IS NULL OR revoked_reason IN (
  'logout', 'rotated', 'expired', 'replay_detected', 'admin_revoked', 'admin_account_action'
 )
);
