# Architecture Diagrams

This document contains comprehensive architecture diagrams for the Zitadel-based identity management system, covering the current POC and the planned Google Workspace clone. All diagrams use Mermaid syntax and are intended to be rendered in any Mermaid-compatible viewer (GitHub, GitLab, Obsidian, etc.).

---

## 1. System Overview - Current POC

This C4-style context diagram shows all components running in the current Docker Compose environment. The Go API on port 8082 handles token introspection, while the Workspace POC on port 8083 simulates Google Docs, Drive, and Admin surfaces. Zitadel on port 8090 acts as the central identity provider backed by PostgreSQL, with the Login v2 UI handling the authentication UX.

```mermaid
graph TB
    classDef user fill:#4A90D9,stroke:#2C5F8A,color:#fff
    classDef service fill:#27AE60,stroke:#1A7A42,color:#fff
    classDef identity fill:#E67E22,stroke:#A85A0A,color:#fff
    classDef data fill:#8E44AD,stroke:#5E2D7A,color:#fff
    classDef ui fill:#16A085,stroke:#0C6B5A,color:#fff

    User["User / Browser"]:::user

    subgraph DockerCompose["Docker Compose Environment"]
        GoAPI["Go API\nport 8082\nToken Introspection\nREST Endpoints"]:::service
        WorkspacePOC["Workspace POC\nport 8083\nDocs · Drive · Admin\nGo HTTP Server"]:::service
        Zitadel["Zitadel\nport 8090\nOIDC · OAuth2 · SAML\nIdentity Provider"]:::identity
        PostgreSQL["PostgreSQL\nport 5432\nZitadel Store"]:::data
        LoginV2["Login v2 UI\nZitadel Login\nMFA · Passkeys"]:::ui
    end

    User -->|"HTTP requests"| GoAPI
    User -->|"HTTP requests"| WorkspacePOC
    User -->|"Browser redirect"| LoginV2

    GoAPI -->|"Introspect tokens"| Zitadel
    WorkspacePOC -->|"Introspect / validate tokens"| Zitadel
    Zitadel -->|"Persist data"| PostgreSQL
    LoginV2 -->|"Authenticate via"| Zitadel
```

---

## 2. OIDC Authorization Code Flow with PKCE

This sequence diagram illustrates the full Authorization Code + PKCE flow used by browser-based applications. PKCE (Proof Key for Code Exchange) prevents authorization code interception attacks by binding the token request to a one-time verifier generated in the client. The result is three tokens: an access token for API calls, an ID token for identity claims, and a refresh token for silent renewal.

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant Browser
    participant App as "App (port 8083)"
    participant Zitadel as "Zitadel (port 8090)"
    participant LoginUI as "Login v2 UI"

    User->>Browser: Navigate to protected resource
    Browser->>App: GET /dashboard

    Note over App: Generate code_verifier (random 43-128 chars)<br/>code_challenge = BASE64URL(SHA256(code_verifier))

    App->>Browser: 302 Redirect to Zitadel /authorize
    Browser->>Zitadel: GET /oauth/v2/authorize<br/>?response_type=code<br/>&client_id=...&redirect_uri=...<br/>&scope=openid profile email<br/>&code_challenge=...&code_challenge_method=S256

    Zitadel->>Browser: Redirect to Login UI
    Browser->>LoginUI: GET /login

    LoginUI->>User: Show login form (username + password / passkey)
    User->>LoginUI: Submit credentials

    LoginUI->>Zitadel: POST /session (authenticate user)
    Zitadel-->>LoginUI: Session established

    Zitadel->>Browser: 302 Redirect to redirect_uri?code=AUTH_CODE&state=...
    Browser->>App: GET /callback?code=AUTH_CODE

    Note over App: Retrieve stored code_verifier

    App->>Zitadel: POST /oauth/v2/token<br/>grant_type=authorization_code<br/>&code=AUTH_CODE<br/>&code_verifier=...&redirect_uri=...

    Note over Zitadel: Verify code_verifier against<br/>stored code_challenge

    Zitadel-->>App: 200 OK<br/>access_token + id_token + refresh_token + expires_in

    App->>App: Store tokens (memory / secure cookie)
    App->>Browser: 200 Render /dashboard with user context
    Browser->>User: Show authenticated dashboard
