# Zitadel — Deep Study Notes

A comprehensive reference on how Zitadel works in the real world, how scopes and roles are
modelled, and how to integrate it into Go APIs and beyond.

---

## Table of Contents

1. [What is Zitadel?](#1-what-is-zitadel)
2. [Architecture](#2-architecture)
3. [Zitadel vs. Alternatives](#3-zitadel-vs-alternatives)
4. [Core Structural Concepts](#4-core-structural-concepts)
5. [Users — Human vs. Service](#5-users--human-vs-service)
6. [OAuth2 Scopes in Zitadel](#6-oauth2-scopes-in-zitadel)
7. [Roles and RBAC](#7-roles-and-rbac)
8. [Token Claims Reference](#8-token-claims-reference)
9. [Token Introspection (RFC 7662)](#9-token-introspection-rfc-7662)
10. [JWT Validation via JWKS](#10-jwt-validation-via-jwks)
11. [Client Credentials Flow (M2M)](#11-client-credentials-flow-m2m)
12. [Personal Access Tokens (PATs)](#12-personal-access-tokens-pats)
13. [Zitadel Actions](#13-zitadel-actions)
14. [API Endpoint Reference](#14-api-endpoint-reference)
15. [RBAC and ABAC Best Practices](#15-rbac-and-abac-best-practices)
16. [Real-World Application Patterns](#16-real-world-application-patterns)
17. [Token Exchange and Impersonation](#17-token-exchange-and-impersonation)
18. [Security Best Practices](#18-security-best-practices)
19. [Quick Reference — Scopes and Claims Cheat Sheet](#19-quick-reference--scopes-and-claims-cheat-sheet)

---

## 1. What is Zitadel?

Zitadel is an **open-source, API-first identity and access management (IAM) platform** written in
Go. The tagline: *"quickly set up like Auth0 but open source like Keycloak."*

- Implements: OpenID Connect 1.0, OAuth 2.0/2.1, SAML 2.0, LDAP, SCIM 2.0, Passkeys/FIDO2,
  TOTP/OTP, WebAuthn.
- Available as a **self-hosted binary** (single Go binary, no JVM, no Node runtime) or
  **Zitadel Cloud** (managed SaaS with a generous free tier).
- Licensed under Apache 2.0 (older) / AGPL-3.0 (v3+).
- GitHub: https://github.com/zitadel/zitadel

### What makes it different

- **Event-sourced** — every IAM change is an immutable event in an append-only log, giving a
  full built-in audit trail.
- **Multi-tenancy is first-class** via the Organization model (not bolted on as realms or tenants).
- **No SDK required for the happy path** — pure OAuth2/OIDC means any standards-compliant library
  (or even raw HTTP) integrates cleanly.
- **Extensible without redeployment** via HTTP-based Actions (webhooks called at key lifecycle
  points).

---

## 2. Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     Zitadel Instance                    │
│                                                         │
│  ┌─────────────┐   ┌─────────────┐   ┌──────────────┐  │
│  │     Org A   │   │     Org B   │   │    Org C     │  │
│  │  (tenant 1) │   │  (tenant 2) │   │  (tenant 3)  │  │
│  │             │   │             │   │              │  │
│  │  Users      │   │  Users      │   │  Users       │  │
│  │  IdP config │   │  IdP config │   │  Policies    │  │
│  │  Branding   │   │  Branding   │   │              │  │
│  └──────┬──────┘   └──────┬──────┘   └──────┬───────┘  │
│         │                 │                  │          │
│         └─────────────────▼──────────────────┘          │
│                           │                             │
│                   ┌───────┴────────┐                    │
│                   │    Project     │                    │
│                   │  (owned by     │                    │
│                   │   one org)     │                    │
│                   │                │                    │
│                   │  Applications  │                    │
│                   │  Roles         │                    │
│                   └────────────────┘                    │
│                                                         │
│  CQRS + Event-Sourcing layer   CockroachDB / PostgreSQL │
└─────────────────────────────────────────────────────────┘
```

**Technology stack:**
- Written in Go — fast startup, low memory, single binary.
- CQRS + Event Sourcing — commands write immutable events; queries read projections.
- CockroachDB (cloud-native HA) or PostgreSQL (self-hosted default).
- Login UI v2 — separate Next.js frontend for customisable login pages.

**Key property:** Every user action, policy change, or role assignment is stored as an event with
a timestamp and actor. You can reconstruct the state of any object at any point in time.

---

## 3. Zitadel vs. Alternatives

| Feature | Zitadel | Keycloak | Auth0 | Okta |
|---|---|---|---|---|
| Language | Go | Java (Quarkus) | Proprietary | Proprietary |
| Architecture | Event-sourced CQRS | Relational stateful | Managed SaaS | Managed SaaS |
| Built-in audit trail | Yes (immutable events) | Manual setup | Paid add-on | Enterprise tier |
| Multi-tenancy | Organizations (native) | Realms (isolated silos) | Tenants (limited) | Orgs |
| Open source | Yes (AGPL-3.0) | Yes (Apache 2.0) | No | No |
| Self-hosted | Yes | Yes | Private cloud only | No |
| Kubernetes-native | Yes (operator available) | Operator available | No | No |
| Customisation | HTTP Actions (no redeploy) | Java SPIs (rebuild + redeploy) | JS Rules | Workflows |
| Passkeys / FIDO2 | Included free | Included | Extra tier | Extra cost |
| Pricing (cloud) | DAU-based | Free (RH commercial) | MAU-based | Per user |

### Zitadel vs. Keycloak (most asked comparison)

Keycloak's **realm** model is one realm per tenant — realms do not share users, IdPs, or
projects. This leads to operational overhead as tenants grow. Zitadel's **Organisation** model
shares a single instance while keeping tenants isolated with their own branding, IdPs, and
policies.

Keycloak customisations (SPIs) are Java code bundled into the Keycloak deployment — a version
upgrade often breaks them. Zitadel Actions are external HTTP endpoints — Zitadel calls your
endpoint, you handle the logic, Zitadel updates. No redeployment, no coupling.

### Zitadel vs. Auth0

Auth0 is proprietary SaaS; self-hosting is not possible. Auth0 bills on peak MAUs which can
cause unexpected cost spikes. Zitadel Cloud bills on DAUs and all features are included in the
free tier. Auth0 requires add-ons for features Zitadel ships built-in (MFA, fine-grained RBAC,
multi-tenancy, audit logs).

---

## 4. Core Structural Concepts

### The three-tier hierarchy

```
Instance  →  Organization  →  Project  →  Application / Roles
```

| Level | What it is | Managed by |
|---|---|---|
| Instance | Isolated Zitadel deployment (own domain, SMTP, config) | System API |
| Organization | A tenant — owns users, IdP config, branding, policies | IAM Admin / Org Admin |
| Project | Security context for one app family; defines roles | Project Owner |
| Application | An OAuth2/OIDC/SAML client registered in a project | Project Owner |

### Organizations

- Primary multi-tenancy unit within an instance.
- Each org has its own **login policy** (password strength, MFA enforcement, lockout).
- Each org can configure its own **Identity Provider** (Azure AD, Google Workspace, GitHub…).
- Users belong to **exactly one org** and cannot be moved.
- Login name includes the org domain: `alice@acme.zitadel.cloud`.
- Identified by a unique **organisation domain**.

### Projects and Project Grants

A **project grant** lets one org (the owner) delegate access to its project to another org (the
grantee), optionally restricting which roles the grantee can assign:

```
Octagon owns "Portal" project with roles: reader, writer, admin
          │
          ├── Grants to Pentagon: only reader, writer (admin withheld)
          │       Pentagon's IT admin assigns writer to Dimitri
          │       Pentagon's IT admin assigns reader to Michael
          │
          └── Grants to Triangle: only reader
                  Triangle's IT admin assigns reader to their users
```

Neither Pentagon nor Triangle needs Octagon to manage their individual users. This is the
**B2B self-service** model.

### User Grants (per-user assignments)

- **Project Grant**: organisation-level grant with a subset of roles.
- **User Grant (Authorization)**: role assignment for a specific user within a project.

---

## 5. Users — Human vs. Service

### Human Users

- Represent real people who log in interactively.
- Authenticate via: password, MFA (TOTP/SMS/Email), Passkeys (FIDO2), social/federated IdP.
- Have a rich profile: email, phone, given_name, family_name, locale, nickname.
- Login name: `username@primaryorgdomain`.
- **Cannot** have Personal Access Tokens.

### Service Users (Machine Users)

- Represent backend services, IoT devices, CI/CD pipelines, scripts.
- Minimal profile: only name and description.
- **Only service users** can have Personal Access Tokens (PATs).
- Authenticate via:
  1. **Private Key JWT** — recommended (asymmetric, no secret to leak)
  2. **Client Credentials** — client_id + client_secret
  3. **Personal Access Token** — opaque long-lived token

| Aspect | Human User | Service User |
|---|---|---|
| Login style | Interactive | Non-interactive |
| Authentication | Password / MFA / Passkeys / IdP | JWT / PAT / Client Credentials |
| Profile | Rich | Minimal |
| PAT support | No | Yes |
| Primary use | End users, admins | APIs, pipelines, microservices |

---

## 6. OAuth2 Scopes in Zitadel

### Standard OIDC Scopes

| Scope | Effect |
|---|---|
| `openid` | Required for OIDC; enables ID token |
| `profile` | Includes name, given_name, family_name, locale, etc. |
| `email` | Includes `email` and `email_verified` |
| `phone` | Includes `phone_number` and `phone_number_verified` |
| `address` | Includes `address` |
| `offline_access` | Issues a refresh token (auth code flow only) |

### Reserved Scopes — Zitadel-Specific URNs

These are proprietary scopes that control what appears in tokens. They use URN syntax.

#### Role Claims

| Scope | Effect |
|---|---|
| `urn:zitadel:iam:org:project:role:{rolekey}` | Assert a specific role into the token. E.g. `urn:zitadel:iam:org:project:role:admin` |
| `urn:zitadel:iam:org:projects:roles` | Assert roles for **all projects** in the token audience |

#### Organisation Context

| Scope | Effect |
|---|---|
| `urn:zitadel:iam:org:id:{id}` | User must be a member of this org; resource owner claims added |
| `urn:zitadel:iam:org:domain:primary:{domain}` | Enforce org membership by domain |

#### Audience Control

| Scope | Effect |
|---|---|
| `urn:zitadel:iam:org:project:id:{projectid}:aud` | Add project ID to the `aud` claim |
| `urn:zitadel:iam:org:project:id:zitadel:aud` | Add Zitadel project to `aud` (required to call Zitadel's own APIs) |

#### User Data

| Scope | Effect |
|---|---|
| `urn:zitadel:iam:user:metadata` | Include user metadata (base64 key-value pairs) in token/userinfo |
| `urn:zitadel:iam:user:resourceowner` | Include the user's owning org ID, name, and domain |

#### Identity Provider Redirect

| Scope | Effect |
|---|---|
| `urn:zitadel:iam:org:idp:id:{idp_id}` | Redirect directly to the specified IdP at login (skip IdP picker) |

### How Scopes Work in Practice

Scopes are requested at the **authorization** or **token** endpoint and control what appears in
the resulting access token and userinfo response. Not all scopes are valid for all flows:

- `offline_access` — only works in authorization code flow (requires user interaction).
- `urn:zitadel:iam:org:project:role:*` — works in client credentials too; the role must be
  assigned to the service user.
- `openid` is required for any OIDC flow but optional for pure OAuth2 flows.

---

## 7. Roles and RBAC

### Role Definition

Roles are defined **per project** — they are not global. Each role has three attributes:

| Attribute | Description | Example |
|---|---|---|
| Key | Unique code identifier within the project | `admin`, `role.writer`, `tenant:manager` |
| Display Name | Human-readable label in the console | "Administrator" |
| Group | Optional grouping for console organisation | "read-ops", "write-ops" |

### Role Assignment

Two ways to assign roles to users:

1. **Console**: Project → Authorizations → New → select user → assign role(s).
2. **Management API**: `POST /management/v1/users/{userId}/grants` with project ID and role keys.

Roles are **additive** — a user with `reader` and `writer` inherits both permission sets.

### Enabling Role Assertion in Tokens

Two settings must be enabled (one or both):

**Setting 1 — Project level: "Assert Roles on Authentication"**
- Roles automatically appear in userinfo and introspection responses.
- Does NOT require any special scope.

**Setting 2 — Application level: "User Roles Inside ID Token"**
- Embed role claims directly into the **ID token** (not just userinfo/introspection).
- Useful for SPAs that rely on the ID token rather than calling userinfo.

**Alternative: scope-based role request**
- Include `urn:zitadel:iam:org:project:role:{rolekey}` in the scope request.
- Or `urn:zitadel:iam:org:projects:roles` to assert all roles across all projects in `aud`.

### Role Claim Structure in Tokens

The roles claim is a **nested object**, not a flat array:

```json
"urn:zitadel:iam:org:project:223281986649719041:roles": {
  "admin": {
    "223281939119866113": "acme.zitadel.cloud"
  },
  "developer": {
    "223281939119866113": "acme.zitadel.cloud"
  }
}
```

Breaking this down:

```
Claim key: "urn:zitadel:iam:org:project:{projectId}:roles"
  └── Role name: "admin"
        └── Org ID: "223281939119866113"
              Value: "acme.zitadel.cloud"   ← org primary domain
```

The org ID key tells you **which organisation granted this role** — critical for B2B scenarios
where the same user might have roles granted by multiple organisations.

### Legacy vs. Modern Role Claim Name

| Format | Claim Name | Status |
|---|---|---|
| Legacy | `urn:zitadel:iam:org:project:roles` | Maintained for backward compatibility |
| Modern (preferred) | `urn:zitadel:iam:org:project:{projectId}:roles` | Use for new implementations |

Both can be present simultaneously in the same token response.

### Checking Roles in Your Application Code

```go
// Go — check if a user has a role in a specific project
func hasProjectRole(claims map[string]interface{}, projectID, role string) bool {
    key := "urn:zitadel:iam:org:project:" + projectID + ":roles"
    roles, ok := claims[key].(map[string]interface{})
    if !ok {
        return false
    }
    _, found := roles[role]
    return found
}

// Python — same logic
def has_project_role(claims, project_id, role):
    key = f"urn:zitadel:iam:org:project:{project_id}:roles"
    roles = claims.get(key, {})
    return role in roles
```

---

## 8. Token Claims Reference

### Standard Claims (appear in ID token, userinfo, and introspection)

| Claim | Description |
|---|---|
| `sub` | Subject — user ID |
| `iss` | Issuer — your Zitadel domain (e.g. `https://myapp.zitadel.cloud`) |
| `aud` | Audience — array of client IDs and project IDs |
| `exp` | Expiration Unix timestamp |
| `iat` | Issued-at Unix timestamp |
| `nbf` | Not-before Unix timestamp |
| `jti` | Unique JWT ID |
| `azp` | Authorized party — client ID that requested the token |
| `acr` | Authentication Context Class Reference (ID token only) |
| `amr` | Authentication Method References — e.g. `["pwd", "mfa"]` (ID token only) |
| `auth_time` | Unix timestamp of authentication (ID token only) |
| `sid` | Session ID (ID token only) |
| `nonce` | Client-supplied nonce (ID token only) |
| `preferred_username` | Login name: `user@primarydomain` |
| `email` | Email address (with `email` scope) |
| `email_verified` | Boolean (with `email` scope) |
| `name`, `given_name`, `family_name` | Name fields (with `profile` scope) |
| `locale`, `gender`, `phone_number` | Profile data (with respective scopes) |
| `scope` | Space-delimited granted scopes (introspection and JWT access token) |
| `active` | Boolean — token is valid (introspection only; RFC 7662) |

### Zitadel-Specific Claims

| Claim | Description |
|---|---|
| `urn:zitadel:iam:org:domain:primary` | Organisation's primary domain |
| `urn:zitadel:iam:org:project:{projectId}:roles` | Role map for a specific project (preferred) |
| `urn:zitadel:iam:org:project:roles` | Role map (legacy; omits project ID) |
| `urn:zitadel:iam:user:metadata` | All user metadata (key-value; values base64-encoded) |
| `urn:zitadel:iam:user:resourceowner:id` | Org ID of the user's owning organisation |
| `urn:zitadel:iam:user:resourceowner:name` | Org name |
| `urn:zitadel:iam:user:resourceowner:primary_domain` | Org primary domain |
| `act` | Actor info for token exchange — `{ "iss": "...", "sub": "..." }` |

### Full Real-World Introspection Response Example

```json
{
  "active": true,
  "aud": ["259254409320529922@portal", "259254317079330818"],
  "client_id": "259254409320529922@portal",
  "exp": 1711142274,
  "iat": 1711099074,
  "iss": "https://myinstance.zitadel.cloud",
  "jti": "259380916843970562",
  "nbf": 1711099074,
  "scope": "openid profile email urn:zitadel:iam:org:projects:roles",
  "sub": "333333333333333333",
  "token_type": "Bearer",
  "username": "john.wayne@acme.zitadel.cloud",
  "email": "john.wayne@mydomain.com",
  "email_verified": true,
  "name": "John Wayne",
  "given_name": "John",
  "family_name": "Wayne",
  "locale": "en",
  "preferred_username": "john.wayne@acme.zitadel.cloud",
  "urn:zitadel:iam:org:project:111111111111111111:roles": {
    "admin": {
      "222222222222222222": "zitadel.mydomain.com"
    },
    "developer": {
      "222222222222222222": "zitadel.mydomain.com"
    }
  },
  "urn:zitadel:iam:org:project:roles": {
    "admin": {
      "222222222222222222": "zitadel.mydomain.com"
    }
  },
  "urn:zitadel:iam:user:resourceowner:id": "222222222222222222",
  "urn:zitadel:iam:user:resourceowner:name": "ACME Corp",
  "urn:zitadel:iam:user:resourceowner:primary_domain": "zitadel.mydomain.com"
}
```

---

## 9. Token Introspection (RFC 7662)

### What it is

You POST a token to Zitadel's introspection endpoint; Zitadel returns whether the token is
active and all of its claims. Your API never decodes the token itself — Zitadel is the
authoritative source.

**Key advantage:** Works for **both opaque tokens and JWTs**, and detects **revoked** tokens.
Local JWT signature verification cannot detect revocation.

### Introspection Endpoint

```
POST {domain}/oauth/v2/introspect
Content-Type: application/x-www-form-urlencoded
```

### Registering Your API

Before you can call the introspection endpoint your backend must be registered as an **API
Application** in Zitadel:

1. Project → Applications → New → "API"
2. Choose authentication method: **Basic Auth** or **JWT Profile**.
3. Save the `client_id` and `client_secret` (Basic) or download the JSON key file (JWT Profile).

### Method 1: Basic Authentication (simpler)

```bash
curl -X POST https://myinstance.zitadel.cloud/oauth/v2/introspect \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -H "Authorization: Basic $(echo -n '$CLIENT_ID:$CLIENT_SECRET' | base64)" \
  -d 'token=VjVxyCZmRmWYqd3_F5db9Pb9mHR5fqzhn...'
```

The Basic auth header value is `base64(urlencode(client_id) + ":" + urlencode(client_secret))`.

### Method 2: Private Key JWT (recommended — no secret to leak)

```bash
curl -X POST https://myinstance.zitadel.cloud/oauth/v2/introspect \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer' \
  -d 'client_assertion=eyJhbGciOiJSUzI1Ni...' \
  -d 'token=VjVxyCZmRmWYqd3_F5db9Pb9mHR'
```

The `client_assertion` is a JWT signed with your private key:

```json
// Header
{ "alg": "RS256", "kid": "<YOUR_KEY_ID>" }

// Payload
{
  "iss": "<YOUR_CLIENT_ID>",
  "sub": "<YOUR_CLIENT_ID>",
  "aud": "https://myinstance.zitadel.cloud",
  "exp": 1605183582,
  "iat": 1605179982
}
```

The JWT must not be older than 1 hour (`iat` constraint enforced by Zitadel).

### Introspection Response Fields

| Field | Type | Description |
|---|---|---|
| `active` | bool | **true** = token valid AND caller is in the audience. **false** = expired, revoked, or audience mismatch |
| `sub` | string | User ID |
| `username` | string | Login name (`user@domain`) |
| `scope` | string | Space-delimited granted scopes |
| `aud` | array | Audience the token was issued for |
| `client_id` | string | Client ID that requested the token |
| `exp`, `iat`, `nbf`, `jti` | number/string | Standard time and ID claims |
| `email`, `email_verified` | string/bool | With `email` scope |
| `name`, `given_name`, etc. | string | With `profile` scope |
| `urn:zitadel:iam:org:project:{id}:roles` | object | Role map |

### Responses for Edge Cases

**Token inactive/expired:**
```json
{ "active": false }
```

**Your API failed authentication:**
```
HTTP 401  { "error": "invalid_client" }
```

### Go Implementation (this project's pattern)

```go
func introspectToken(ctx context.Context, client *http.Client, cfg authConfig, token string) (introspectionResult, error) {
    form := url.Values{}
    form.Set("token", token)
    form.Set("token_type_hint", "access_token")

    req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
        cfg.introspectionURL, strings.NewReader(form.Encode()))
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.SetBasicAuth(cfg.clientID, cfg.clientSecret)

    resp, _ := client.Do(req)
    defer resp.Body.Close()

    var result introspectionResult
    json.NewDecoder(resp.Body).Decode(&result)
    return result, nil
}

// Then check:
if !result.Active {
    // 401 — token expired or revoked
}
if !audienceMatches(result.Aud, cfg.requiredAudience) {
    // 403 — wrong audience
}
// Check roles: result.Roles["owner"] != nil
// Check scopes: strings.Contains(result.Scope, "videos:write")
```

### Introspection vs. JWT Local Validation — When to Use Which

| Consideration | Introspection | Local JWT Validation |
|---|---|---|
| Detects revoked tokens | Yes | No |
| Network dependency | Yes (one call per request) | No (after initial JWKS fetch) |
| Works with opaque tokens | Yes | No |
| Latency | +5–20ms | ~0ms |
| Token lifetime | Works regardless | Relies on short lifetimes |
| **Use when** | Sensitive APIs, revocation matters | High-throughput, short-lived tokens |

---

## 10. JWT Validation via JWKS

### JWKS Endpoint

```
GET {domain}/oauth/v2/keys
Cache-Control: max-age=300, must-revalidate   ← cache for 5 minutes
```

Returns all active public keys:

```json
{
  "keys": [
    {
      "use": "sig",
      "kty": "RSA",
      "kid": "key-id-1",
      "alg": "RS256",
      "n": "...",
      "e": "AQAB"
    }
  ]
}
```

### Discovery Document

```
GET {domain}/.well-known/openid-configuration
```

Returns all endpoint URLs including `jwks_uri`, `token_endpoint`, `introspection_endpoint`, etc.

### Supported Signing Algorithms

| Algorithm | Type |
|---|---|
| RS256 | RSA + SHA-256 (default) |
| RS384 / RS512 | RSA + SHA-384/512 |
| ES256 / ES384 / ES512 | ECDSA (P-256 / P-384 / P-521) |
| EdDSA | Ed25519 (RFC 8037) |

### Local Validation Flow

```
1. Receive Bearer token
2. Decode JWT header → get kid (Key ID)
3. Fetch JWKS from {domain}/oauth/v2/keys  (cache 5 min)
4. Find public key matching kid
5. Verify JWT signature using that public key
6. Validate standard claims:
   - iss == your Zitadel domain
   - aud contains your client ID or project ID
   - exp is in the future
   - nbf is in the past
7. Extract role claims from urn:zitadel:iam:org:project:{id}:roles
8. Apply your RBAC/ABAC logic
```

### Key Rotation

Zitadel supports planned key rotation via the Web Keys API:

1. **Generate** a new key pair → it enters "initial" state and is published in JWKS immediately.
2. **Cache** the new key for at least `max-age` time across all service instances.
3. **Activate** the new key → Zitadel starts signing new tokens with it.
4. **Deactivate** the old key → old tokens remain verifiable, new ones use the new key.
5. **Delete** the old key after all tokens it signed have expired.

---

## 11. Client Credentials Flow (M2M)

Used for service-to-service authentication — no user interaction involved.

### Setup

1. Create a **service user** (Users → Service Accounts → New).
2. Generate client credentials: service user → Actions → Generate Client Secret.
3. Note the `client_id` and `client_secret`.
4. Assign the service user any needed roles/grants.

### Token Request

```bash
# Method 1: Basic auth header
curl -X POST https://myinstance.zitadel.cloud/oauth/v2/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -u "$CLIENT_ID:$CLIENT_SECRET" \
  -d 'grant_type=client_credentials' \
  -d 'scope=openid profile urn:zitadel:iam:org:project:id:zitadel:aud'

# Method 2: Credentials in body
curl -X POST https://myinstance.zitadel.cloud/oauth/v2/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d "grant_type=client_credentials" \
  -d "client_id=$CLIENT_ID" \
  -d "client_secret=$CLIENT_SECRET" \
  -d "scope=openid urn:zitadel:iam:org:projects:roles"
```

**Note:** Add `urn:zitadel:iam:org:project:id:zitadel:aud` to the scope when calling Zitadel's
own Management or Admin API.

### Token Response

```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIs...",
  "token_type": "Bearer",
  "expires_in": 43199
}
```

### Private Key JWT Flow (Preferred for M2M — no secret ever transmitted)

```bash
# 1. Create the JWT assertion using your private key (store it as a secret in CI/CD)
# 2. Exchange for an access token
curl -X POST https://myinstance.zitadel.cloud/oauth/v2/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer' \
  -d 'scope=openid urn:zitadel:iam:org:project:id:zitadel:aud' \
  -d "assertion=$SIGNED_JWT"
```

The assertion JWT:

```json
// Header: { "alg": "RS256", "kid": "<key_id_from_zitadel>" }
// Payload:
{
  "iss": "<service_user_id>",
  "sub": "<service_user_id>",
  "aud": "https://myinstance.zitadel.cloud",
  "exp": 1605183582,
  "iat": 1605179982
}
```

---

## 12. Personal Access Tokens (PATs)

PATs are **opaque, long-lived tokens** available **only for service users**. They act as
pre-issued bearer tokens — you don't need to call the token endpoint first.

### When to Use PATs

| Situation | Use PAT? |
|---|---|
| CI/CD pipeline that needs Zitadel API access | Yes — simpler than JWT flow in many CI systems |
| Script or CLI tool | Yes |
| Production service handling user tokens | No — use client credentials or JWT |
| Short-lived sessions | No — PATs are long-lived by nature |

### Creating a PAT

1. Users → Service Accounts → select the service user.
2. Scroll to "Access Tokens" → click "+New".
3. Set an expiration date (best practice: never more than 90 days).
4. **Copy the token immediately** — it is never shown again.

### Usage

```bash
curl -H "Authorization: Bearer <PAT_TOKEN>" \
  https://myinstance.zitadel.cloud/management/v1/users/me
```

### Security Properties

- Truly opaque — no information extractable from the token string itself.
- Grants access to **all rights of the service user** (all manager roles + project grants).
- Best practice: one PAT per use case, minimum required permissions, rotation on schedule.
- Can be revoked instantly from the Zitadel console or API.

---

## 13. Zitadel Actions

Actions let you inject custom logic into Zitadel's request lifecycle via HTTP POST webhooks.
**No Zitadel redeployment required.**

### Three Components

| Component | What it is |
|---|---|
| Target | Your external HTTP endpoint that Zitadel calls |
| Execution | A Zitadel resource that maps a lifecycle event to one or more targets |
| Code | Your custom business logic running at the target |

### Target Types

| Type | Status code handled | Response processed | Can interrupt flow |
|---|---|---|---|
| Webhook | Yes | No | Optional |
| Call (REST Call) | Yes | Yes — can return data to Zitadel | Optional |
| Async | No | No — fire and forget | No |

Each target has a **signing key**. Zitadel adds a `ZITADEL-Signature` HMAC header so your
endpoint can verify the request is genuine.

### Execution Types

**Request executions** — run before Zitadel processes the API call:
- Validate domain restrictions before creating a user.
- Enrich an incoming request.

**Response executions** — run after Zitadel generates its response:
- Provision a newly created user to Stripe, Salesforce, or an LDAP directory.
- Sync user data to external systems.

**Function executions** — run at specific internal functions:
- `PreUserinfo` — before userinfo / ID token / introspection response is built → add custom claims.
- `PreAccessToken` — before JWT access token claims are finalised → add custom claims.
- `PreSAMLResponse` — before SAML assertions.

**Event executions** — run when Zitadel stores a specific event:
- `UserLocked` → send alert notification.
- `OrganizationAdded` → provision resources.
- `UserLoginSucceeded` → sync to audit system.

### Adding Custom Claims via Actions (V2 / HTTP Target)

Your endpoint receives a JSON payload and returns claims to append:

```json
// Response from your endpoint → Zitadel appends these to the token
{
  "append_claims": {
    "department": "engineering",
    "clearance_level": "3",
    "tenant_tier": "enterprise",
    "feature_flags": ["beta_dashboard", "export_csv"]
  }
}
```

**Important:** Custom claim keys must **not** start with `urn:zitadel:iam` — that namespace is
reserved for Zitadel's own claims.

### Adding Custom Claims via Actions (V1 / JS Runtime — older)

```javascript
function addCustomClaims(ctx, api) {
    // ctx.v1.user.grants = all project grants with roles
    if (ctx.v1.user.grants && ctx.v1.user.grants.count > 0) {
        let flatRoles = [];
        ctx.v1.user.grants.grants.forEach(grant => {
            grant.roles.forEach(role => {
                flatRoles.push(grant.projectId + ':' + role);
            });
        });
        api.v1.claims.setClaim('flat_roles', flatRoles);
    }

    // ctx.v1.user.getMetadata() = all user metadata key-value pairs
    let meta = ctx.v1.user.getMetadata();
    let dept = meta.get('department');
    if (dept) {
        api.v1.claims.setClaim('department', dept.value);
    }
}
```

### Forwarding Errors from Your Endpoint

```json
{
  "forwardedStatusCode": 403,
  "forwardedErrorMessage": "Email domain not allowed for registration"
}
```

Only HTTP 4xx status codes are forwarded to the end user; other errors become a generic
`PreconditionFailed`.

### Common Action Use Cases

| Use case | Execution type | Trigger |
|---|---|---|
| Add department / clearance claims | Function | PreUserinfo |
| Restrict signup to specific email domains | Request | AddHumanUser |
| Auto-provision user to Stripe on signup | Response | AddHumanUser |
| Notify on user lockout | Event | UserLocked |
| Auto-assign roles based on external directory | Function | PreUserinfo |
| Convert role map to flat array for legacy systems | Function | PreUserinfo |
| Audit all login events | Event | UserLoginSucceeded |
| Create default resources on org creation | Event | OrganizationAdded |

---

## 14. API Endpoint Reference

### OpenID Connect / OAuth2 Endpoints

| Endpoint | URL | Method |
|---|---|---|
| Discovery | `{domain}/.well-known/openid-configuration` | GET |
| Authorization | `{domain}/oauth/v2/authorize` | GET / POST |
| Token | `{domain}/oauth/v2/token` | POST |
| UserInfo | `{domain}/oidc/v1/userinfo` | GET |
| Introspection | `{domain}/oauth/v2/introspect` | POST |
| JWKS | `{domain}/oauth/v2/keys` | GET |
| Token Revocation | `{domain}/oauth/v2/revoke` | POST |
| End Session | `{domain}/oidc/v1/end_session` | GET |

### Zitadel Management APIs

| API | REST Base | Purpose |
|---|---|---|
| Auth API | `{domain}/auth/v1/` | Operations on the authenticated user (reads `sub` from token) |
| Management API | `{domain}/management/v1/` | Manage orgs, projects, apps, users; requires org context header |
| Admin API | `{domain}/admin/v1/` | Configure the Zitadel instance |
| System API | `{domain}/system/v1/` | Manage multiple instances (self-hosted only) |
| Assets API | `{domain}/assets/v1/` | Upload logos, avatars |

All Zitadel Management/Admin API calls require a Bearer token with the right manager roles, or a
PAT from a service user with those roles.

**Specifying organisation context for the Management API:**
```
x-zitadel-orgid: {organisation_id}
```

### Common Management API Calls

```bash
# Get my own user info
GET {domain}/auth/v1/users/me

# List users in an org
GET {domain}/management/v1/users

# Create user grant (assign role)
POST {domain}/management/v1/users/{userId}/grants
{
  "projectId": "...",
  "roleKeys": ["editor"]
}

# Get user metadata
GET {domain}/management/v1/users/{userId}/metadata

# Set user metadata
PUT {domain}/management/v1/users/{userId}/metadata/{key}
{ "value": "base64(value)" }

# Create PAT for service user
POST {domain}/management/v1/users/{userId}/pats
{ "expirationDate": "2026-12-31T00:00:00Z" }
```

---

## 15. RBAC and ABAC Best Practices

### Pure RBAC (Role-Based Access Control)

Best for: clear, well-defined permission tiers (viewer / editor / owner / admin).

```
1. Define roles in Zitadel with descriptive, version-stable key names.
2. Enable "Assert Roles on Authentication" on the project.
3. Introspect (or validate JWT) on every request.
4. Check the role map in the token using the project-specific claim key.
5. Enforce the minimum required role for each endpoint.
```

**Role design principles:**

- Use fine-grained, purpose-named roles (`doc:viewer`, `doc:editor`) rather than broad
  monolithic ones (`admin`).
- Keep roles additive and composable — don't design mutually exclusive roles.
- Group roles by resource/feature for clarity in the console.
- Avoid more than 10–15 roles per project; use metadata/claims for fine-grained attributes.

### Pure ABAC (Attribute-Based Access Control)

Best for: dynamic, context-sensitive decisions (region, department, clearance level).

```
1. Store attributes as user metadata in Zitadel.
2. Use Actions to inject attributes as custom claims at token issuance.
3. Validate attributes in your API alongside (or instead of) roles.
```

Example metadata flow:
```bash
# Set metadata on a user
PUT {domain}/management/v1/users/{userId}/metadata/department
Body: { "value": "ZW5naW5lZXJpbmc=" }  # base64("engineering")

# Request metadata in token
scope=openid urn:zitadel:iam:user:metadata

# Resulting claim in token
"urn:zitadel:iam:user:metadata": {
  "department": "ZW5naW5lZXJpbmc="
}
# Your API base64-decodes "ZW5naW5lZXJpbmc=" → "engineering"
```

### Hybrid RBAC + ABAC (Recommended Pattern)

Use RBAC for coarse-grained baseline access, ABAC for fine-grained contextual decisions:

```
Request arrives with token
       │
       ▼
[Role check: has "viewer" role?] ─── No ──→ 403
       │ Yes
       ▼
[Attribute check: department == "engineering"?] ─── No ──→ 403
       │ Yes
       ▼
[Attribute check: clearance_level >= 3?] ─── No ──→ 403
       │ Yes
       ▼
Serve request
```

### Scope-Based Access Control (as used in this project)

Map application-level permissions to OAuth2 scopes:

| Scope | Action | Min role |
|---|---|---|
| `resource:view` | GET collection | viewer |
| `resource:read` | GET individual item | viewer |
| `resource:write` | POST / PUT | editor |
| `resource:delete` | DELETE | owner |

A token holder can access an endpoint if they have **either** the required scope **or** a role
that implies that permission — whichever the application logic decides. This is the
`requireScopeOrRole` pattern used in this project.

---

## 16. Real-World Application Patterns

### Pattern 1: SaaS Multi-Tenant Application

```
One Zitadel instance per environment
  └── One Organisation per customer
        └── One shared Project (your app)
              ├── Project granted to Org A (roles: viewer, editor)
              ├── Project granted to Org B (roles: viewer, editor, owner)
              └── Org A/B managers self-assign roles to their users
```

**Token flow:**
1. User logs in at `https://app.example.com`.
2. Redirected to Zitadel with `scope=openid profile urn:zitadel:iam:org:projects:roles`.
3. Zitadel authenticates via the org's configured IdP (if federated).
4. Token issued with the user's org-specific roles.
5. Your API reads `urn:zitadel:iam:org:project:{id}:roles` for RBAC.
6. The org ID key inside the role map identifies the tenant.

### Pattern 2: Microservices with API Gateway

```
User  ──→  API Gateway  ──→  POST /oauth/v2/introspect  ──→  Zitadel
                │                                              │
                │◄─── active: true + claims ──────────────────┘
                │
                ├──→  Service A  (validates JWT locally via JWKS)
                └──→  Service B  (validates JWT locally via JWKS)

Service A  ──→  POST /oauth/v2/token (client_credentials)  ──→  Zitadel
           ←── access_token
           ──→  Service B  (Bearer token)
```

- Gateway does introspection once per external request.
- Backend services validate JWTs locally for speed.
- Service-to-service calls use client credentials.

### Pattern 3: CI/CD Pipeline

```bash
# In GitHub Actions / GitLab CI
JWT=$(sign_jwt --key "$ZITADEL_PRIVATE_KEY" --kid "$KEY_ID" \
  --iss "$SERVICE_USER_ID" --aud "$ZITADEL_DOMAIN")

TOKEN=$(curl -s -X POST "$ZITADEL_DOMAIN/oauth/v2/token" \
  -d "grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer" \
  -d "assertion=$JWT" \
  -d "scope=openid urn:zitadel:iam:org:project:id:zitadel:aud" \
  | jq -r '.access_token')

# Use the token to call Zitadel management APIs or your own APIs
```

Private key is stored as a CI/CD secret. No static secret transmitted over the wire.

### Pattern 4: B2B Partner Portal

```
Your org owns "Portal" project
  └── Roles: reader, writer, admin

Grant to Partner A: reader, writer (admin withheld)
  └── Partner A's IT admin assigns writer to Dimitri
  └── Partner A's IT admin assigns reader to Michael

Grant to Partner B: reader only
  └── Partner B's IT admin assigns reader to their team
```

Each partner configures their own corporate IdP (Azure AD, Okta, etc.) into their Zitadel
Organisation. Users SSO via their corporate credentials — you never manage their passwords.

### Pattern 5: Delegated / White-Label SaaS

Customer gets an Organisation in your Zitadel instance and an "Organisation Manager" role.
They manage:
- Their own user pool.
- Their own IdP (corporate SSO).
- Their own branding (colours, logo, login page).
- Their own login policy (MFA requirements, password rules).

You (the SaaS provider) only manage the Zitadel instance configuration and project definitions.

---

## 17. Token Exchange and Impersonation

Zitadel implements RFC 8693 for token exchange — exchanging one token for another, possibly with
a reduced scope or on behalf of a different user.

### Request

```bash
curl -X POST https://myinstance.zitadel.cloud/oauth/v2/token \
  -d 'grant_type=urn:ietf:params:oauth:grant-type:token-exchange' \
  -d 'subject_token=<token_of_user_to_act_as>' \
  -d 'subject_token_type=urn:ietf:params:oauth:token-type:access_token' \
  -d 'actor_token=<your_service_token>' \
  -d 'actor_token_type=urn:ietf:params:oauth:token-type:access_token' \
  -d 'requested_token_type=urn:ietf:params:oauth:token-type:access_token'
```

### Impersonation Roles Required

| Role | Scope |
|---|---|
| `IAM_ADMIN_IMPERSONATOR` | Can impersonate any user across all orgs |
| `IAM_END_USER_IMPERSONATOR` | Can impersonate end users globally |
| `ORG_ADMIN_IMPERSONATOR` | Can impersonate within one organisation |
| `ORG_END_USER_IMPERSONATOR` | Can impersonate end users within one org |

### Use Cases

- Support staff acting on behalf of a user to debug an issue.
- Reducing token scope before forwarding to an untrusted downstream service.
- Converting an opaque token to a JWT for a service that requires local validation.

---

## 18. Security Best Practices

| Practice | Why |
|---|---|
| Always validate `aud` claim | Prevents token reuse across services |
| Don't rely on `aud` alone | A user can request tokens for any audience; also check roles |
| Use introspection for sensitive endpoints | Detects revocation; local JWT cannot |
| Set short token lifetimes | Reduce the window of a leaked token being usable |
| Use Private Key JWT for your API registration | No client secret to leak or rotate |
| Use Private Key JWT for M2M | No secret transmitted over the wire |
| Enable "Check authorization on Authentication" | Users must have at least one role before getting a token |
| Enable "Check for Project on Authentication" | Verifies the user's org has a grant for your project |
| Limit scope of PATs | One PAT per use case; minimum required permissions |
| Rotate PATs regularly | Set expiration; create a new one before the old one expires |
| Use org-specific JWKS caching | Cache for 5 minutes maximum (respect Cache-Control header) |
| Use the project-specific role claim | `urn:zitadel:iam:org:project:{id}:roles` is more precise than the legacy one |
| Never log tokens | Treat access tokens as passwords in logs and error messages |
| Use HTTPS everywhere | Tokens in transit must be encrypted |

---

## 19. Quick Reference — Scopes and Claims Cheat Sheet

### OAuth2 Endpoints

```
Authorization:  {domain}/oauth/v2/authorize
Token:          {domain}/oauth/v2/token
Introspect:     {domain}/oauth/v2/introspect
JWKS:           {domain}/oauth/v2/keys
UserInfo:       {domain}/oidc/v1/userinfo
Revoke:         {domain}/oauth/v2/revoke
End Session:    {domain}/oidc/v1/end_session
Discovery:      {domain}/.well-known/openid-configuration
```

### Management APIs

```
Auth API:       {domain}/auth/v1/
Management:     {domain}/management/v1/       (add x-zitadel-orgid header)
Admin:          {domain}/admin/v1/
System:         {domain}/system/v1/
```

### Reserved Scope URNs

```
# Role assertion
urn:zitadel:iam:org:project:role:{rolekey}
urn:zitadel:iam:org:projects:roles

# Organisation context
urn:zitadel:iam:org:id:{org_id}
urn:zitadel:iam:org:domain:primary:{domain}

# Audience control
urn:zitadel:iam:org:project:id:{projectid}:aud
urn:zitadel:iam:org:project:id:zitadel:aud

# User data
urn:zitadel:iam:user:metadata
urn:zitadel:iam:user:resourceowner

# IdP redirect
urn:zitadel:iam:org:idp:id:{idp_id}
```

### Token Claim Names

```
# Roles
urn:zitadel:iam:org:project:{projectId}:roles   ← preferred modern form
urn:zitadel:iam:org:project:roles               ← legacy (backward compat)

# User / Org metadata
urn:zitadel:iam:user:metadata
urn:zitadel:iam:user:resourceowner:id
urn:zitadel:iam:user:resourceowner:name
urn:zitadel:iam:user:resourceowner:primary_domain
urn:zitadel:iam:org:domain:primary

# Token exchange actor
act
```

### Role Claim Structure

```json
"urn:zitadel:iam:org:project:{projectId}:roles": {
  "{roleName}": {
    "{orgId}": "{orgPrimaryDomain}"
  }
}
```

→ `roleName` = the role the user holds
→ `orgId` = the organisation that granted the role
→ `orgPrimaryDomain` = human-readable org domain

### Scope → Endpoint → Role Matrix (This Project)

| Scope | HTTP | Path | Roles (fallback) |
|---|---|---|---|
| `videos:view` | GET | `/api/videos` | viewer, editor, owner, admin |
| `videos:read` | GET | `/api/videos/{id}` | viewer, editor, owner, admin |
| `videos:write` | POST | `/api/videos` | editor, owner, admin |
| `videos:write` | PUT | `/api/videos/{id}` | editor, owner, admin |
| `videos:delete` | DELETE | `/api/videos/{id}` | owner, admin |
| `channels:view` | GET | `/api/channels` | viewer, editor, owner, admin |
| `channels:read` | GET | `/api/channels/{id}` | viewer, editor, owner, admin |
| `channels:write` | POST | `/api/channels` | editor, owner, admin |
| `channels:write` | PUT | `/api/channels/{id}` | editor, owner, admin |
| `channels:delete` | DELETE | `/api/channels/{id}` | owner, admin |
| `playlists:view` | GET | `/api/playlists` | viewer, editor, owner, admin |
| `playlists:read` | GET | `/api/playlists/{id}` | viewer, editor, owner, admin |
| `playlists:write` | POST | `/api/playlists` | editor, owner, admin |
| `playlists:write` | PUT | `/api/playlists/{id}` | editor, owner, admin |
| `playlists:delete` | DELETE | `/api/playlists/{id}` | owner, admin |
| `comments:view` | GET | `/api/comments` | viewer, editor, owner, admin |
| `comments:read` | GET | `/api/comments/{id}` | viewer, editor, owner, admin |
| `comments:write` | POST | `/api/comments` | viewer, editor, owner, admin |
| `comments:delete` | DELETE | `/api/comments/{id}` | editor, owner, admin |
| *(role only)* | GET | `/api/admin/users` | admin |
| *(role only)* | DELETE | `/api/admin/users/{id}` | admin |

Use `GET /api/scope-introspect` with any valid token to see this matrix evaluated against
*your specific token* — showing which endpoints are granted, which are denied, and why.

---

## Sources

- https://zitadel.com/docs — Official documentation
- https://zitadel.com/docs/apis/openidoauth/scopes — Scopes reference
- https://zitadel.com/docs/apis/openidoauth/claims — Claims reference
- https://zitadel.com/docs/guides/integrate/token-introspection — Introspection guide
- https://zitadel.com/docs/concepts/structure/organizations — Organisation model
- https://zitadel.com/docs/guides/solution-scenarios/b2b — B2B multi-tenancy
- https://zitadel.com/docs/concepts/features/actions_v2 — Actions V2
- https://zitadel.com/docs/guides/integrate/retrieve-user-roles — Roles guide
- https://zitadel.com/blog/zitadel-vs-keycloak — Comparison
- https://zitadel.com/blog/authorization-with-role-based-access-control — RBAC patterns
- https://zitadel.com/blog/custom-claims — Custom claims via Actions
- https://zitadel.com/docs/guides/integrate/token-exchange — Token exchange / impersonation
- https://zitadel.com/docs/apis/openidoauth/grant-types — Grant types
- https://zitadel.com/docs/guides/integrate/login/oidc/webkeys — JWKS / key rotation
- https://github.com/zitadel/zitadel — Source code
