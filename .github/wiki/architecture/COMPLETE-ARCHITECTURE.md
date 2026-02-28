# Complete Architecture Documentation

**Version 0.9.9** | System Design, Patterns, and Technical Decisions

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [C4 Model Diagrams](#c4-model-diagrams)
3. [Component Architecture](#component-architecture)
4. [Data Flow](#data-flow)
5. [Security Architecture](#security-architecture)
6. [Scalability Architecture](#scalability-architecture)
7. [Design Decisions](#design-decisions)
8. [Technology Stack](#technology-stack)
9. [Integration Patterns](#integration-patterns)

---

## Architecture Overview

nself is a **full-stack backend platform** built on modern cloud-native principles with a focus on:
- **Self-hosting** - Complete infrastructure control
- **Multi-tenancy** - Isolated data per tenant with shared infrastructure
- **Scalability** - Horizontal and vertical scaling capabilities
- **Security** - Defense-in-depth with RLS, JWT, SSL, and audit logging
- **Developer Experience** - GraphQL-first API with CLI automation

### Architectural Principles

1. **Separation of Concerns** - Each service has a single responsibility
2. **API-First** - GraphQL as primary interface, REST for legacy compatibility
3. **Database-Centric** - PostgreSQL as source of truth, RLS for isolation
4. **Stateless Services** - Services can be restarted/scaled without data loss
5. **Configuration as Code** - Everything defined in `.env` and generated files
6. **Zero-Trust Security** - Every request authenticated and authorized
7. **Observability Built-In** - Metrics, logs, and traces from day one

---

## C4 Model Diagrams

### Level 1: System Context

```
┌────────────────────────────────────────────────────────────────┐
│                      External Actors                           │
└────────────────────────────────────────────────────────────────┘

    ┌──────────────┐      ┌──────────────┐      ┌──────────────┐
    │  End Users   │      │  Developers  │      │  Admins      │
    │              │      │              │      │              │
    │ Web/Mobile   │      │ CLI/API      │      │ Dashboard    │
    └──────┬───────┘      └──────┬───────┘      └──────┬───────┘
           │                     │                     │
           │                     ▼                     │
           │            ┌─────────────────┐            │
           │            │  GraphQL API    │            │
           └───────────►│  (Hasura)       │◄───────────┘
                        └────────┬────────┘
                                 │
        ┌────────────────────────┼────────────────────────┐
        │                        │                        │
        ▼                        ▼                        ▼
┌───────────────┐      ┌─────────────────┐      ┌────────────────┐
│  nself        │      │   PostgreSQL    │      │  Object Storage│
│  Platform     │◄────►│   Database      │      │   (MinIO)      │
│               │      │                 │      │                │
│ • Auth        │      │ • Data          │      │ • Files        │
│ • Functions   │      │ • RLS           │      │ • Media        │
│ • Monitoring  │      │ • Multi-tenant  │      │ • Backups      │
└───────────────┘      └─────────────────┘      └────────────────┘
        │
        │
        ▼
┌───────────────┐
│  External     │
│  Services     │
│               │
│ • Email       │
│ • Payment     │
│ • Analytics   │
└───────────────┘
```

**External Systems:**
- **End Users** - Access via web/mobile applications
- **Developers** - Interact via CLI, API, and Hasura Console
- **Administrators** - Manage via Admin Dashboard and monitoring tools
- **External Services** - Email providers, payment gateways, analytics

---

### Level 2: Container Diagram

```
┌──────────────────────────────────────────────────────────────────────┐
│                        nself Platform                                │
│                      Docker Compose Deployment                       │
└──────────────────────────────────────────────────────────────────────┘

    Internet
       │
       ▼
┌─────────────────┐
│     Nginx       │  Port 80/443
│  Reverse Proxy  │  • SSL Termination
│                 │  • Load Balancing
│                 │  • Rate Limiting
└────────┬────────┘
         │
         │  Routes to Services
         │
    ┌────┼────────────────┬───────────────┬──────────────┐
    │    │                │               │              │
    ▼    ▼                ▼               ▼              ▼
┌─────────┐  ┌────────────┐  ┌──────────┐  ┌─────────┐  ┌──────────┐
│ Hasura  │  │   Auth     │  │ Functions│  │  Admin  │  │  Custom  │
│ GraphQL │  │  Service   │  │ Runtime  │  │   UI    │  │ Services │
│         │  │            │  │          │  │         │  │ (CS_1-10)│
│ :8080   │  │   :4000    │  │  :3001   │  │  :3000  │  │  :800x   │
└────┬────┘  └─────┬──────┘  └────┬─────┘  └────┬────┘  └────┬─────┘
     │             │              │             │            │
     └─────────────┼──────────────┼─────────────┘            │
                   │              │                          │
                   ▼              ▼                          ▼
            ┌──────────────────────────┐            ┌────────────┐
            │     PostgreSQL           │            │   Redis    │
            │   Port 5432              │            │  :6379     │
            │                          │            │            │
            │ • Application Data       │            │ • Sessions │
            │ • Auth Tables            │            │ • Cache    │
            │ • Multi-tenant Schemas   │            │ • Queues   │
            │ • Row Level Security     │            └────────────┘
            └──────────────────────────┘

     ┌──────────────┐            ┌──────────────────────┐
     │    MinIO     │            │  Monitoring Stack    │
     │  S3 Storage  │            │                      │
     │   :9000      │            │ • Prometheus :9090   │
     │              │            │ • Grafana :3000      │
     │ • User Files │            │ • Loki :3100         │
     │ • Uploads    │            │ • Tempo :3200        │
     │ • Backups    │            │ • Alertmanager :9093 │
     └──────────────┘            └──────────────────────┘
```

**Container Responsibilities:**

1. **Nginx** - Entry point for all traffic
   - SSL/TLS termination
   - Routing to backend services
   - Static file serving
   - Rate limiting and DDoS protection

2. **Hasura GraphQL Engine** - API layer
   - GraphQL API generation from database schema
   - Real-time subscriptions
   - Remote schemas and actions
   - Authorization via permissions and RLS

3. **Auth Service** (nHost Auth) - Authentication
   - User registration and login
   - JWT token generation
   - OAuth provider integration
   - MFA support
   - Session management

4. **Functions Runtime** - Serverless functions
   - Node.js/Deno runtime
   - Event-driven execution
   - Database triggers
   - Scheduled jobs

5. **Admin UI** - Management interface
   - Visual service management
   - Database browser
   - User management
   - Monitoring dashboards

6. **Custom Services** (CS_1 - CS_10) - User-defined
   - Generated from templates
   - Any language/framework
   - Custom business logic
   - Microservices architecture

7. **PostgreSQL** - Primary database
   - Application data storage
   - Auth system tables
   - Multi-tenant data isolation (RLS)
   - Full-text search
   - JSON/JSONB support

8. **Redis** - In-memory data store
   - Session storage
   - Caching layer
   - Rate limiting counters
   - Job queues (with BullMQ)

9. **MinIO** - S3-compatible object storage
   - File uploads
   - Media storage
   - Backup storage
   - CDN source

10. **Monitoring Stack** - Observability
    - Prometheus (metrics)
    - Grafana (visualization)
    - Loki (logs)
    - Tempo (traces)
    - Alertmanager (alerts)

---

### Level 3: Component Diagram (Hasura)

```
┌────────────────────────────────────────────────────────────┐
│              Hasura GraphQL Engine Container               │
└────────────────────────────────────────────────────────────┘

    ┌────────────────┐
    │  GraphQL       │  Port 8080
    │  HTTP Server   │  /v1/graphql
    └────────┬───────┘  /v1/metadata
             │          /healthz
             │
    ┌────────┼────────────────┬───────────────┐
    │        │                │               │
    ▼        ▼                ▼               ▼
┌─────────────┐  ┌──────────────┐  ┌───────────┐  ┌────────────┐
│  Query      │  │ Subscription │  │ Metadata  │  │  Actions   │
│  Engine     │  │  Engine      │  │  Manager  │  │  Handler   │
│             │  │              │  │           │  │            │
│ • Parse     │  │ • WebSocket  │  │ • Schema  │  │ • REST     │
│ • Validate  │  │ • Live       │  │ • Perms   │  │ • Custom   │
│ • Execute   │  │ • Streaming  │  │ • Remote  │  │ • Webhook  │
└──────┬──────┘  └──────┬───────┘  └─────┬─────┘  └─────┬──────┘
       │                │                │              │
       │                │                │              │
       └────────────────┼────────────────┘              │
                        │                               │
                        ▼                               ▼
            ┌───────────────────┐             ┌─────────────────┐
            │  Authorization    │             │  Remote Schema  │
            │  Layer            │             │  Proxy          │
            │                   │             │                 │
            │ • Session Vars    │             │ • Stitching     │
            │ • RLS Context     │             │ • Federation    │
            │ • Permissions     │             └─────────────────┘
            └──────────┬────────┘
                       │
                       ▼
            ┌───────────────────┐
            │  SQL Compiler     │
            │                   │
            │ • Query Builder   │
            │ • Join Optimizer  │
            │ • RLS Injection   │
            └──────────┬────────┘
                       │
                       ▼
            ┌───────────────────┐
            │  PostgreSQL       │
            │  Connection Pool  │
            │                   │
            │ • Pool Manager    │
            │ • Health Check    │
            │ • Reconnect       │
            └───────────────────┘
                       │
                       ▼
                   PostgreSQL
                   Database
```

**Component Interactions:**

1. **Query Engine**
   - Receives GraphQL queries
   - Parses and validates against schema
   - Compiles to SQL
   - Returns JSON response

2. **Subscription Engine**
   - Maintains WebSocket connections
   - Polls database for changes (live queries)
   - Pushes updates to clients
   - Multiplexing for efficiency

3. **Authorization Layer**
   - Extracts session variables from JWT
   - Sets PostgreSQL session context
   - Applies role-based permissions
   - Injects RLS policies

4. **Metadata Manager**
   - Stores schema configuration
   - Manages permissions
   - Handles remote schemas
   - Triggers and event handlers

5. **Actions Handler**
   - Proxies to custom REST endpoints
   - Transforms requests/responses
   - Error handling and retries

---

### Level 4: Code-Level (PostgreSQL RLS)

```sql
-- Multi-Tenant Row Level Security Implementation

-- 1. Enable RLS on table
CREATE TABLE posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    title TEXT NOT NULL,
    content TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

ALTER TABLE posts ENABLE ROW LEVEL SECURITY;

-- 2. Create policies

-- Policy: Users can only see posts in their tenant
CREATE POLICY tenant_isolation_select ON posts
    FOR SELECT
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- Policy: Users can only insert posts in their tenant
CREATE POLICY tenant_isolation_insert ON posts
    FOR INSERT
    WITH CHECK (
        tenant_id = current_setting('app.tenant_id', true)::uuid
        AND user_id = current_setting('hasura.user.id', true)::uuid
    );

-- Policy: Users can only update their own posts
CREATE POLICY user_update_own ON posts
    FOR UPDATE
    USING (
        tenant_id = current_setting('app.tenant_id', true)::uuid
        AND user_id = current_setting('hasura.user.id', true)::uuid
    );

-- Policy: Admin can see all posts in tenant
CREATE POLICY admin_select_all ON posts
    FOR SELECT
    USING (
        tenant_id = current_setting('app.tenant_id', true)::uuid
        AND current_setting('hasura.user.role', true) = 'admin'
    );

-- 3. Session variables set by Hasura from JWT

-- JWT payload:
{
  "sub": "user-uuid",
  "https://hasura.io/jwt/claims": {
    "x-hasura-user-id": "user-uuid",
    "x-hasura-allowed-roles": ["user", "admin"],
    "x-hasura-default-role": "user",
    "x-hasura-tenant-id": "tenant-uuid"
  }
}

-- Hasura sets PostgreSQL session:
SET LOCAL app.tenant_id = 'tenant-uuid';
SET LOCAL hasura.user.id = 'user-uuid';
SET LOCAL hasura.user.role = 'user';

-- 4. Query execution with RLS

-- User queries:
SELECT * FROM posts;

-- PostgreSQL rewrites to:
SELECT * FROM posts
WHERE tenant_id = 'tenant-uuid'  -- RLS policy applied
  AND user_id = 'user-uuid';      -- If user, not admin
```

---

## Component Architecture

### Request Flow (GraphQL Query)

```
1. Client Request
   ↓
2. Nginx (SSL termination, routing)
   ↓
3. Hasura GraphQL Engine
   • Parse GraphQL query
   • Validate against schema
   • Extract JWT from Authorization header
   ↓
4. JWT Validation
   • Verify signature (HMAC-SHA256)
   • Check expiration
   • Extract claims (user ID, role, tenant ID)
   ↓
5. Set Session Variables
   • app.tenant_id
   • hasura.user.id
   • hasura.user.role
   ↓
6. Authorization Check
   • Check table permissions for role
   • Apply column-level permissions
   ↓
7. SQL Compilation
   • GraphQL → SQL
   • Inject RLS policy filters
   • Optimize joins
   ↓
8. Database Query
   • PostgreSQL executes query
   • RLS policies filter rows
   • Return result set
   ↓
9. Response Transformation
   • SQL result → JSON
   • Apply field transformations
   • Nested object resolution
   ↓
10. Client Response
   • JSON over HTTPS
```

**Performance Optimizations:**
- **Query Caching** - Hasura caches compiled SQL
- **Connection Pooling** - Reuse database connections
- **Prepared Statements** - Faster query execution
- **Multiplexing** - Batch subscriptions
- **Compression** - gzip response payloads

---

### Authentication Flow (JWT)

```
1. User Login Request
   POST /v1/auth/login
   { "email": "user@example.com", "password": "..." }
   ↓
2. Auth Service
   • Hash password (bcrypt)
   • Query: SELECT * FROM auth.users WHERE email = ?
   • Compare password hash
   ↓
3. Password Match
   ↓
4. Generate JWT
   • Header: { "alg": "HS256", "typ": "JWT" }
   • Payload: {
       "sub": "user-uuid",
       "iat": 1706745600,
       "exp": 1706749200,
       "https://hasura.io/jwt/claims": {
         "x-hasura-user-id": "user-uuid",
         "x-hasura-allowed-roles": ["user"],
         "x-hasura-default-role": "user",
         "x-hasura-tenant-id": "tenant-uuid"
       }
     }
   • Sign with secret key
   ↓
5. Generate Refresh Token
   • Random 64-byte token
   • Store in database with expiry (30 days)
   ↓
6. Response
   {
     "accessToken": "eyJhbGc...",  // 15 min expiry
     "refreshToken": "abc123...",  // 30 day expiry
     "user": { "id": "...", "email": "..." }
   }
   ↓
7. Client Stores Tokens
   • Access token in memory
   • Refresh token in httpOnly cookie
   ↓
8. Subsequent Requests
   Authorization: Bearer eyJhbGc...
   ↓
9. Token Refresh (when expired)
   POST /v1/auth/refresh
   { "refreshToken": "abc123..." }
   ↓
10. New Access Token Issued
```

**Security Features:**
- **Short-lived access tokens** (15 minutes)
- **Long-lived refresh tokens** (30 days)
- **Token rotation** on refresh
- **Secure storage** (httpOnly cookies for refresh token)
- **Token revocation** support
- **Rate limiting** on auth endpoints

---

## Data Flow

### File Upload Flow

```
1. Client Request
   POST https://api.yourdomain.com/v1/storage/upload
   Content-Type: multipart/form-data
   Authorization: Bearer <jwt>
   ↓
2. Nginx Route
   → Forward to Hasura Actions endpoint
   ↓
3. Hasura Actions Handler
   • Validate JWT
   • Extract user/tenant from claims
   • Proxy to custom upload function
   ↓
4. Upload Function (Node.js)
   • Validate file type and size
   • Generate unique filename
   • Extract tenant_id from session
   ↓
5. MinIO Upload
   const s3 = new AWS.S3({
     endpoint: 'http://minio:9000',
     accessKeyId: process.env.MINIO_ACCESS_KEY,
     secretAccessKey: process.env.MINIO_SECRET_KEY,
   });

   await s3.putObject({
     Bucket: `tenant-${tenant_id}`,
     Key: filename,
     Body: fileBuffer,
     ACL: 'private',
   });
   ↓
6. Database Record
   INSERT INTO files (id, tenant_id, user_id, filename, url, size)
   VALUES (uuid, tenant_id, user_id, filename, url, file_size);
   ↓
7. Response
   {
     "fileId": "file-uuid",
     "url": "https://cdn.yourdomain.com/tenant-uuid/filename.jpg",
     "size": 1024000
   }
```

**Security:**
- **Tenant isolation** - Separate S3 buckets per tenant
- **Access control** - Pre-signed URLs for private files
- **Virus scanning** - ClamAV integration (optional)
- **File type validation** - Whitelist allowed MIME types
- **Size limits** - Per-user and per-tenant quotas

---

### Real-Time Subscription Flow

```
1. Client Subscription
   subscription {
     posts(where: { user_id: { _eq: $userId } }) {
       id
       title
       content
     }
   }
   ↓
2. WebSocket Handshake
   ws://api.yourdomain.com/v1/graphql
   Connection: Upgrade
   ↓
3. Hasura Subscription Manager
   • Parse subscription
   • Validate permissions
   • Set session variables
   ↓
4. Initial Data Fetch
   • Execute query once
   • Return current data to client
   ↓
5. Polling Setup (Live Query)
   • Hasura polls database every 1 second (configurable)
   • Compares result hash with previous
   ↓
6. Data Change Detected
   • New post inserted
   • Hash changed
   ↓
7. Push Update to Client
   {
     "type": "data",
     "id": "subscription-id",
     "payload": {
       "data": {
         "posts": [/* updated data */]
       }
     }
   }
   ↓
8. Client Updates UI
   • React/Vue/Angular component re-renders
   • New post appears instantly
```

**Optimization:**
- **Multiplexing** - Batch identical subscriptions
- **Refetch interval** - Configurable (default: 1s)
- **Cursor-based** - Only fetch changes since last poll
- **Connection management** - Automatic reconnection

---

## Security Architecture

### Defense in Depth

```
Layer 1: Network Security
├── Firewall (ufw/iptables)
├── DDoS Protection (Cloudflare/AWS Shield)
└── Rate Limiting (Nginx)

Layer 2: Transport Security
├── TLS 1.3 (SSL certificates)
├── HSTS (Strict-Transport-Security)
└── Certificate Pinning (mobile apps)

Layer 3: Application Security
├── JWT Authentication
├── CORS Configuration
├── Security Headers (CSP, X-Frame-Options, etc.)
└── Input Validation

Layer 4: API Security
├── GraphQL Query Depth Limiting
├── Query Cost Analysis
├── Rate Limiting per User
└── API Key Management

Layer 5: Database Security
├── Row Level Security (RLS)
├── Role-Based Access Control
├── SQL Injection Prevention (Parameterized Queries)
└── Encrypted Connections (SSL)

Layer 6: Data Security
├── Encryption at Rest
├── Encryption in Transit
├── PII Anonymization
└── Secure Backups

Layer 7: Audit & Monitoring
├── Audit Logging
├── Anomaly Detection
├── Intrusion Detection
└── Security Alerts
```

---

### RLS (Row Level Security) Enforcement

**Multi-Tenant Isolation:**

```sql
-- Every query is rewritten by PostgreSQL

-- User query:
SELECT * FROM posts WHERE title LIKE '%search%';

-- PostgreSQL rewrites to:
SELECT * FROM posts
WHERE title LIKE '%search%'
  AND tenant_id = current_setting('app.tenant_id')::uuid  -- RLS policy
  AND (
    user_id = current_setting('hasura.user.id')::uuid     -- User posts
    OR current_setting('hasura.user.role') = 'admin'      -- Or admin
  );
```

**Benefits:**
- **Zero-trust** - Database enforces isolation, not application
- **SQL injection proof** - Policies can't be bypassed
- **Centralized** - Security rules in one place
- **Performance** - Indexes work with RLS

---

## Scalability Architecture

### Scaling Strategy by Load

**0-10K Users: Single Server**
```
┌─────────────────────────────┐
│   Single Server (16GB RAM)  │
│                             │
│  • All services in Docker   │
│  • PostgreSQL               │
│  • Redis                    │
│  • Hasura                   │
│  • Auth                     │
└─────────────────────────────┘
```

**10K-100K Users: Separated Database**
```
┌─────────────────┐        ┌──────────────────┐
│  App Server     │        │  Database Server │
│                 │        │                  │
│  • Hasura       │───────►│  • PostgreSQL    │
│  • Auth         │        │  • Redis         │
│  • Functions    │        │                  │
│  • Nginx        │        └──────────────────┘
└─────────────────┘
```

**100K+ Users: Horizontal Scaling**
```
       ┌──────────────┐
       │ Load Balancer│
       └──────┬───────┘
              │
    ┌─────────┼─────────┐
    ▼         ▼         ▼
┌────────┐ ┌────────┐ ┌────────┐
│ App 1  │ │ App 2  │ │ App 3  │
└────┬───┘ └────┬───┘ └────┬───┘
     │          │          │
     └──────────┼──────────┘
                ▼
       ┌────────────────┐
       │  PostgreSQL    │
       │  Primary       │
       └────┬───────────┘
            │
       ┌────┴────┐
       ▼         ▼
  ┌─────────┐ ┌─────────┐
  │Replica 1│ │Replica 2│
  └─────────┘ └─────────┘
```

---

## Design Decisions

### Why GraphQL (Hasura) over REST?

**Decision:** Use Hasura GraphQL as primary API layer

**Rationale:**
1. **Automatic API generation** - No manual endpoint coding
2. **Real-time built-in** - WebSocket subscriptions
3. **Type safety** - Schema-driven development
4. **Performance** - Fetch exactly what you need
5. **RLS integration** - Direct PostgreSQL security

**Trade-offs:**
- ✅ Faster development
- ✅ Better developer experience
- ✅ Built-in subscriptions
- ❌ Learning curve for GraphQL
- ❌ Caching more complex than REST

---

### Why PostgreSQL over NoSQL?

**Decision:** PostgreSQL as primary database

**Rationale:**
1. **ACID compliance** - Strong consistency guarantees
2. **Row Level Security** - Built-in multi-tenancy
3. **Rich data types** - JSON, arrays, full-text search
4. **Mature ecosystem** - 30+ years of development
5. **Excellent performance** - Scales to millions of rows

**Trade-offs:**
- ✅ Data integrity
- ✅ Complex queries
- ✅ ACID guarantees
- ❌ Harder horizontal scaling than NoSQL
- ❌ Schema migrations required

---

### Why Docker Compose over Kubernetes (for small/medium)?

**Decision:** Docker Compose for <100K users, Kubernetes for larger

**Rationale:**
1. **Simplicity** - Single YAML file vs many manifests
2. **Local development** - Same as production
3. **Resource efficiency** - No k8s overhead
4. **Easier debugging** - Logs and exec simpler
5. **Cost** - No k8s control plane costs

**Trade-offs:**
- ✅ Simpler operations
- ✅ Lower resource usage
- ✅ Faster iteration
- ❌ Less auto-scaling
- ❌ Manual failover

**When to switch to Kubernetes:**
- > 100K concurrent users
- Multi-region deployment
- Advanced auto-scaling needed
- Service mesh requirements

---

## Technology Stack

### Infrastructure
- **Container Orchestration:** Docker Compose (development/small), Kubernetes (large scale)
- **Reverse Proxy:** Nginx
- **SSL:** Let's Encrypt / Commercial certs
- **Load Balancer:** HAProxy / Cloud LB

### Backend Services
- **GraphQL API:** Hasura GraphQL Engine v2.35+
- **Database:** PostgreSQL 15+
- **Auth:** nHost Auth (fork of Hasura Auth)
- **Cache/Queue:** Redis 7+
- **Object Storage:** MinIO (S3-compatible)
- **Functions:** Node.js 20 / Deno 1.40

### Monitoring
- **Metrics:** Prometheus
- **Visualization:** Grafana
- **Logs:** Loki + Promtail
- **Traces:** Tempo
- **Alerts:** Alertmanager

### Security
- **Authentication:** JWT (HS256/RS256)
- **Authorization:** RLS + Hasura permissions
- **Encryption:** TLS 1.3, AES-256
- **Secrets:** Encrypted environment variables

### Development
- **CLI:** Bash 3.2+ (POSIX-compliant)
- **CI/CD:** GitHub Actions
- **Testing:** Bats (Bash), Jest (JS), pytest (Python)
- **Documentation:** Markdown, Mermaid diagrams

---

## Integration Patterns

### Service-to-Service Communication

**Pattern 1: Database-Mediated**
```
Service A → PostgreSQL ← Service B
(via triggers, LISTEN/NOTIFY)
```

**Pattern 2: Event-Driven**
```
Service A → Redis Pub/Sub → Service B
```

**Pattern 3: Direct HTTP**
```
Service A → HTTP → Service B
(via Hasura Actions or custom endpoints)
```

**Pattern 4: Message Queue**
```
Service A → BullMQ (Redis) → Service B
(for async jobs)
```

---

## Related Documentation

- [Multi-Tenancy Architecture](MULTI-TENANCY.md)
- [Billing Architecture](BILLING-ARCHITECTURE.md)
- [Build Architecture](BUILD_ARCHITECTURE.md)
- [Command Reorganization](COMMAND-REORGANIZATION-PROPOSAL.md)
- [API Documentation](API.md)

---

**Maintainers:**
- Architecture Review: Monthly
- Diagram Updates: On major changes
- Performance Benchmarks: Quarterly
