# Cloudflare Turnstile

Public endpoints protected when TURNSTILE_SECRET_KEY is configured:
- POST /api/v1/auth/register
- POST /api/v1/auth/resend-verification
- POST /api/v1/auth/password-reset/request
- POST /api/v1/contact

The frontend must supply `X-Turnstile-Token` with each request. Tokens are short-lived and single-use; obtain a new challenge token for retries. Set `NEXT_PUBLIC_TURNSTILE_SITE_KEY` in the **web build environment**, and `TURNSTILE_SECRET_KEY` in the backend runtime. Never expose the secret to the browser.

Both keys must be configured together. With no backend secret, CAPTCHA is disabled for local development. In production, verify both variables before exposing registration endpoints; do not deploy only the frontend key.

Backend requires outbound HTTPS to `challenges.cloudflare.com:443`. For restricted environments, configure a supported HTTPS proxy for the Go HTTP client, or arrange direct egress. The verification middleware fails closed when the siteverify API is unavailable.

After successful verification the token is consumed. CAPTCHA does not replace IP/email/global rate limits or pending-account cleanup.
