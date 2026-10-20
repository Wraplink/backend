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

**Safety behavior:** If both variables are unset, bootstrap is skipped. If one is missing or the password is shorter than 16 characters, startup fails. If the email already belongs to a non-admin user, bootstrap fails rather than elevating that account. Existing access administrators are not modified or assigned a new password. The initialization is serialized and idempotent.

After the first successful creation, remove both bootstrap variables from the deployment and restart the backend. Rotate the initial password through account settings. Protect deployment secrets and database backups.
