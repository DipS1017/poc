# Zitadel Identity Management - Complete Guide

## Table of Contents

1. [What is Zitadel?](#1-what-is-zitadel)
2. [Core Concepts](#2-core-concepts)
3. [Setup Guide (This Project)](#3-setup-guide-this-project)
   - [3.1 Prerequisites](#31-prerequisites)
   - [3.2 Start Zitadel](#32-start-zitadel)
   - [3.3 Configure Zitadel for this POC](#33-configure-zitadel-for-this-poc)
   - [3.4 Configure and Run the Go App](#34-configure-and-run-the-go-app)
   - [3.5 Test the Endpoints](#35-test-the-endpoints)
4. [Authentication Patterns](#4-authentication-patterns)
   - [4.1 Token Introspection (RFC 7662)](#41-token-introspection-rfc-7662)
   - [4.2 JWT Validation (JWKS)](#42-jwt-validation-jwks)
   - [4.3 OIDC Authorization Code Flow with PKCE](#43-oidc-authorization-code-flow-with-pkce)
   - [4.4 Client Credentials (M2M)](#44-client-credentials-m2m)
   - [4.5 Personal Access Tokens (PAT)](#45-personal-access-tokens-pat)
5. [Authorization Patterns](#5-authorization-patterns)
   - [5.1 Scope-Based Access Control](#51-scope-based-access-control)
   - [5.2 Role-Based Access Control (RBAC)](#52-role-based-access-control-rbac)
   - [5.3 Organization-Based Multi-Tenancy](#53-organization-based-multi-tenancy)
   - [5.4 Attribute-Based Access Control (ABAC)](#54-attribute-based-access-control-abac)
6. [Good Practices](#6-good-practices)
   - [6.1 Security](#61-security)
   - [6.2 Performance](#62-performance)
   - [6.3 Architecture](#63-architecture)
   - [6.4 Development Workflow](#64-development-workflow)
7. [Real-World Examples](#7-real-world-examples)
8. [Future Architecture: Google Workspace Clone](#8-future-architecture-google-workspace-clone)
9. [Zitadel API Reference](#9-zitadel-api-reference-quick-reference)
10. [Troubleshooting](#10-troubleshooting)

---

## 1. What is Zitadel?

Zitadel is an open-source identity and access management (IAM) platform built for cloud-native workloads. It ships as a single binary or Docker image and requires only a PostgreSQL database. Zitadel handles user authentication, machine-to-machine authorization, multi-tenancy, and extensible event-driven logic — the full IAM stack without needing to bolt together separate services.

### Why Zitadel over alternatives?

| Feature | Zitadel | Keycloak | Auth0 | Firebase Auth | Supabase Auth |
|---------|---------|----------|-------|---------------|---------------|
| Built-in multi-tenancy (Organizations) | Native | Realms (complex) | Organizations (paid) | Projects only | Projects only |
| Machine users & service accounts | Native | Service accounts | M2M apps | Firebase Admin | Limited |
| Actions (event triggers / webhooks) | Built-in | Custom SPIs (Java) | Rules (JS, paid) | Cloud Functions | Webhooks |
| OIDC / OAuth2 / SAML | All three | All three | All three | OIDC only | OIDC only |
| Self-hosted single binary | Yes | Yes (JVM) | No (cloud only) | No | Partial |
| Free tier on managed cloud | Yes (zitadel.cloud) | No | Yes (limited) | Yes (limited) | Yes (limited) |
| Login v2 (modern UI, customizable) | Yes | Theme-based | Yes | Limited | No |

Zitadel is a strong choice when you need multi-tenant B2B SaaS, service-to-service auth, or want to avoid vendor lock-in with a self-hostable, protocol-compliant system.

---

## 2. Core Concepts

### Instance

An **Instance** is the top-level isolation boundary in Zitadel. Think of it as one entire Zitadel deployment. In a self-hosted setup, you typically run one instance. Zitadel Cloud gives each customer their own instance. Everything below — organizations, users, projects — lives inside an instance.

### Organization

An **Organization** (Org) is a tenant within an instance. In a B2B SaaS product, each customer company gets its own organization. Organizations are isolated: users, roles, and settings are scoped to their org. Cross-org collaboration is possible via Grants. In this project, the default org created by the Zitadel setup is where all resources are configured.

### Project

A **Project** is a container for applications and role definitions within an organization. All applications that belong to the same product or domain live under one project so they share role definitions and authorization settings. In this project, a single "Workspace" project holds the API app, the web app, and all role definitions (`viewer`, `editor`, `owner`, `admin`).

### Application

An **Application** represents a specific client registered in a project. Zitadel supports four types:

- **Web** — server-rendered or SPA apps using PKCE or client secrets
- **Native** — mobile/desktop apps using PKCE
- **API** — backend services that introspect tokens (confidential clients)
- **User Agent** — deprecated; prefer Web with PKCE

Each application gets a Client ID and (optionally) a Client Secret. In this project, the "Workspace API" is an API-type application used for token introspection.

### User

Zitadel has two user types:

- **Human users** — real people with username/password, MFA, and login sessions
- **Machine users** — programmatic identities for services, CI/CD, scripts; no password, uses PATs or client credentials

Machine users are first-class citizens in Zitadel — not an afterthought like in many other IAM systems.

### Roles

**Roles** are project-level labels that represent permissions. You define them on the project, then grant them to users via User Grants. Zitadel includes roles in the token's claims under `urn:zitadel:iam:org:project:roles`. Role structure in the claim:

```json
{
  "urn:zitadel:iam:org:project:roles": {
    "editor": { "org-id-abc": "My Organization" },
    "viewer": { "org-id-abc": "My Organization" }
  }
}
```

The key is the role name. The value is a map of org ID to org name, indicating which org granted that role.

### Grants

**Grants** (User Grants) are the binding between a user, a project, and a set of roles. You grant a user one or more roles on a project. Cross-organization grants are also supported — an organization can grant a user from another org access to their project's resources.

### Actions

**Actions** are JavaScript functions that run as hooks at specific points in the authentication or user lifecycle — before or after user creation, token issuance, authentication, etc. Actions can:

- Add custom claims to tokens
- Enrich user metadata
- Call external webhooks
- Enforce custom business logic (e.g., block login from certain countries)

Actions are Zitadel's extension mechanism and replace the need for custom login flows.

### Personal Access Tokens (PAT)

**Personal Access Tokens** are static bearer tokens tied to a machine user. They do not expire until the configured expiry date. PATs are sent in the `Authorization: Bearer <token>` header just like any other token. They are ideal for local development, CI/CD pipelines, and scripts. Treat them like passwords and rotate them regularly.

### Service Users

**Service Users** (also called Machine Users) are non-human identities used for machine-to-machine communication. They can authenticate via:

- PAT (static token)
- JWT profile (private key JWT, recommended for production)
- Client credentials (client ID + secret via OAuth2)

Service users can be granted project roles just like human users.

---

## 3. Setup Guide (This Project)

### 3.1 Prerequisites

- Docker and Docker Compose (v2+)
- Go 1.22+ (the workspace POC uses the Go 1.22 enhanced `ServeMux` with method-prefix patterns)
- `curl` and `jq` (for testing scripts)
- A modern browser (for the Zitadel console)

### 3.2 Start Zitadel

```bash
# From the project root
docker compose up -d
```

Docker Compose starts three containers:

| Container | Image | Port | Purpose |
|-----------|-------|------|---------|
| `zitadel` | `ghcr.io/zitadel/zitadel:latest` | `8090` (host) → `8080` (container) | Core Zitadel server |
| `login` | `ghcr.io/zitadel/zitadel-login:latest` | `3000` (shared with zitadel) | Login v2 UI |
| `db` | `postgres:17` | `5432` | PostgreSQL database |

Access points after startup:

- **Zitadel Console**: http://localhost:8090/ui/console
- **Login v2 UI**: http://localhost:3000/ui/v2/login/
- **Default admin credentials**: `zitadel-admin@zitadel.localhost` / `Password1!`
- **OIDC discovery**: http://localhost:8090/.well-known/openid-configuration

The first boot initializes the database schema, creates the default organization, and writes the Login v2 service account PAT to `./login-client.pat`. This takes 20–40 seconds on first run.

```bash
# Verify Zitadel is healthy
curl -s http://localhost:8090/debug/healthz | jq .
# Expected: {"status": "SERVING"}

# Check OIDC discovery
curl -s http://localhost:8090/.well-known/openid-configuration | jq '{issuer, token_endpoint, introspection_endpoint}'
```

### 3.3 Configure Zitadel for this POC

All configuration is done via the Zitadel Console at http://localhost:8090/ui/console.

**Step 1 — Log in to the Console**

Navigate to http://localhost:8090/ui/console and log in with:
- Username: `zitadel-admin@zitadel.localhost`
- Password: `Password1!`

You will land on the instance dashboard. All project work happens inside the default organization.

**Step 2 — Create a Project**

1. In the left sidebar, click **Projects**.
2. Click **New Project** (top right).
3. Name it `Workspace`.
4. Leave the defaults and click **Continue**.
5. On the project settings page, find **Assert Roles on Authentication** and enable it. This is critical — without it, role claims will not appear in tokens.
6. Save.

**Step 3 — Create Roles in the Project**

Inside the Workspace project, go to **Roles** → **New Role** and create the following four roles:

| Role Key | Display Name | Group |
|----------|-------------|-------|
| `viewer` | Viewer | workspace |
| `editor` | Editor | workspace |
| `owner` | Owner | workspace |
| `admin` | Admin | workspace |

The role key is what appears in the token claim. The group is optional metadata.

**Step 4 — Create an API Application (for Token Introspection)**

The API application is the confidential client whose credentials the Go service uses to call the introspect endpoint.

1. Inside the Workspace project, go to **Applications** → **New**.
2. Name it `Workspace API`.
3. Select **API** as the type.
4. Authentication method: **Basic** (HTTP Basic Auth with client ID and secret).
5. Click **Create**.
6. Copy the **Client ID** and **Client Secret** — you will need these for the `.env` file. The secret is only shown once.

**Step 5 — Create a Web Application (for user login via PKCE)**

This is the OIDC client for a browser-based frontend.

1. In the Workspace project → **Applications** → **New**.
2. Name it `Workspace Web`.
3. Select **Web** as the type.
4. Authentication method: **PKCE** (recommended for SPAs; no client secret required).
5. Redirect URI: `http://localhost:3000/callback`
6. Post-logout redirect URI: `http://localhost:3000`
7. Click **Create** and note the **Client ID**.

**Step 6 — Create a Machine User (for M2M testing)**

1. In the sidebar, go to **Users** → switch to the **Service Accounts** tab.
2. Click **New** → enter username `docs-service` and description `Docs service account`.
3. After creation, open the user → go to **Personal Access Tokens** → **New**.
4. Set an expiry date (e.g., 2029-01-01) and click **Add**.
5. Copy the generated PAT — it is shown only once. Store it securely.

**Step 7 — Assign Roles to Users**

Human users need role grants before their tokens include role claims.

1. In the sidebar, go to **Users** → click a human user.
2. Go to **Authorizations** tab → **New Authorization**.
3. Select the **Workspace** project.
4. Select one or more roles: `viewer`, `editor`, `owner`, `admin`.
5. Click **Save**.

Repeat for machine users: open the machine user → **Authorizations** → **New Authorization**.

**Step 8 — Enable Role Assertion (verify project setting)**

Return to the **Workspace** project settings and confirm:
- "Assert Roles on Authentication" is enabled (toggles role claims in tokens)
- "Check Authorization on Authentication" can be left off unless you want to block login for users with no grants

### 3.4 Configure and Run the Go App

**Basic introspection API (port 8082)**

```bash
# Copy and edit the environment file
cp .env.example .env

# Fill in the credentials from Step 4 above
# ZITADEL_CLIENT_ID=<api-app-client-id>
# ZITADEL_CLIENT_SECRET=<api-app-client-secret>
# ZITADEL_INTROSPECTION_URL=http://localhost:8090/oauth/v2/introspect

# Load env vars and start the server
export $(grep -v '^#' .env | xargs)
go run .
# Output: Server started on :8082
```

**Workspace POC (port 8083)**

```bash
# Copy and edit the workspace environment file
cp poc/workspace/.env.example poc/workspace/.env

# Fill in the same API app credentials
# ZITADEL_CLIENT_ID=<api-app-client-id>
# ZITADEL_CLIENT_SECRET=<api-app-client-secret>
# ZITADEL_INTROSPECTION_URL=http://localhost:8090/oauth/v2/introspect

# The workspace POC has a built-in .env loader — no export needed
go run ./poc/workspace/
# Output: workspace POC listening on :8083
#         introspection endpoint: http://localhost:8090/oauth/v2/introspect
```

The workspace POC's `loadDotEnv` function reads `poc/workspace/.env` automatically at startup. Values already set in the environment take precedence over the file.

### 3.5 Test the Endpoints

**Get an access token using client credentials**

```bash
# Using the helper script (requires jq)
TOKEN=$(./scripts/get-token.sh \
  --client-id     "<your-client-id>" \
  --client-secret "<your-client-secret>" \
  --scope         "openid docs:read docs:write drive:read drive:write")

echo "Token: ${TOKEN:0:40}..."

# Or manually with curl
TOKEN=$(curl -s -X POST http://localhost:8090/oauth/v2/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --user "<client-id>:<client-secret>" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "scope=openid" \
  | jq -r '.access_token')
```

**Test the basic API (port 8082)**

```bash
# Public endpoint — no auth needed
curl http://localhost:8082/public
# Response: public endpoint

# Protected endpoint — no token (should fail)
curl http://localhost:8082/protected
# Response: missing authorization header (401)

# Protected endpoint — with valid token
curl -H "Authorization: Bearer $TOKEN" http://localhost:8082/protected
# Response:
# {
#   "message": "protected endpoint",
#   "subject": "<user-or-machine-id>",
#   "scope": "openid"
# }
```

**Test the workspace POC (port 8083)**

```bash
# Public health check
curl http://localhost:8083/health
# Response: {"service":"workspace","status":"ok"}

# Authenticated user profile
curl -H "Authorization: Bearer $TOKEN" http://localhost:8083/api/me
# Response:
# {
#   "email": "...",
#   "name": "...",
#   "roles": ["editor", "viewer"],
#   "scopes": ["openid", "docs:read", "docs:write"],
#   "sub": "<user-id>",
#   "username": "..."
# }

# List documents (requires docs:read scope OR viewer/editor/owner role)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8083/api/docs
# Response:
# {
#   "documents": [
#     {"id": "doc-001", "title": "Q1 Strategy", "owner": "<user-id>"},
#     {"id": "doc-002", "title": "Architecture Notes", "owner": "<user-id>"}
#   ]
# }

# Create a document (requires docs:write scope OR editor/owner role)
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title": "My New Document"}' \
  http://localhost:8083/api/docs
# Response (201 Created):
# {
#   "created_at": "2025-01-15T10:30:00Z",
#   "id": "doc-1736937000000",
#   "owner": "<user-id>",
#   "title": "My New Document"
# }

# List Drive files (requires drive:read scope OR viewer/editor/owner role)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8083/api/drive
# Response:
# {
#   "files": [
#     {"id": "file-001", "name": "report.pdf", "owner": "<user-id>"},
#     {"id": "file-002", "name": "logo.png", "owner": "<user-id>"}
#   ]
# }

# Upload a file (requires drive:write scope OR editor/owner role)
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "presentation.pptx"}' \
  http://localhost:8083/api/drive/upload
# Response (201 Created):
# {
#   "id": "file-1736937000000",
#   "name": "presentation.pptx",
#   "owner": "<user-id>",
#   "uploaded_at": "2025-01-15T10:30:00Z"
# }

# Admin — list users (requires admin role, no scope fallback)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8083/api/admin/users
# Without admin role → 403:
# {"error":"insufficient permissions","required_role":"admin"}
# With admin role → 200:
# {
#   "requested_by": "<user-id>",
#   "users": [
#     {"email": "alice@example.com", "id": "user-001", "role": "editor"},
#     ...
#   ]
# }
```

**Run the full test suite**

```bash
# Get a token first, then run the smoke test script
TOKEN=$(./scripts/get-token.sh --client-id "$ZITADEL_CLIENT_ID" --client-secret "$ZITADEL_CLIENT_SECRET")
./scripts/test-workspace.sh "$TOKEN"
```

**Expected error responses**

```bash
# No token
curl http://localhost:8083/api/docs
# 401: {"error":"missing Authorization header"}

# Expired or invalid token
curl -H "Authorization: Bearer invalid-token" http://localhost:8083/api/docs
# 401: {"error":"token validation failed"}

# Active token but wrong role
curl -H "Authorization: Bearer $VIEWER_TOKEN" -X POST \
  -H "Content-Type: application/json" -d '{"title":"test"}' \
  http://localhost:8083/api/docs
# 403: {"error":"insufficient permissions","required_scope":"docs:write","required_roles":"editor, owner"}
```

---

## 4. Authentication Patterns

### 4.1 Token Introspection (RFC 7662)

Token introspection is the pattern used throughout this project. The API never validates the token itself — it sends the raw bearer token to Zitadel's introspection endpoint and receives a JSON response indicating whether the token is active and what claims it carries.

**How it works:**

1. Client sends `Authorization: Bearer <token>` to the Go API.
2. The Go API extracts the token and POSTs it to `POST /oauth/v2/introspect` with HTTP Basic auth (client ID + secret).
3. Zitadel returns a JSON body with `active: true|false` and claims.
4. The API checks `active`, validates the audience, then checks scopes/roles.

**When to use:** When you need real-time revocation (a revoked token is immediately inactive), when tokens may be opaque (not JWTs), or when simplicity matters more than raw throughput.

**Pros:**
- Token revocation is immediate — no delay
- Works with any token format (opaque or JWT)
- Simple to implement
- Centralized validation — no local crypto

**Cons:**
- One network call to Zitadel per request (unless cached)
- Latency added to every protected request
- Zitadel becomes a synchronous dependency

**Code example from this project (`poc/workspace/main.go`):**

```go
func introspectToken(ctx context.Context, client *http.Client, cfg authConfig, rawToken string) (introspectionResponse, error) {
    form := url.Values{}
    form.Set("token", rawToken)
    form.Set("token_type_hint", "access_token")

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.introspectionURL, strings.NewReader(form.Encode()))
    if err != nil {
        return introspectionResponse{}, fmt.Errorf("build introspection request: %w", err)
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.SetBasicAuth(cfg.clientID, cfg.clientSecret)

    resp, err := client.Do(req)
    // ... decode and return introspectionResponse
}
```

The `introspectionResponse` struct includes Zitadel-specific role claims:

```go
type introspectionResponse struct {
    Active   bool        `json:"active"`
    Sub      string      `json:"sub"`
    Scope    string      `json:"scope"`
    Aud      interface{} `json:"aud"` // string or []string
    Name     string      `json:"name"`
    Email    string      `json:"email"`
    Username string      `json:"username"`
    // Zitadel project roles: role name -> (org ID -> org name)
    ZitadelRoles map[string]map[string]string `json:"urn:zitadel:iam:org:project:roles"`
}
```

### 4.2 JWT Validation (JWKS)

Rather than calling Zitadel per request, the API fetches Zitadel's public signing keys from the JWKS endpoint and validates the JWT signature locally.

**How it works:**

1. On startup (or on first request), fetch JWKS from `GET /oauth/v2/keys`.
2. Cache the keys locally (they rarely change).
3. For each request, parse and verify the JWT signature using the cached public key.
4. Check standard claims: `exp`, `iss`, `aud`.
5. Extract roles and scopes from the validated payload.

**Zitadel JWKS endpoint:**
```
GET http://localhost:8090/oauth/v2/keys
```

**When to use:** High-throughput APIs where eliminating the network round-trip matters, or when Zitadel availability should not be in the hot path.

**Pros:**
- No network call per request — very low latency
- Zitadel only needs to be reachable for key rotation, not per-request
- Scales linearly with request volume

**Cons:**
- Revoked tokens remain valid until they expire (delay of up to the token TTL)
- Must handle key rotation (watch JWKS for new kids)
- More code to maintain (JWT parsing, signature verification, key caching)

**Conceptual code snippet (not yet implemented in this project):**

```go
import "github.com/zitadel/oidc/v3/pkg/oidc"

// Fetch OIDC provider config, which includes the JWKS URI
provider, err := oidc.NewProvider(ctx, "http://localhost:8090")
if err != nil {
    log.Fatal(err)
}

// Use the provider's key set to verify tokens
keySet := provider.RSAPublicKeys(ctx)
token, err := jwt.ParseWithClaims(rawToken, &claims{}, func(token *jwt.Token) (interface{}, error) {
    kid := token.Header["kid"].(string)
    return keySet.GetKey(kid)
})
```

For production use, consider [`github.com/zitadel/zitadel-go`](https://github.com/zitadel/zitadel-go) which handles key caching, rotation, and claim parsing for you.

### 4.3 OIDC Authorization Code Flow with PKCE

Used when a human user logs in via a browser. The browser never handles client secrets — PKCE replaces the secret with a cryptographic challenge.

**Flow:**

1. Browser generates a random `code_verifier` and computes `code_challenge = BASE64URL(SHA256(code_verifier))`.
2. Browser redirects to Zitadel: `GET /oauth/v2/authorize?response_type=code&client_id=...&redirect_uri=...&code_challenge=...&code_challenge_method=S256`
3. User authenticates on the Zitadel Login v2 UI (http://localhost:3000).
4. Zitadel redirects back with `?code=AUTH_CODE`.
5. Browser POSTs to Zitadel: `POST /oauth/v2/token` with `grant_type=authorization_code&code=AUTH_CODE&code_verifier=...`
6. Zitadel returns `access_token`, `id_token`, `refresh_token`.

**Best for:** SPAs, mobile apps, server-rendered web apps where the user logs in interactively.

```bash
# Step 1: Construct the authorization URL (browser navigates here)
AUTH_URL="http://localhost:8090/oauth/v2/authorize\
?response_type=code\
&client_id=<web-app-client-id>\
&redirect_uri=http://localhost:3000/callback\
&scope=openid+profile+email\
&code_challenge=<challenge>\
&code_challenge_method=S256\
&state=<random-state>"

# Step 5: Exchange code for tokens (after redirect back)
curl -X POST http://localhost:8090/oauth/v2/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=authorization_code" \
  --data-urlencode "code=<AUTH_CODE>" \
  --data-urlencode "redirect_uri=http://localhost:3000/callback" \
  --data-urlencode "client_id=<web-app-client-id>" \
  --data-urlencode "code_verifier=<original-code-verifier>"
```

### 4.4 Client Credentials (M2M)

Machine-to-machine authentication where no user is involved. A service authenticates directly with its own client ID and secret.

**Flow:**

1. Service POSTs to `POST /oauth/v2/token` with `grant_type=client_credentials`.
2. Zitadel returns an `access_token`.
3. Service uses the token in API calls.
4. Service caches the token until near-expiry, then requests a new one.

**Best for:** Background jobs, microservice-to-microservice calls, CLI tools, cron jobs.

```bash
# Get a client credentials token
curl -X POST http://localhost:8090/oauth/v2/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --user "<client-id>:<client-secret>" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "scope=openid docs:read drive:write"

# Response:
# {
#   "access_token": "...",
#   "token_type": "Bearer",
#   "expires_in": 3600
# }
```

The helper script `scripts/get-token.sh` automates this:

```bash
TOKEN=$(./scripts/get-token.sh \
  --client-id     "$ZITADEL_CLIENT_ID" \
  --client-secret "$ZITADEL_CLIENT_SECRET" \
  --scope         "openid docs:read docs:write")
```

### 4.5 Personal Access Tokens (PAT)

PATs are static bearer tokens generated for a machine user in the Zitadel console. They are conceptually equivalent to GitHub personal access tokens.

**How to use:**

```bash
# Use a PAT exactly like any other bearer token
curl -H "Authorization: Bearer <PAT>" http://localhost:8083/api/me
```

**Best for:**
- Local development (avoid managing OAuth2 flows)
- CI/CD pipelines (stored as secrets)
- Ad-hoc scripts
- Terraform Zitadel provider authentication

**Important cautions:**
- PATs do not automatically expire unless you set an expiry date during creation
- Treat PATs like passwords — store in secrets managers, not in code
- Rotate PATs periodically (Zitadel does not force rotation)
- If a PAT is leaked, delete it immediately in the console under the machine user's PAT tab

---

## 5. Authorization Patterns

### 5.1 Scope-Based Access Control

OAuth2 scopes define what actions a token is permitted to perform. The client requests scopes when obtaining a token, and the API enforces them.

**Custom scopes in this project:**

| Scope | Description |
|-------|-------------|
| `docs:read` | Read documents (list, view) |
| `docs:write` | Create and modify documents |
| `drive:read` | List and download files from Drive |
| `drive:write` | Upload and modify files in Drive |
| `openid` | Standard OIDC scope (always include) |

Scopes are included in the `scope` field of the introspection response as a space-separated string: `"openid docs:read drive:write"`.

**Middleware from `poc/workspace/main.go`:**

```go
// hasScope reports whether the space-separated scope string contains required.
func hasScope(scope, required string) bool {
    for _, s := range strings.Fields(scope) {
        if s == required {
            return true
        }
    }
    return false
}
```

Note: Zitadel does not enforce custom scopes automatically. The API is responsible for checking whether the token's scope claim includes the required scope. Zitadel passes through whatever scopes were requested (and granted).

**To configure scopes in Zitadel:**

Custom scopes are not explicitly configured in Zitadel UI — the client simply requests them and Zitadel includes them in the token. For fine-grained scope control (e.g., only allow certain clients to request certain scopes), use Zitadel Actions to validate and filter requested scopes.

### 5.2 Role-Based Access Control (RBAC)

Roles are defined at the project level in Zitadel and granted to users via User Grants. When a user authenticates, Zitadel includes their roles in the token.

**Role claim in the token:**

```json
{
  "urn:zitadel:iam:org:project:roles": {
    "editor": { "org-id-abc123": "My Organization" },
    "viewer": { "org-id-abc123": "My Organization" }
  }
}
```

**Role hierarchy in this project (most permissive last):**

```
viewer  →  read-only access (GET endpoints)
editor  →  viewer + create/modify (POST endpoints)
owner   →  editor + delete + management
admin   →  full system access including admin panel
```

**Middleware from `poc/workspace/main.go`:**

```go
// hasRole reports whether the Zitadel roles map contains the required role.
func hasRole(roles map[string]map[string]string, required string) bool {
    if roles == nil {
        return false
    }
    _, ok := roles[required]
    return ok
}

// requireAnyRole allows access when the user has the given scope OR any listed role.
func requireAnyRole(scope string, roles []string, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        user, ok := userFromCtx(r.Context())
        if !ok {
            writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
            return
        }
        if hasScope(user.Scope, scope) {
            next.ServeHTTP(w, r)
            return
        }
        for _, role := range roles {
            if hasRole(user.ZitadelRoles, role) {
                next.ServeHTTP(w, r)
                return
            }
        }
        writeJSON(w, http.StatusForbidden, map[string]string{
            "error":          "insufficient permissions",
            "required_scope": scope,
            "required_roles": strings.Join(roles, ", "),
        })
    })
}
```

**Route-to-permission mapping:**

| Route | Method | Required scope | Required role(s) |
|-------|--------|----------------|-----------------|
| `/health` | GET | (public) | (public) |
| `/api/me` | GET | any valid token | any valid token |
| `/api/docs` | GET | `docs:read` | `viewer`, `editor`, or `owner` |
| `/api/docs` | POST | `docs:write` | `editor` or `owner` |
| `/api/drive` | GET | `drive:read` | `viewer`, `editor`, or `owner` |
| `/api/drive/upload` | POST | `drive:write` | `editor` or `owner` |
| `/api/admin/users` | GET | (none) | `admin` only (strict) |

**Important:** For `Assert Roles on Authentication` to work, you must enable it on the project settings page in the Zitadel console. Without it, the roles claim is omitted from tokens.

### 5.3 Organization-Based Multi-Tenancy

Zitadel's native multi-tenancy maps cleanly to B2B SaaS: each customer company becomes a Zitadel Organization. Data isolation is enforced by including the organization ID in every database query.

**Org ID claim in the token:**

```json
{
  "urn:zitadel:iam:user:resourceowner:id": "org-id-abc123",
  "urn:zitadel:iam:user:resourceowner:name": "Acme Corporation",
  "urn:zitadel:iam:user:resourceowner:domain": "acme.example.com"
}
```

**Data isolation pattern:**

```go
// In every database query, scope by the org from the token
func listDocuments(ctx context.Context, db *sql.DB, orgID string) ([]Document, error) {
    return db.QueryContext(ctx,
        "SELECT id, title FROM documents WHERE org_id = $1",
        orgID,
    )
}

// Extract org ID from the introspection response in the handler
user, _ := userFromCtx(r.Context())
// The resourceowner claim is available in the full introspection response
// (extend the introspectionResponse struct to include it)
docs, err := listDocuments(r.Context(), db, user.OrgID)
```

**Cross-org access** is handled via Zitadel Grants: an organization can grant a user from another org access to a project, enabling partner access, admin visibility across orgs, and federated scenarios.

### 5.4 Attribute-Based Access Control (ABAC)

ABAC combines multiple attributes (who the user is, what resource they are accessing, under what conditions) for fine-grained decisions. Zitadel supports ABAC through custom claims injected by Actions.

**When pure RBAC is not enough:**

- A user has the `editor` role but should only edit documents they own or are shared with
- A user's access should change based on time of day, IP address, or subscription tier
- Claims from an external system (HR database, entitlement service) should influence access

**Pattern using Zitadel Actions:**

```javascript
// Zitadel Action: runs after token issuance
// Adds a custom claim with the user's subscription tier from an external API
function setCustomClaims(ctx, api) {
  const tier = fetchSubscriptionTier(ctx.v1.user.username);
  api.v1.claims.setClaim("subscription_tier", tier);
  api.v1.claims.setClaim("max_storage_gb", tier === "pro" ? 100 : 5);
}
```

The Go API then reads `subscription_tier` from the introspection response (by extending the struct with the custom field) and makes access decisions accordingly.

**Combined check example:**

```go
// Check: user has editor role AND is listed as a collaborator on this document
func canEditDocument(user introspectionResponse, doc Document) bool {
    if !hasRole(user.ZitadelRoles, "editor") && !hasRole(user.ZitadelRoles, "owner") {
        return false // base role check
    }
    // Resource-level check in the application
    return doc.OwnerID == user.Sub || slices.Contains(doc.EditorIDs, user.Sub)
}
```

---

## 6. Good Practices

### 6.1 Security

**Token validation**
- Always validate tokens server-side. Never trust claims passed from a client in query parameters or request bodies.
- For introspection-based APIs, ensure the `active` field is `true` before reading any other claim.
- Validate the `aud` (audience) claim to prevent token confusion attacks — a token issued for Service A should not be accepted by Service B.

**Token lifetimes**
- Use short-lived access tokens: 5–15 minutes for high-security APIs, up to 1 hour for general use. Zitadel defaults to 12 hours — reduce this for production.
- Implement refresh token rotation: each refresh issues a new refresh token and invalidates the old one. Enable this in Zitadel's OIDC settings.

**Client security**
- Use PKCE for all public clients (SPAs, mobile apps) — never use implicit flow.
- Rotate client secrets periodically for confidential clients. Update all deployments atomically.
- Store client secrets in a secrets manager (Vault, AWS Secrets Manager, 1Password Secrets Automation) — never in source code or unencrypted config files.

**Network security**
- Use TLS (HTTPS) for all Zitadel endpoints in production. The Docker Compose here disables TLS for local dev only.
- Restrict introspection endpoint calls to backend services only — never call introspect from a browser.
- Apply principle of least privilege: each service should have its own API application with only the scopes and roles it needs.

**PAT hygiene**
- Set expiry dates on all PATs. Prefer JWT profile authentication for production machine users.
- Audit PAT usage periodically via Zitadel's event log.
- Revoke PATs immediately on employee offboarding or suspected compromise.

### 6.2 Performance

**Cache introspection results**

Each introspection call adds 5–50ms of latency. A short-TTL cache eliminates most of this cost:

```go
type introspectionCache struct {
    mu      sync.Mutex
    entries map[string]cacheEntry
}

type cacheEntry struct {
    result    introspectionResponse
    expiresAt time.Time
}

func (c *introspectionCache) get(tokenHash string) (introspectionResponse, bool) {
    c.mu.Lock()
    defer c.mu.Unlock()
    entry, ok := c.entries[tokenHash]
    if !ok || time.Now().After(entry.expiresAt) {
        return introspectionResponse{}, false
    }
    return entry.result, true
}
```

Cache TTL recommendation: 30–60 seconds. This is a trade-off — longer TTL means faster APIs but slower revocation propagation. For admin operations or sensitive writes, skip the cache and always introspect live.

**Use JWT validation for high-throughput reads**

For endpoints that handle thousands of requests per second (document reads, file listings), local JWT validation eliminates all per-request network calls. Reserve introspection for writes and admin operations where immediate revocation matters.

**HTTP client pooling**

The introspection HTTP client should be shared and reuse connections:

```go
// Good: shared client with connection pooling
httpClient := &http.Client{
    Timeout: 5 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    },
}
```

**Circuit breaker**

If Zitadel becomes unavailable, introspection calls will fail and block goroutines for the full timeout duration. Wrap introspection calls with a circuit breaker (e.g., `github.com/sony/gobreaker`) to fail fast and protect your service.

### 6.3 Architecture

**Separate auth concerns into middleware**

Keep authentication and authorization out of business logic handlers. Use middleware layers:

1. `authMiddleware` — validates the token, populates context
2. `requireScopeOrRole` / `requireRole` — enforces access control
3. Handler — reads user from context, runs business logic

**Use context propagation**

Store the full introspection response in the request context (as this project does). Handlers read the user from context rather than re-parsing headers or re-calling Zitadel:

```go
type userContext struct{}

// Store in middleware
ctx := context.WithValue(r.Context(), userContext{}, introspectionResult)

// Read in handler
user, ok := userFromCtx(r.Context())
```

**Plan for multi-tenancy from day one**

Retrofitting multi-tenancy is painful. From the start:
- Include `org_id` in every data model
- Filter every query by `org_id` from the token
- Design APIs to be org-scoped by default

**Use Zitadel Actions for cross-cutting logic**

Rather than duplicating logic across services (e.g., "users on the free plan cannot create more than 10 documents"), centralize it in Zitadel Actions. The Action adds a custom claim, and each service enforces based on that claim without duplicating the business rule.

**Design roles to be composable**

Avoid role explosion. Start with a small set (viewer, editor, owner, admin) and use resource-level permissions in the application for fine-grained control. Only add a new Zitadel role when it genuinely represents a distinct identity-level permission.

### 6.4 Development Workflow

**Use PATs for local development**

Instead of managing OAuth2 flows during development, create a machine user and generate a PAT. Set it as an environment variable and use it in all local API calls.

```bash
export DEV_TOKEN="<your-pat-from-zitadel-console>"
curl -H "Authorization: Bearer $DEV_TOKEN" http://localhost:8083/api/me
```

**Automate Zitadel setup with Terraform**

The official Zitadel Terraform provider (`registry.terraform.io/providers/zitadel/zitadel`) can automate all console steps:

```hcl
resource "zitadel_project" "workspace" {
  name   = "Workspace"
  org_id = data.zitadel_org.default.id
  project_role_assertion = true
}

resource "zitadel_project_role" "viewer" {
  project_id   = zitadel_project.workspace.id
  org_id       = data.zitadel_org.default.id
  role_key     = "viewer"
  display_name = "Viewer"
  group        = "workspace"
}
```

**Use the management API for programmatic setup**

All Zitadel console actions are available via gRPC/REST management APIs. This is useful for seeding test data or automating environment setup in CI:

```bash
# Create a user via management API
curl -X POST http://localhost:8090/v2/users/human \
  -H "Authorization: Bearer <admin-pat>" \
  -H "Content-Type: application/json" \
  -d '{"username": "test@example.com", "profile": {"given_name": "Test", "family_name": "User"}, "email": {"email": "test@example.com", "is_email_verified": true}}'
```

**Seed test data with scripts**

Maintain a `scripts/seed.sh` that creates test users, assigns roles, and generates tokens. Run it once after `docker compose up` to bootstrap a ready-to-use local environment.

---

## 7. Real-World Examples

### 7.1 How Companies Use Zitadel (Patterns)

**SaaS Platform (Multi-tenant B2B)**

A project management tool (like Jira) where each company is an isolated tenant:

- Each customer company → one Zitadel Organization
- Customer admins are granted the `org:admin` role, allowing them to invite teammates
- Custom branding (logo, colors) configured per organization via Zitadel's private labeling
- The application scopes every database query by the org ID from the token
- External auditors from other orgs access specific projects via cross-org Grants

**Microservices Architecture**

An e-commerce platform with order, inventory, payment, and notification services:

- API Gateway handles JWT validation — services behind the gateway trust forwarded headers
- Each service has its own machine user with a client credentials token for inter-service calls
- The notification service uses `drive:read` scope to read templates from Drive
- The payment service requires the `admin` role for refund operations
- Services cache M2M tokens locally and refresh before expiry

```
Client → API Gateway (JWT validation) → Order Service → Inventory Service (M2M token)
                                                       → Payment Service (M2M token)
                                                       → Notification Service (M2M token)
```

**Developer Platform**

A platform like GitHub where developers access APIs programmatically:

- Human users get PATs for CLI access (`gh auth login` equivalent)
- Third-party integrations use OAuth2 Authorization Code + PKCE
- Scopes act as rate limit tiers: `api:free` (1000 req/day), `api:pro` (100k req/day)
- Zitadel Actions inject the tier claim, which the API Gateway reads for rate limiting
- Machine users represent installed GitHub Apps with their own client credentials

### 7.2 This Project's Architecture

**Basic API (`main.go`, port 8082)**

- Single service with one public endpoint and one protected endpoint
- Uses token introspection for auth
- Demonstrates the minimal viable middleware pattern
- Good starting point for adding introspection to any existing Go HTTP service

**Workspace POC (`poc/workspace/main.go`, port 8083)**

- Simulates a multi-service workspace (Docs + Drive + Admin) in a single binary
- Token introspection for all protected routes
- Scope-based access: `docs:read`, `docs:write`, `drive:read`, `drive:write`
- Role-based access: `viewer`, `editor`, `owner`, `admin`
- Strict admin endpoint: requires `admin` role with no scope fallback
- Context propagation: full user profile available in all handlers
- Built-in `.env` file loader for zero-dependency local setup

```
GET  /health              — public liveness probe
GET  /api/me              — any authenticated user (profile from token)
GET  /api/docs            — docs:read OR viewer/editor/owner
POST /api/docs            — docs:write OR editor/owner
GET  /api/drive           — drive:read OR viewer/editor/owner
POST /api/drive/upload    — drive:write OR editor/owner
GET  /api/admin/users     — admin role only (strict)
```

---

## 8. Future Architecture: Google Workspace Clone

### 8.1 Vision

The end-goal is a full Google Workspace clone demonstrating real-world IAM patterns at scale: Docs, Drive, Sheets, Calendar, Admin Console, and more — all backed by Zitadel for identity and authorization.

### 8.2 Service Breakdown

| Service | Description | Auth Pattern | Roles |
|---------|-------------|-------------|-------|
| Auth Gateway | Centralized JWT validation, header forwarding | JWT validation (local) | - |
| Docs Service | Document CRUD, real-time collaboration | Scope + RBAC | viewer, editor, owner |
| Drive Service | File storage, folder hierarchy, sharing | Scope + RBAC | viewer, editor, owner |
| Sheets Service | Spreadsheet CRUD, formulas | Scope + RBAC | viewer, editor, owner |
| Calendar Service | Events, scheduling, invites | Scope + RBAC | viewer, editor, attendee, organizer |
| Admin Console | User management, org settings, audit logs | RBAC (strict) | admin, super_admin |
| Notification Service | Email, push, in-app notifications | M2M only | - |
| Sharing Service | Cross-service permissions, link sharing | RBAC + resource-level | - |
| Search Service | Full-text search across Docs + Drive | M2M + scope | - |

### 8.3 Approach Strategy

**Phase 1 — Foundation**

- Zitadel configured with proper org structure and all project roles
- API Gateway implementation with JWT validation (eliminates introspection for reads)
- Service-to-service auth using client credentials and token caching
- Terraform provider setup for Zitadel config-as-code (reproducible environments)
- Base roles defined: `viewer`, `editor`, `owner`, `admin`, `super_admin`
- Custom scopes defined for each service: `docs:*`, `drive:*`, `sheets:*`, `calendar:*`

**Phase 2 — Core Services**

- Docs Service: document CRUD, version history, document-level ACL
- Drive Service: file/folder CRUD, folder permissions, recursive permission inheritance
- Sharing Service: link sharing (anyone with link), explicit user grants, expiring shares

**Phase 3 — Extended Services**

- Sheets Service: spreadsheet operations
- Calendar Service: event scheduling, attendee management, shared calendars
- Admin Console: org management, user lifecycle, audit log viewer

**Phase 4 — Advanced**

- Cross-organization sharing (external sharing like Google's "Share with people outside your org")
- Custom branding per organization (logo, color scheme via Zitadel private labeling)
- SSO/SAML federation for enterprise customers (bring your own IdP)
- Comprehensive audit logging via Zitadel Actions and event streaming
- Zitadel Actions for custom claim injection (subscription tier, feature flags)
- Rate limiting at the Gateway based on user/org and subscription tier

### 8.4 Resource-Level Permissions

Zitadel handles **who the user is** and their **base role**. The application handles **which specific resources** they can access.

```
Zitadel (Identity Layer)          Application (Resource Layer)
├── User: john@acme.com           ├── Document: doc-123
├── Role: editor                  │   ├── owner: john@acme.com
├── Org: acme-corp                │   ├── editors: [jane@acme.com, bob@acme.com]
└── Scopes: [docs:read,write]     │   └── viewers: [team-alpha-group]
                                  └── Decision: hasRole(editor) AND isDocMember(doc-123, user.Sub)
```

**Decision flow:**

1. `authMiddleware` — validates token, populates user context
2. `requireAnyRole` — checks base role (coarse-grained)
3. Handler fetches the resource from the database
4. Handler checks resource-level membership (fine-grained)
5. Returns the resource or 403

```go
func handleGetDocument(w http.ResponseWriter, r *http.Request) {
    user, _ := userFromCtx(r.Context())
    docID := r.PathValue("id")

    doc, err := db.GetDocument(r.Context(), docID, user.OrgID) // org-scoped
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
        return
    }

    // Resource-level check: is this user a member of this document?
    if !doc.HasMember(user.Sub) {
        writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
        return
    }

    writeJSON(w, http.StatusOK, doc)
}
```

### 8.5 API Gateway Pattern

The API Gateway eliminates per-service introspection latency by validating JWTs once and forwarding claims as trusted headers.

**Gateway responsibilities:**
- Validate JWT signature using Zitadel's JWKS (cached, periodic refresh)
- Check token expiry and audience
- Forward validated claims as internal headers:
  - `X-User-ID`: subject from token
  - `X-User-Email`: user email
  - `X-User-Roles`: comma-separated role list
  - `X-User-Org`: organization ID
  - `X-User-Scopes`: granted scopes
- Rate limiting by user/org at the gateway level
- Route requests to downstream services

**Downstream services** trust the gateway (internal network, mTLS). They read user info from headers without making their own Zitadel calls:

```go
// In a downstream service (no introspection needed)
func authFromGateway(r *http.Request) (User, error) {
    userID := r.Header.Get("X-User-ID")
    if userID == "" {
        return User{}, errors.New("missing X-User-ID header (not routed through gateway?)")
    }
    return User{
        ID:     userID,
        Email:  r.Header.Get("X-User-Email"),
        Roles:  strings.Split(r.Header.Get("X-User-Roles"), ","),
        OrgID:  r.Header.Get("X-User-Org"),
        Scopes: strings.Fields(r.Header.Get("X-User-Scopes")),
    }, nil
}
```

**For write/delete/admin operations** that need real-time revocation, the gateway (or the downstream service) can optionally call introspect to get a live token status, bypassing the JWT TTL.

### 8.6 Multi-Tenancy Strategy

| Concern | Implementation |
|---------|---------------|
| User isolation | Each user belongs to exactly one organization |
| Data isolation | Every DB table includes `org_id`; all queries filter by it |
| Role isolation | Roles are granted per-org; users have different roles in different orgs |
| Billing isolation | Subscription tier stored as a custom claim via Actions |
| Admin visibility | `super_admin` role grants cross-org visibility in the Admin Console |
| Cross-org sharing | Zitadel Grants + application-level sharing records |
| Custom branding | Zitadel private labeling per org (logo, colors, domain) |

**Admin Console access model:**

```
super_admin → sees all organizations, can manage any user
org_admin   → sees their organization only, can manage org members
admin       → same as org_admin (project-level role)
```

A `super_admin` is identified by holding the `super_admin` role in the IAM (instance-level) project, not in the Workspace project. The Admin Console checks both:

```go
func isSuperAdmin(user introspectionResponse) bool {
    return hasRole(user.ZitadelRoles, "super_admin")
}

func isOrgAdmin(user introspectionResponse, targetOrgID string) bool {
    if isSuperAdmin(user) {
        return true // super admin can manage any org
    }
    return hasRole(user.ZitadelRoles, "admin") && user.OrgID == targetOrgID
}
```

---

## 9. Zitadel API Reference (Quick Reference)

All endpoints are relative to `http://localhost:8090` in local development. In production, replace with your Zitadel domain.

### OAuth2 / OIDC Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/.well-known/openid-configuration` | GET | OIDC discovery document (all endpoint URLs) |
| `/oauth/v2/authorize` | GET | Start authorization code flow |
| `/oauth/v2/token` | POST | Exchange code/credentials for tokens |
| `/oauth/v2/introspect` | POST | Validate a token (RFC 7662) |
| `/oauth/v2/revoke` | POST | Revoke an access or refresh token |
| `/oauth/v2/keys` | GET | JWKS public keys for JWT validation |
| `/oauth/v2/userinfo` | GET | Get user info from OIDC access token |
| `/oauth/v2/end_session` | GET | Logout / end session |

### Management API (v1)

Requires an admin PAT or management API token. Base path: `http://localhost:8090/management/v1`.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/projects` | POST | Create a new project |
| `/projects/{id}` | GET / PUT | Get or update a project |
| `/projects/{id}/roles` | POST | Create a role in a project |
| `/projects/{id}/roles/{key}` | DELETE | Delete a role |
| `/projects/{id}/apps` | POST | Create an application in a project |
| `/users` | POST | Create a human user (v1) |
| `/users/machine` | POST | Create a machine user (v1) |
| `/users/{id}/grants` | POST | Grant roles to a user |
| `/users/{id}/pats` | POST | Create a personal access token |

### User API (v2)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v2/users/human` | POST | Create a human user (v2, recommended) |
| `/v2/users/{id}` | GET | Get user by ID |
| `/v2/users/{id}` | DELETE | Delete a user |
| `/v2/users/{id}/email` | PUT | Update user email |
| `/v2/users/{id}/password` | POST | Set user password |

### Example: Get Token via Client Credentials

```bash
curl -X POST http://localhost:8090/oauth/v2/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --user "<CLIENT_ID>:<CLIENT_SECRET>" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "scope=openid"
```

### Example: Introspect a Token

```bash
curl -X POST http://localhost:8090/oauth/v2/introspect \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --user "<API_CLIENT_ID>:<API_CLIENT_SECRET>" \
  --data-urlencode "token=<ACCESS_TOKEN>"
```

### Example: Create a Project via Management API

```bash
curl -X POST http://localhost:8090/management/v1/projects \
  -H "Authorization: Bearer <ADMIN_PAT>" \
  -H "Content-Type: application/json" \
  -d '{"name": "Workspace", "projectRoleAssertion": true}'
```

### Example: Grant a Role to a User

```bash
# Get the user ID first
USER_ID=$(curl -s http://localhost:8090/v2/users?userName=john@example.com \
  -H "Authorization: Bearer <ADMIN_PAT>" | jq -r '.result[0].userId')

# Grant role on the Workspace project
curl -X POST "http://localhost:8090/management/v1/users/${USER_ID}/grants" \
  -H "Authorization: Bearer <ADMIN_PAT>" \
  -H "Content-Type: application/json" \
  -d '{
    "projectId": "<WORKSPACE_PROJECT_ID>",
    "roleKeys": ["editor"]
  }'
```

---

## 10. Troubleshooting

### Token introspection returns 401

**Cause:** The client credentials used to call the introspect endpoint are wrong.

**Check:**
```bash
# Verify your credentials work
curl -X POST http://localhost:8090/oauth/v2/introspect \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --user "$ZITADEL_CLIENT_ID:$ZITADEL_CLIENT_SECRET" \
  --data-urlencode "token=test"
# Should return {"active":false} not 401
```

**Fix:** Ensure `ZITADEL_CLIENT_ID` and `ZITADEL_CLIENT_SECRET` match the API application credentials in the Zitadel console. The API application must be of type "API" with "Basic" auth method.

---

### Token is `active: false`

**Cause 1:** Token has expired. Access tokens have a limited TTL (default 12 hours in Zitadel, shorter in production configs).

**Fix:** Request a new token using client credentials or refresh token.

**Cause 2:** Token was revoked (user logged out, admin revoked session, or token was explicitly revoked).

**Fix:** Obtain a new token.

**Cause 3:** Token was issued for a different Zitadel instance or environment.

**Check:** Decode the token at [jwt.io](https://jwt.io) and verify the `iss` (issuer) claim matches your Zitadel URL (`http://localhost:8090`).

---

### Roles not appearing in the token

**Cause 1:** "Assert Roles on Authentication" is not enabled on the project.

**Fix:** In the Zitadel console → Projects → Workspace → Settings → enable "Assert Roles on Authentication". Request a new token after enabling.

**Cause 2:** The user has no role grant for the Workspace project.

**Fix:** Go to Users → click the user → Authorizations tab → New Authorization → select Workspace project → assign roles.

**Cause 3:** The token was issued before the role was granted. Roles are injected at token issuance time.

**Fix:** Request a new token after granting the role.

**Cause 4:** The introspection client is not the same application as the one the token was issued for. Roles are scoped to the project and audience.

**Fix:** Ensure the token was requested with the correct audience or that `ZITADEL_AUDIENCE` is set correctly (or left empty to skip audience validation).

---

### CORS issues

**Cause:** Browser requests to Zitadel are blocked by CORS policy.

**Fix:** In the Zitadel console → Instance Settings → Security Policy → add your frontend origin to the allowed origins list (e.g., `http://localhost:3000`).

For the Go API itself, add CORS middleware:

```go
mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
    w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
    if r.Method == http.MethodOptions {
        w.WriteHeader(http.StatusNoContent)
        return
    }
    // ... route to actual handler
})
```

---

### Login redirect loop

**Cause:** The redirect URI configured in Zitadel does not exactly match the one the client sends.

**Check:** In the Zitadel console → Workspace project → Workspace Web application → Redirect URIs. The URI must match character-for-character including trailing slashes, protocol, and port.

**Common mismatches:**
- `http://localhost:3000/callback` vs `http://localhost:3000/callback/` (trailing slash)
- `https://` vs `http://`
- `localhost:3000` vs `127.0.0.1:3000`

**Fix:** Ensure the redirect URI in your application code exactly matches what is registered in Zitadel.

---

### "token audience mismatch" (403)

**Cause:** `ZITADEL_AUDIENCE` is set in the `.env` file but the token's `aud` claim does not include that value.

**Fix option 1:** Clear `ZITADEL_AUDIENCE` to skip audience validation (acceptable for development).

**Fix option 2:** Ensure the token is requested with the correct audience. When using client credentials, include the project's client ID as the audience by setting it in the token request or Zitadel project settings.

**Fix option 3:** In Zitadel project settings, enable "Check for Project on Authentication" and ensure the client used to get the token is within the Workspace project.

---

### Zitadel console not loading

**Cause:** Docker containers are not running or Zitadel has not finished initialization.

```bash
# Check container status
docker compose ps

# Check Zitadel logs
docker compose logs zitadel --tail=50

# Wait for the health check to pass
docker compose ps --format "{{.Name}} {{.Health}}"
# zitadel should show "healthy"

# If the db is not healthy, check postgres logs
docker compose logs db --tail=20
```

If the database volume is corrupted, reset with:

```bash
docker compose down -v   # removes volumes — destroys all Zitadel data
docker compose up -d
```

---

### Machine user token has no roles

Machine users (service accounts) receive role claims just like human users — but only if they have been explicitly granted roles via User Grants.

**Fix:**
1. In the Zitadel console → Users → Service Accounts tab → click the machine user.
2. Go to **Authorizations** → **New Authorization**.
3. Select the Workspace project and assign the desired roles.
4. Request a new token; roles will now appear in the introspection response.

---

*This documentation covers the `go-zitadel` project. Zitadel runs at `localhost:8090` via `docker compose up -d`. The basic API runs on port `8082`, and the Workspace POC runs on port `8083`. Refer to `architecture.md` for Mermaid sequence and entity diagrams.*
