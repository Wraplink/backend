# Initial WrapLink administrator

The backend can provision a **new** administrator at startup. No public default password is embedded in source code.

After applying migrations through `000022`, configure both environment variables on the backend process:

```bash
export WRAPLINK_BOOTSTRAP_ADMIN_EMAIL='admin@example.com'
export WRAPLINK_BOOTSTRAP_ADMIN_PASSWORD='REPLACE_WITH_A_LONG_RANDOM_SECRET'
```

Start the backend normally. On first startup, the administrator is created with a verified email, an Argon2id password hash, and roles in `access_admins`, `payment_review_admins`, and `discount_admins`. Sign in using the configured email/password and open `/admin` or `/fa/admin`.

For Docker Compose, supply these values through deployment secrets/environment configuration (never commit real credentials):

```yaml
services:
  backend:
    environment:
      WRAPLINK_BOOTSTRAP_ADMIN_EMAIL: ${WRAPLINK_BOOTSTRAP_ADMIN_EMAIL}
      WRAPLINK_BOOTSTRAP_ADMIN_PASSWORD: ${WRAPLINK_BOOTSTRAP_ADMIN_PASSWORD}
```

**Safety behavior:** If both variables are unset, bootstrap is skipped. If one is missing or the password is shorter than 16 characters, startup fails. If the email already belongs to a non-admin user, bootstrap fails rather than elevating that account. When the configured email already belongs to an access administrator, the startup routine compares the configured password with its stored Argon2id hash and updates the hash only when the password differs. A new password in the environment takes effect after recreating/restarting the backend container; it is not monitored live. Existing sessions are not automatically revoked by this bootstrap routine. The account must already be an access administrator; an ordinary user is never promoted. The initialization is serialized and idempotent.

To manage the bootstrap administrator password through deployment configuration, keep both variables configured and recreate the backend after changing the password. If you remove both variables, bootstrap and environment-based password synchronization are disabled; the existing account remains. Never store real secrets in Git. Note that an account-settings password change will be overridden on the next backend restart while these variables remain configured. Protect deployment secrets and database backups.
