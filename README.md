## Security
Generate a strong secret:
```shell
openssl rand -base64 64
```

export JWT_SECRET='YOUR_GENERATED_SECRET'

## Database
```shell
migrate \
  -path migrations \
  -database "$DATABASE_URL" \
  up
```