```

---

## 3. Token Introspection Flow

This sequence diagram shows how an API server validates an opaque or JWT access token by calling Zitadel's introspection endpoint. This approach is appropriate when the API does not want to manage JWKS key rotation locally, or when using opaque tokens. The introspection response includes active status, user claims, scopes, and expiry.

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant API as "Go API (port 8082)"
    participant Zitadel as "Zitadel (port 8090)"

    Client->>API: GET /api/resource<br/>Authorization: Bearer access_token

    Note over API: Extract Bearer token from header

    API->>Zitadel: POST /oauth/v2/introspect<br/>token=access_token<br/>Authorization: Basic client_credentials

    alt Token is valid and active
        Zitadel-->>API: 200 OK<br/>active=true, sub=user-id,<br/>username=alice, email=alice@example.com,<br/>scope=openid profile docs:read,<br/>exp=1234567890, iss=https://zitadel:8090

        Note over API: Build userContext from claims<br/>Check required scope/role

        alt Authorization check passes
            API-->>Client: 200 OK + resource data
        else Missing required scope or role
            API-->>Client: 403 Forbidden - insufficient_scope
        end

    else Token is expired or invalid
        Zitadel-->>API: 200 OK - active=false
        API-->>Client: 401 Unauthorized - invalid_token

    else Introspection call fails
        Zitadel-->>API: 5xx or network error
        API-->>Client: 503 Service Unavailable
    end
```

---

## 4. JWT Validation Flow (JWKS)

This sequence diagram shows the high-performance local JWT validation path using JSON Web Key Sets (JWKS). On startup the API fetches and caches Zitadel's public keys; each subsequent request validates the token signature locally without a network call to Zitadel. The cache is periodically refreshed to handle key rotation gracefully.

```mermaid
sequenceDiagram
    autonumber
    participant API as "Go API"
    participant Cache as "JWKS Cache (in-memory)"
    participant Zitadel as "Zitadel (port 8090)"
    actor Client

    Note over API,Zitadel: Startup - Key Fetch

    API->>Zitadel: GET /.well-known/openid-configuration
    Zitadel-->>API: jwks_uri, issuer, token_endpoint, ...

    API->>Zitadel: GET /oauth/v2/keys
    Zitadel-->>API: keys array with kty, kid, n, e fields
    API->>Cache: Store public keys indexed by kid

    Note over API,Client: Per Request Validation

    Client->>API: GET /api/docs<br/>Authorization: Bearer JWT

    API->>API: Decode JWT header - extract kid

    API->>Cache: Lookup public key by kid

    alt Key found in cache
        Cache-->>API: RSA/EC public key
    else Key not in cache - rotation event
        API->>Zitadel: GET /oauth/v2/keys
        Zitadel-->>API: Updated JWKS
        API->>Cache: Refresh cache
        Cache-->>API: RSA/EC public key
    end

    Note over API: Verify JWT signature with public key<br/>Validate: exp > now<br/>Validate: iss == expected issuer<br/>Validate: aud contains client_id<br/>Extract: roles, scopes, sub

    alt JWT valid
        API->>API: Build userContext, run authz checks
        API-->>Client: 200 OK + response data
    else Signature invalid
        API-->>Client: 401 Unauthorized - invalid_signature
    else Token expired
        API-->>Client: 401 Unauthorized - token_expired
    else Audience mismatch
        API-->>Client: 401 Unauthorized - invalid_audience
    end

    Note over API,Cache: Periodic JWKS Refresh (e.g. every 1h)

    loop Every 1 hour
        API->>Zitadel: GET /oauth/v2/keys
        Zitadel-->>API: Current JWKS
        API->>Cache: Update cached keys
    end
```

---

