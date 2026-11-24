# WrapLink administrator access

Run database migrations through 000097 before enabling administrator pages.

## Bootstrap

The first access administrator must be explicitly assigned by a trusted database operator. Do not expose this operation through registration or public APIs.

```sql
-- Replace with the verified UUID of the intended administrator.
INSERT INTO access_admins (user_id)
VALUES ('00000000-0000-0000-0000-000000000000')
ON CONFLICT (user_id) DO NOTHING;
```

The example UUID is a placeholder and will fail unless it matches a real user.

## Staff permissions

Open **Admin → Permissions** as an access administrator. Copy the staff account’s UUID from **Admin → Users**, enter it in the grant form, then choose a permission. Staff can hold several grants. The permissions page lists explicit grants and lets you revoke each one.

- **Payment reviews** covers order and wallet receipts. **Payment methods** controls destination cards and payment settings. **Discounts** is independent.
- **Plans** and **Starter plans** control their respective catalogs.
- **Support tickets** covers replies and status changes. **Contact messages** covers submissions from the public contact form.
- **Network monitoring** covers node health, sessions, DNS queries, and route IP inventory. **Network management** includes monitoring and permits node removal, session disconnection, and clearing DNS query reports.
- **Domain policies** covers managed domains and DNS resolution policies.
- **Users** and **Orders** grant read-only views. Account suspension, restoration, deletion, and permission changes require an access administrator.

Migration 000097 makes the payment-method, support-ticket, and contact-message access previously inherited by payment reviewers into explicit grants. These grants can be revoked individually for existing users. Newly granted payment reviewers receive receipt access alone.

## Endpoints

All endpoints require a valid active user session.

- `GET /api/v1/admin/access/me` returns only the caller’s effective permissions.
- `GET /api/v1/admin/access/permissions` lists up to 500 accounts with explicit grants. It requires access administrator membership.
- `PUT /api/v1/admin/access/users/{userID}/permissions/{permission}` requires access administrator membership and accepts `{"enabled":true}` or `{"enabled":false}`. The grant or revocation and its audit event commit together.

Access administrator membership is provisioned through the bootstrap mechanism or controlled database administration.
