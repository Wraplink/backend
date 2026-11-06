# Slack project reporting

This repository reports GitHub issue changes, pull requests, and failed workflow runs to Slack.

## Setup
1. Create a Slack Incoming Webhook for a channel in `wraplink.slack.com`.
2. Set `WRAPLINK_SLACK_WEBHOOK_URL` in GitHub repository Actions secrets.
3. Merge the PR, then run **Actions → Slack project reporting → Run workflow**.
4. Check the Slack channel and GitHub Actions logs.

The workflow does not execute code from untrusted pull requests and does not include issue bodies, stack traces, secrets, or user data.

## Scope and limitations
This workflow reports **GitHub activity only**. It does not capture runtime errors, application logs, DNS failures, nftables failures, or support tickets submitted through the website.

For application-level reporting, emit structured events to a backend-owned alert dispatcher, persist notifications in an outbox, redact sensitive information, deduplicate by fingerprint, retry delivery with bounded backoff, and preserve user-submitted tickets even if Slack is unavailable. Keep the Slack webhook on the server only. Report ticket IDs, categories, severity and admin links; do not send ticket body or personal details by default.


## Application notifications (backend)
The backend additionally writes `support.ticket.created` and `contact.created` events to `slack_outbox` within the same database transaction as the user submission. A background worker dispatches events to Slack. The original support ticket or contact message remains available if Slack delivery fails after the database transaction commits.

Configure the **backend service environment** with `WRAPLINK_SLACK_WEBHOOK_URL` (separately from the GitHub Actions secret). Run migration `000043_slack_outbox` before deploying the updated backend. The webhook must never be prefixed with `NEXT_PUBLIC_` or exposed to the web frontend.

Messages contain only ticket number, priority, category, or contact reference. They exclude email, message bodies, customer names, and user identifiers.

**Limitations:** Delivery is at-least-once, so a crash after Slack accepts an event but before DB acknowledgment can produce duplicates. Pending events retry with backoff. A permanently invalid webhook requires operator intervention. Runtime errors from DNS Control, Edge Agent and Backend are not yet collected by this dispatcher.

## Runtime alert ingestion
Set `WRAPLINK_ALERT_INGEST_KEY` to a strong shared secret in the backend and each authorized service. When configured, backend exposes `POST /api/v1/internal/alerts`, requiring `X-Wraplink-Alert-Key`. Only use this endpoint over trusted internal networks; restrict it at the reverse proxy and firewall. Alerts are deduplicated by service, severity, fingerprint, environment and five-minute bucket.

Payload example:
```json
{"service":"dns-control","severity":"error","code":"domain_policy_sync_failed","fingerprint":"domain_policy_sync_failed","summary":"Backend domain policy sync failed","environment":"production"}
```
No raw errors or stack traces should be submitted. Alert delivery to Slack requires the backend's `WRAPLINK_SLACK_WEBHOOK_URL` and migration 000043.