## 5. Client Credentials Flow (M2M)

This sequence diagram illustrates the OAuth 2.0 Client Credentials grant used for machine-to-machine (M2M) communication between backend services. There is no user interaction; Service A authenticates directly with its client credentials to obtain an access token, then uses that token to call Service B. Service B validates the token independently using introspection or local JWKS verification.

```mermaid
sequenceDiagram
    autonumber
    participant ServiceA as "Service A\n(e.g. Notification Svc)"
    participant Zitadel as "Zitadel (port 8090)"
    participant ServiceB as "Service B\n(e.g. Docs Service)"

    Note over ServiceA: Needs to call Service B<br/>No user context available

    ServiceA->>Zitadel: POST /oauth/v2/token<br/>grant_type=client_credentials<br/>&client_id=svc-a-client-id<br/>&client_secret=svc-a-secret<br/>&scope=docs:read docs:write

    Note over Zitadel: Authenticate client credentials<br/>Validate requested scopes<br/>Issue access token (short-lived, e.g. 1h)

    Zitadel-->>ServiceA: 200 OK<br/>access_token, token_type=Bearer,<br/>expires_in=3600, scope=docs:read docs:write

    ServiceA->>ServiceA: Cache token until expiry

    ServiceA->>ServiceB: POST /internal/docs<br/>Authorization: Bearer access_token

    ServiceB->>Zitadel: Validate token via introspect or JWKS
    Zitadel-->>ServiceB: active=true, scope=docs:read docs:write,<br/>client_id=svc-a-client-id

    ServiceB->>ServiceB: Check scope / client_id allowlist
    ServiceB-->>ServiceA: 200 OK + response data

    Note over ServiceA: On token expiry, repeat token fetch
```

---

## 6. RBAC Decision Flow

This flowchart shows the multi-strategy authorization decision tree used in the workspace API middleware. Each endpoint declares its required authorization type (scope-based, role-based, scope-or-role, or resource-level), and the middleware evaluates the extracted claims accordingly. Deny decisions return a 403 response immediately without proceeding to the handler.

```mermaid
graph TD
    classDef decision fill:#E67E22,stroke:#A85A0A,color:#fff
    classDef allow fill:#27AE60,stroke:#1A7A42,color:#fff
    classDef deny fill:#E74C3C,stroke:#A52A1A,color:#fff
    classDef process fill:#3498DB,stroke:#1A6FA0,color:#fff
    classDef start fill:#2C3E50,stroke:#1A252F,color:#fff

    A[/"Incoming Request"/]:::start
    B["Extract Bearer Token\nfrom Authorization header"]:::process
    C{Token present?}:::decision
    D["401 Unauthorized\nmissing token"]:::deny
    E["Validate Token\nJWKS local or introspect"]:::process
    F{Token valid?}:::decision
    G["401 Unauthorized\ninvalid or expired"]:::deny
    H{Token active?}:::decision
    I["401 Unauthorized\ntoken inactive"]:::deny
    J["Build userContext\nsub, email, roles, scopes"]:::process
    K{Required auth type?}:::decision

    L["Scope-based check"]:::process
    M{Has required scope?}:::decision

    N["Role-based check"]:::process
    O{Has required role?}:::decision

    P["Scope OR Role check"]:::process
    Q{Has required scope?}:::decision
    R{Has required role?}:::decision

    S["Resource-level check"]:::process
    T{Has base role?}:::decision
    U["Fetch resource ACL / permissions"]:::process
    V{Has resource permission?}:::decision

    ALLOW1["Allow - Call handler"]:::allow
    ALLOW2["Allow - Call handler"]:::allow
    ALLOW3["Allow - scope"]:::allow
    ALLOW4["Allow - role"]:::allow
    ALLOW5["Allow - resource"]:::allow
    DENY1["403 Forbidden\ninsufficient scope"]:::deny
    DENY2["403 Forbidden\ninsufficient role"]:::deny
    DENY3["403 Forbidden\ninsufficient scope or role"]:::deny
    DENY4["403 Forbidden\nbase role missing"]:::deny
    DENY5["403 Forbidden\nresource access denied"]:::deny

    A --> B --> C
    C -->|No| D
    C -->|Yes| E --> F
    F -->|No| G
    F -->|Yes| H
    H -->|No| I
    H -->|Yes| J --> K

    K -->|scope| L --> M
    M -->|Yes| ALLOW1
    M -->|No| DENY1

    K -->|role| N --> O
    O -->|Yes| ALLOW2
    O -->|No| DENY2

    K -->|scope_or_role| P --> Q
    Q -->|Yes| ALLOW3
    Q -->|No| R
    R -->|Yes| ALLOW4
    R -->|No| DENY3

    K -->|resource| S --> T
    T -->|No| DENY4
    T -->|Yes| U --> V
    V -->|Yes| ALLOW5
    V -->|No| DENY5
```

