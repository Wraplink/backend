## Security
Generate a strong secret:
```shell
openssl rand -base64 64
```

export JWT_SECRET='YOUR_GENERATED_SECRET'

## Database
Apply schema migrations before starting the backend:

```shell
migrate \
  -path migrations \
  -database "$DATABASE_URL" \
  up
```

## Required startup data
After PostgreSQL is connected, the backend runs an idempotent reference-data bootstrap before starting the HTTP server.

Missing default plans are inserted automatically. Existing plans with the same `code` are never overwritten, so later pricing/quota changes survive application restarts.

| Code | Name | Price | Currency | Duration | Traffic |
| --- | --- | ---: | --- | ---: | ---: |
| `starter` | Starter | 1,000,000 | IRR | 30 days | 50 GiB |
| `gamer` | Gamer | 2,500,000 | IRR | 30 days | 150 GiB |
| `pro` | Pro | 5,000,000 | IRR | 30 days | 500 GiB |

The bootstrap is intentionally limited to system/reference data. It does not create users, wallet balances, payments, or subscriptions.
