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

## Update database
```shell

set -a
source .env.local
set +a

migrate \
  -path migrations \
  -database "$DATABASE_URL" \
  up
```