---

## 7. Google Workspace Clone - Full Architecture

This comprehensive architecture diagram shows the planned full Google Workspace clone with layered separation of concerns. The API Gateway handles all inbound traffic, enforces JWT validation and rate limiting, then routes to purpose-built microservices. Each service owns its own PostgreSQL database, shares a Redis cluster for caching, and stores files in S3-compatible object storage. All services can authenticate with Zitadel via M2M client credentials for inter-service calls.

```mermaid
graph TB
    classDef client fill:#2980B9,stroke:#1A5276,color:#fff
    classDef gateway fill:#1ABC9C,stroke:#0E8070,color:#fff
    classDef service fill:#27AE60,stroke:#1A7A42,color:#fff
    classDef identity fill:#E67E22,stroke:#A85A0A,color:#fff
    classDef data fill:#8E44AD,stroke:#5E2D7A,color:#fff
    classDef storage fill:#C0392B,stroke:#7B241C,color:#fff

    subgraph ClientLayer["Client Layer"]
        WebApp["Web App\nReact / Next.js"]:::client
        MobileApp["Mobile App\niOS / Android"]:::client
        CLI["CLI Tool\ngo binary"]:::client
    end

    subgraph GatewayLayer["API Gateway Layer"]
        Gateway["API Gateway\nJWT Validation\nRate Limiting\nRequest Routing\nCORS / TLS termination"]:::gateway
    end

    subgraph ServiceLayer["Service Layer"]
        AuthSvc["Auth Service\nSession mgmt\nToken refresh\nLogout"]:::service
        DocsSvc["Docs Service\nCreate / edit docs\nVersion history\nComments"]:::service
        DriveSvc["Drive Service\nFile upload/download\nFolder mgmt\nMetadata"]:::service
        SheetsSvc["Sheets Service\nSpreadsheet engine\nFormulas\nCharts"]:::service
        CalSvc["Calendar Service\nEvents\nInvites\nTimezones"]:::service
        AdminSvc["Admin Service\nUser mgmt\nOrg settings\nAudit logs"]:::service
        NotifSvc["Notification Service\nEmail\nPush\nWebSocket"]:::service
        SharingSvc["Sharing Service\nACL mgmt\nPublic links\nCross-org grants"]:::service
    end

    subgraph IdentityLayer["Identity Layer"]
        Zitadel["Zitadel\nOIDC · OAuth2 · SAML\nMFA · Passkeys\nMulti-tenancy"]:::identity
    end

    subgraph DataLayer["Data Layer"]
        PG_Auth[("PostgreSQL\nAuth DB")]:::data
        PG_Docs[("PostgreSQL\nDocs DB")]:::data
        PG_Drive[("PostgreSQL\nDrive DB")]:::data
        PG_Sheets[("PostgreSQL\nSheets DB")]:::data
        PG_Cal[("PostgreSQL\nCalendar DB")]:::data
        PG_Admin[("PostgreSQL\nAdmin DB")]:::data
        PG_Zitadel[("PostgreSQL\nZitadel DB")]:::data
        Redis[("Redis Cluster\nSessions · Caching\nRate limits")]:::data
        S3["S3 / MinIO\nFile Storage\nDoc blobs · Attachments"]:::storage
    end

    WebApp & MobileApp & CLI -->|"HTTPS"| Gateway

    Gateway -->|"auth ops"| AuthSvc
    Gateway -->|"doc ops"| DocsSvc
    Gateway -->|"file ops"| DriveSvc
    Gateway -->|"sheet ops"| SheetsSvc
    Gateway -->|"calendar ops"| CalSvc
    Gateway -->|"admin ops"| AdminSvc
    Gateway -->|"notify ops"| NotifSvc
    Gateway -->|"share ops"| SharingSvc

    Gateway -->|"validate tokens"| Zitadel

    AuthSvc -->|"M2M token"| Zitadel
    DocsSvc -->|"M2M token"| Zitadel
    DriveSvc -->|"M2M token"| Zitadel
    SheetsSvc -->|"M2M token"| Zitadel
    AdminSvc -->|"user mgmt API"| Zitadel

    SharingSvc -->|"sharing rules"| DocsSvc
    SharingSvc -->|"sharing rules"| DriveSvc
    SharingSvc -->|"sharing rules"| SheetsSvc

    NotifSvc -->|"cache"| Redis
    AuthSvc -->|"sessions"| Redis
    Gateway -->|"rate limits"| Redis

    AuthSvc --- PG_Auth
    DocsSvc --- PG_Docs
    DriveSvc --- PG_Drive
    SheetsSvc --- PG_Sheets
    CalSvc --- PG_Cal
    AdminSvc --- PG_Admin
    Zitadel --- PG_Zitadel

    DriveSvc -->|"file blobs"| S3
    DocsSvc -->|"doc exports"| S3
    SheetsSvc -->|"sheet exports"| S3
```

