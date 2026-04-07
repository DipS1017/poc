# Go + ZITADEL setup

This project includes a minimal ZITADEL integration for a Go API using OAuth2 token introspection.

## 1) Start ZITADEL

```bash
docker compose up -d
```

ZITADEL will be available at `http://localhost:8080`.

## 2) Configure ZITADEL

In the ZITADEL console:

1. Create a Project (or reuse one).
2. Create an API/application that issues access tokens for your client/user flow.
3. Create a confidential machine/client application for introspection.
4. Grant it permission to introspect/access tokens in your project/org.
5. Copy the introspection client ID and secret.
6. Note the audience you want your API to enforce (optional).

## 3) Configure Go app

```bash
cp .env.example .env
```

Fill:

- `ZITADEL_CLIENT_ID`
- `ZITADEL_CLIENT_SECRET`
- `ZITADEL_AUDIENCE` (optional but recommended)

## 4) Run Go app

```bash
export $(grep -v '^#' .env | xargs)
go run .
```

Server runs on `http://localhost:8082`.

## 5) Test endpoints

Public:

```bash
curl http://localhost:8082/public
```

Protected (replace `<ACCESS_TOKEN>`):

```bash
curl -H "Authorization: Bearer <ACCESS_TOKEN>" http://localhost:8082/protected
```

## Behavior

- `/protected` calls ZITADEL introspection endpoint to validate token activity.
- If `ZITADEL_AUDIENCE` is set, the API also enforces audience.
- Subject and scope are forwarded to handler via request headers.
