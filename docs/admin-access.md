# WrapLink administrator access

Run database migrations through 000022 before enabling administrator pages.

## Bootstrap

The first access administrator must be explicitly assigned by a trusted database operator. Do not expose this operation through registration or public APIs.

```sql
-- Replace with the verified UUID of the intended administrator.
INSERT INTO access_admins (user_id)
VALUES ('00000000-0000-0000-0000-000000000000')
ON CONFLICT (user_id) DO NOTHING;
```

The example UUID is a placeholder and will fail unless it matches a real user.

## Endpoints

All endpoints require a valid active user session. Only members of `access_admins` may call:

- `GET /api/v1/admin/access/permissions`: list accounts with review/discount permissions (up to 500).
- `PUT /api/v1/admin/access/users/{userID}/permissions/payment_review`
- `PUT /api/v1/admin/access/users/{userID}/permissions/discount_admin`

PUT JSON: `{"enabled":true}` or `{"enabled":false}`. All changes are written to `admin_permission_events` in the same database transaction.

Access administrator membership is intentionally not editable via these endpoints. Provision and revoke it through controlled database administration. A reviewer cannot grant their own permissions.

## Operational notes

Restrict database access, audit administrator changes, and grant permissions only to verified users. Test unauthorized, missing-user, grant, revoke, and concurrent requests before production.