---

## 8. Multi-Tenancy Model

This diagram shows how Zitadel's organizational hierarchy enables multi-tenancy for the workspace clone. Each organization (tenant) has its own isolated user pool, project, roles, and application registrations. Cross-organization grants allow users from one organization to access resources in another with explicitly assigned roles, enabling B2B collaboration scenarios without merging user directories.

```mermaid
graph TB
    classDef instance fill:#2C3E50,stroke:#1A252F,color:#fff
    classDef org fill:#2980B9,stroke:#1A5276,color:#fff
    classDef project fill:#27AE60,stroke:#1A7A42,color:#fff
    classDef user fill:#E67E22,stroke:#A85A0A,color:#fff
    classDef role fill:#8E44AD,stroke:#5E2D7A,color:#fff
    classDef app fill:#16A085,stroke:#0C6B5A,color:#fff
    classDef grant fill:#E74C3C,stroke:#A52A1A,color:#fff

    ZInstance["Zitadel Instance\nhttps://zitadel:8090"]:::instance

    subgraph OrgA["Organization A - Acme Corp"]
        UsersA["Users\nalice (admin)\nbob (editor)"]:::user
        ProjectA["Project: Workspace"]:::project
        RolesA["Roles\nadmin, editor, viewer"]:::role
        AppA["Application\nWeb App Client\nclient_id: acme-web"]:::app
    end

    subgraph OrgB["Organization B - Globex Inc"]
        UsersB["Users\ncharlie (admin)\ndave (viewer)"]:::user
        ProjectB["Project: Workspace"]:::project
        RolesB["Roles\nadmin, editor, viewer"]:::role
        AppB["Application\nWeb App Client\nclient_id: globex-web"]:::app
    end

    CrossGrant["Cross-Org Grant\ncharlie from Globex\ngets viewer role\non Acme's Workspace"]:::grant

    ZInstance --> OrgA
    ZInstance --> OrgB

    OrgA --> UsersA
    OrgA --> ProjectA
    ProjectA --> RolesA
    ProjectA --> AppA

    OrgB --> UsersB
    OrgB --> ProjectB
    ProjectB --> RolesB
    ProjectB --> AppB

    UsersB -->|"charlie gets"| CrossGrant
    CrossGrant -->|"access to"| ProjectA
```

---

## 9. Request Lifecycle (Workspace POC)

This sequence diagram shows the complete middleware chain for an authenticated request in the Workspace POC. Each middleware layer has a distinct responsibility: logging, authentication (token introspection), and authorization (scope or role checking). Only after all middleware passes successfully does the request reach the business logic handler, keeping concerns cleanly separated.

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant Browser
    participant Zitadel as "Zitadel (port 8090)"
    participant WorkspaceAPI as "Workspace API (port 8083)"
    participant LogMW as "loggerMiddleware"
    participant AuthMW as "authMiddleware"
    participant ScopeMW as "requireScopeOrRole"
    participant Handler as "handleListDocs"

    Note over User,Zitadel: Initial Login

    User->>Browser: Navigate to /
    Browser->>WorkspaceAPI: GET /
    WorkspaceAPI->>Browser: Redirect to Zitadel OIDC authorize
    Browser->>Zitadel: GET /oauth/v2/authorize with PKCE params
    Zitadel-->>Browser: Render login page
    User->>Browser: Enter credentials
    Browser->>Zitadel: POST credentials
    Zitadel-->>Browser: Redirect with auth code
    Browser->>WorkspaceAPI: GET /callback?code=...
    WorkspaceAPI->>Zitadel: POST /oauth/v2/token - exchange code
    Zitadel-->>WorkspaceAPI: access_token + id_token + refresh_token
    WorkspaceAPI->>Browser: Set secure cookie, redirect to /dashboard

    Note over Browser,Handler: Authenticated API Request

    Browser->>WorkspaceAPI: GET /api/docs<br/>Authorization: Bearer access_token

    WorkspaceAPI->>LogMW: Pass request
    LogMW->>LogMW: Log: method, path, remote_addr, timestamp
    LogMW->>AuthMW: next(w, r)

    AuthMW->>AuthMW: Extract Bearer token from header
    AuthMW->>Zitadel: POST /oauth/v2/introspect<br/>token=access_token

    alt Introspection succeeds - token active
        Zitadel-->>AuthMW: active=true, sub=...,<br/>scope=openid profile docs:read,<br/>urn:zitadel:iam:org:project:roles=viewer

        AuthMW->>AuthMW: Build userContext<br/>UserID, Email, Username,<br/>Scopes, Roles, OrgID
        AuthMW->>AuthMW: Store userContext in request context
        AuthMW->>ScopeMW: next(w, r)

        ScopeMW->>ScopeMW: Required: scope=docs:read OR role=viewer/editor/owner
        ScopeMW->>ScopeMW: Check: userContext.Scopes contains docs:read

        alt Has docs:read scope
            ScopeMW->>Handler: next(w, r) - scope check passed
        else No scope - check roles
            ScopeMW->>ScopeMW: Check: userContext.Roles contains viewer/editor/owner
            alt Has required role
                ScopeMW->>Handler: next(w, r) - role check passed
            else Missing both scope and role
                ScopeMW-->>Browser: 403 Forbidden - insufficient_permissions
            end
        end

        Handler->>Handler: Load docs for userContext.UserID
        Handler-->>Browser: 200 OK - docs array with count

    else Token inactive or expired
        Zitadel-->>AuthMW: active=false
        AuthMW-->>Browser: 401 Unauthorized - invalid_token

    else No Authorization header
        AuthMW-->>Browser: 401 Unauthorized - missing_token
    end

    LogMW->>LogMW: Log: status_code, duration_ms
```

---

## 10. Future: Event-Driven Architecture

This flowchart shows how Zitadel Actions (webhooks) can feed an event bus to decouple identity events from downstream consumers. When a user is created in Zitadel, the Provisioning Service automatically creates default workspace resources (docs, drive folders, calendar) for that user. Other consumers handle audit trails, notifications, and analytics asynchronously without coupling to the identity provider.

```mermaid
graph LR
    classDef trigger fill:#E67E22,stroke:#A85A0A,color:#fff
    classDef bus fill:#2C3E50,stroke:#1A252F,color:#fff
    classDef consumer fill:#27AE60,stroke:#1A7A42,color:#fff
    classDef resource fill:#2980B9,stroke:#1A5276,color:#fff
    classDef event fill:#8E44AD,stroke:#5E2D7A,color:#fff

    subgraph IdentityEvents["Identity Events - Zitadel Actions"]
        ZA1["user.created"]:::event
        ZA2["user.updated"]:::event
        ZA3["session.created"]:::event
        ZA4["auth.failed"]:::event
        ZA5["role.granted"]:::event
        ZA6["org.created"]:::event
    end

    subgraph EventBus["Event Bus (NATS / Kafka)"]
        Bus["Message Broker\nTopics per event type\nAt-least-once delivery\nDead letter queue"]:::bus
    end

    subgraph Consumers["Event Consumers"]
        AuditSvc["Audit Service\nImmutable event log\nCompliance records"]:::consumer
        NotifSvc["Notification Service\nWelcome email\nSecurity alerts"]:::consumer
        AnalyticsSvc["Analytics Service\nDAU / MAU tracking\nAuth funnel metrics"]:::consumer
        ProvisionSvc["Provisioning Service\nAuto-setup for new users"]:::consumer
    end

    subgraph ProvisionedResources["Resources Created on user.created"]
        DefaultDocs["Default Documents\nGetting Started doc\nPersonal notes doc"]:::resource
        DefaultDrive["Default Drive\nMy Drive folder\nShared with Me folder"]:::resource
        DefaultCal["Default Calendar\nPersonal calendar\nHolidays calendar"]:::resource
    end

    ZA1 & ZA2 & ZA3 & ZA4 & ZA5 & ZA6 -->|"webhook POST"| Bus

    Bus -->|"all events"| AuditSvc
    Bus -->|"user.created, auth.failed"| NotifSvc
    Bus -->|"all events"| AnalyticsSvc
    Bus -->|"user.created, org.created"| ProvisionSvc

    ProvisionSvc -->|"create"| DefaultDocs
    ProvisionSvc -->|"create"| DefaultDrive
    ProvisionSvc -->|"create"| DefaultCal
```

---

## 11. Deployment Architecture (Production)

This diagram shows a production-grade deployment topology with high availability at every layer. The application tier runs multiple replicas of each service behind the API Gateway, Zitadel runs in clustered mode, the data tier uses primary-replica PostgreSQL and a Redis cluster, and all infrastructure is observable via Prometheus and Grafana. The DMZ layer isolates public-facing components from internal services.

```mermaid
graph TB
    classDef lb fill:#2C3E50,stroke:#1A252F,color:#fff
    classDef dmz fill:#C0392B,stroke:#7B241C,color:#fff
    classDef app fill:#27AE60,stroke:#1A7A42,color:#fff
    classDef identity fill:#E67E22,stroke:#A85A0A,color:#fff
    classDef data fill:#8E44AD,stroke:#5E2D7A,color:#fff
    classDef monitor fill:#2980B9,stroke:#1A5276,color:#fff
    classDef storage fill:#16A085,stroke:#0C6B5A,color:#fff

    subgraph Internet["Internet"]
        Users["Users / Clients\nWeb, Mobile, CLI"]
    end

    subgraph DMZ["DMZ Layer"]
        LB["Load Balancer\nnginx / Traefik\nTLS termination\nDDoS protection\nHealth checks"]:::lb
    end

    subgraph AppTier["Application Tier"]
        subgraph GatewayCluster["API Gateway - 2 or more replicas"]
            GW1["Gateway Replica 1"]:::app
            GW2["Gateway Replica 2"]:::app
        end

        subgraph ZitadelCluster["Zitadel - 2 or more replicas"]
            Z1["Zitadel Replica 1"]:::identity
            Z2["Zitadel Replica 2"]:::identity
        end

        subgraph Services["Microservices"]
            Docs1["Docs Svc\nReplica 1"]:::app
            Docs2["Docs Svc\nReplica 2"]:::app
            Drive1["Drive Svc\nReplica 1"]:::app
            Drive2["Drive Svc\nReplica 2"]:::app
            Sheets1["Sheets Svc\nReplica 1"]:::app
            Cal1["Calendar Svc"]:::app
            Admin1["Admin Svc"]:::app
            Notif1["Notification Svc"]:::app
        end
    end

    subgraph DataTier["Data Tier"]
        subgraph PostgresCluster["PostgreSQL HA"]
            PGPrimary[("PostgreSQL\nPrimary\nread-write")]:::data
            PGReplica[("PostgreSQL\nReplica\nread-only")]:::data
        end

        subgraph RedisCluster["Redis Cluster"]
            R1[("Redis Primary")]:::data
            R2[("Redis Replica")]:::data
            R3[("Redis Replica")]:::data
        end

        S3Compat["S3-Compatible\nObject Storage\nMinIO / AWS S3\nFile blobs and exports"]:::storage
    end

    subgraph Monitoring["Monitoring Stack"]
        Prometheus["Prometheus\nMetrics scraping\nAlerting rules"]:::monitor
        Grafana["Grafana\nDashboards\nAlert routing"]:::monitor
        Loki["Loki\nLog aggregation"]:::monitor
        Tempo["Tempo\nDistributed tracing"]:::monitor
    end

    Users -->|"HTTPS :443"| LB

    LB --> GW1 & GW2

    GW1 & GW2 -->|"route"| Docs1 & Docs2
    GW1 & GW2 -->|"route"| Drive1 & Drive2
    GW1 & GW2 -->|"route"| Sheets1 & Cal1 & Admin1 & Notif1
    GW1 & GW2 -->|"validate tokens"| Z1 & Z2

    Z1 & Z2 -->|"read-write"| PGPrimary
    PGPrimary -->|"replication"| PGReplica

    Docs1 & Docs2 & Drive1 & Drive2 & Sheets1 & Cal1 & Admin1 & Notif1 -->|"read-write"| PGPrimary
    Docs1 & Docs2 & Drive1 & Drive2 & Sheets1 -->|"read"| PGReplica

    GW1 & GW2 & Notif1 -->|"cache / sessions"| R1
    R1 --> R2 & R3

    Drive1 & Drive2 & Docs1 & Docs2 & Sheets1 -->|"blob storage"| S3Compat

    GW1 & GW2 & Docs1 & Docs2 & Drive1 & Drive2 & Z1 & Z2 -->|"metrics"| Prometheus
    GW1 & GW2 & Docs1 & Docs2 & Drive1 & Drive2 & Z1 & Z2 -->|"logs"| Loki
    GW1 & GW2 & Docs1 & Docs2 -->|"traces"| Tempo
    Prometheus --> Grafana
    Loki --> Grafana
    Tempo --> Grafana
```

---

## Summary

| Diagram | Purpose | Mermaid Type |
|---|---|---|
| 1. System Overview | Current Docker Compose POC components | `graph TB` |
| 2. OIDC + PKCE Flow | Browser login with code exchange | `sequenceDiagram` |
| 3. Token Introspection | Opaque token validation via Zitadel API | `sequenceDiagram` |
| 4. JWT / JWKS Validation | Local signature verification with key cache | `sequenceDiagram` |
| 5. Client Credentials | M2M service-to-service auth | `sequenceDiagram` |
| 6. RBAC Decision Tree | Multi-strategy authorization middleware | `graph TD` |
| 7. Workspace Architecture | Full microservices + data layer design | `graph TB` |
| 8. Multi-Tenancy Model | Zitadel org hierarchy + cross-org grants | `graph TB` |
| 9. Request Lifecycle | Middleware chain in Workspace POC | `sequenceDiagram` |
| 10. Event-Driven Future | Webhook-driven async provisioning | `graph LR` |
| 11. Production Deployment | HA topology with monitoring | `graph TB` |
