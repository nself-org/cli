# Custom Services (CS_N) - Complete Environment Variable Reference

Complete guide to configuring custom backend services using the CS_N pattern in nself.

## Table of Contents

- [Quick Start](#quick-start)
- [Service Definition Format](#service-definition-format)
- [Complete Variable Reference](#complete-variable-reference)
- [Configuration Examples](#configuration-examples)
- [Advanced Patterns](#advanced-patterns)
- [Best Practices](#best-practices)

---

## Quick Start

Define custom services using `CS_N` variables where N = 1 to 20:

```bash
# Basic format
CS_1=service_name:template_type:port

# Example: Express.js API on port 8001
CS_1=api:express-ts:8001
```

That's it! The service will be built from a template and added to your stack.

---

## Service Definition Format

### Basic Format

```bash
CS_N=service_name:template_type[:port][:route]
```

**Parameters:**
- `service_name` - Unique identifier (lowercase, alphanumeric, hyphens, underscores)
- `template_type` - Framework/language template (see [Available Templates](#available-templates))
- `port` - Optional: Service port (default: 8000 + N)
- `route` - Optional: External route (omit for internal-only services)

### Routing Behavior

**Internal-Only (No Route)**
```bash
CS_1=worker:bullmq-ts               # Internal service, no external access
CS_2=cache:node-ts                  # Internal cache service
```

**External Access (With Route)**
```bash
CS_3=api:express-ts:3000:api        # Accessible at api.{BASE_DOMAIN}
CS_4=admin:express-ts:3001:admin    # Accessible at admin.{BASE_DOMAIN}
```

**Custom Domains (Auto-Detected)**
```bash
CS_5=webhook:express-ts:3002:webhook.external.com    # Full domain
CS_6=app:fastapi:8000:api.v2                        # Multi-level: api.v2.{BASE_DOMAIN}
```

---

## Complete Variable Reference

### CS_N - Service Definition (REQUIRED)

**Variable:** `CS_N=service_name:template_type:port:route`

**Example:**
```bash
CS_1=api:express-ts:8001:api
```

Defines the basic service configuration. This is the only required variable.

---

### CS_N_PORT - Service Port

**Variable:** `CS_N_PORT=<port_number>`

**Default:** `8000 + N` (e.g., CS_1 defaults to 8001, CS_2 to 8002)

**Example:**
```bash
CS_1=api:express-ts
CS_1_PORT=3000              # Override default port
```

**Use Cases:**
- Match existing service port requirements
- Avoid port conflicts
- Standardize ports across environments

---

### CS_N_ROUTE - External Route

**Variable:** `CS_N_ROUTE=<route_path>`

**Default:** No route (internal-only service)

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_ROUTE=api              # Accessible at api.{BASE_DOMAIN}
```

**Routing Options:**

1. **Subdomain Route**
   ```bash
   CS_1_ROUTE=api              # → api.example.com
   CS_1_ROUTE=admin            # → admin.example.com
   ```

2. **Multi-Level Subdomain**
   ```bash
   CS_1_ROUTE=api.v2           # → api.v2.example.com
   CS_1_ROUTE=internal.admin   # → internal.admin.example.com
   ```

3. **Full Custom Domain**
   ```bash
   CS_1_ROUTE=api.mycompany.com        # Uses full domain as-is
   CS_1_ROUTE=webhook.external.org     # External domain routing
   ```

4. **Internal Only (No Route)**
   ```bash
   # Omit CS_N_ROUTE entirely for internal-only services
   CS_2=worker:bullmq-ts:8002
   # No CS_2_ROUTE = internal only
   ```

---

### CS_N_MEMORY - Memory Limit

**Variable:** `CS_N_MEMORY=<size>`

**Default:** `512M`

**Format:** Number followed by M (megabytes) or G (gigabytes)

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_MEMORY=1G              # 1 gigabyte RAM limit
CS_1_MEMORY=256M            # 256 megabytes RAM limit
```

**Recommended Limits:**
- **Node.js/Bun/Deno:** 256M - 512M (small apps), 512M - 1G (large apps)
- **Python/FastAPI:** 256M - 512M (small apps), 1G - 2G (ML/data apps)
- **Go/Rust:** 128M - 256M (very efficient)
- **Java/JVM:** 512M - 2G (JVM overhead)
- **Background Workers:** 128M - 256M

**Use Cases:**
- Prevent memory leaks from crashing the host
- Optimize resource usage in production
- Ensure fair resource distribution across services

---

### CS_N_CPU - CPU Limit

**Variable:** `CS_N_CPU=<cores>`

**Default:** `0.5` (half a CPU core)

**Format:** Decimal number representing CPU cores

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_CPU=1.0                # 1 full CPU core
CS_1_CPU=0.25               # Quarter CPU core
CS_1_CPU=2.0                # 2 full CPU cores
```

**Recommended Limits:**
- **API Services:** 0.5 - 1.0 (moderate load), 1.0 - 2.0 (high load)
- **Background Workers:** 0.25 - 0.5 (low priority)
- **ML Services:** 1.0 - 4.0 (computation-heavy)
- **Microservices:** 0.25 - 0.5 (lightweight)

**Use Cases:**
- Prevent CPU hogging
- Guarantee minimum performance
- Cost optimization in cloud environments

---

### CS_N_REPLICAS - Instance Count

**Variable:** `CS_N_REPLICAS=<count>`

**Default:** `1`

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_REPLICAS=3             # Run 3 instances for load balancing
```

**Use Cases:**
- **High Availability:** Run multiple instances for redundancy
- **Load Balancing:** Distribute traffic across replicas
- **Zero-Downtime Deploys:** Rolling updates across replicas
- **Horizontal Scaling:** Scale out instead of up

**Important Notes:**
- Replicas > 1 disables `container_name` (Docker Compose requirement)
- All replicas share the same configuration
- Load balancing handled automatically by Docker Swarm or external LB
- Stateless services work best with replicas

**Example with Load Balancer:**
```bash
# High-traffic API with 5 replicas
CS_1=api:express-ts:8001:api
CS_1_REPLICAS=5
CS_1_MEMORY=512M
CS_1_CPU=0.5

# Each replica gets 512M RAM and 0.5 CPU
# Total: 2.5GB RAM, 2.5 CPU cores
```

---

### CS_N_PUBLIC - Public Access Flag

**Variable:** `CS_N_PUBLIC=<true|false>`

**Default:** `auto` (based on whether `CS_N_ROUTE` is set)

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_ROUTE=api
CS_1_PUBLIC=true            # Explicitly enable nginx routing
```

**Use Cases:**
- Force public access for services without routes
- Explicitly disable public access despite having a route
- Control access in multi-environment setups

**Behavior:**
- `true` → Nginx route created, service accessible externally
- `false` → No nginx route, service only accessible internally
- `auto` → Public if `CS_N_ROUTE` is set, private otherwise

---

### CS_N_HEALTHCHECK - Health Check Endpoint

**Variable:** `CS_N_HEALTHCHECK=<path>`

**Default:** `/health`

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_HEALTHCHECK=/api/health        # Custom health endpoint
CS_1_HEALTHCHECK=/status            # Different path
```

**Use Cases:**
- Custom health check paths for existing apps
- Different health check strategies (deep vs shallow)
- Integration with monitoring systems

**Default Health Check Configuration:**
```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:${PORT}${HEALTHCHECK_PATH}"]
  interval: 30s
  timeout: 10s
  retries: 5
  start_period: 60s
```

---

### CS_N_TABLE_PREFIX - Database Table Prefix

**Variable:** `CS_N_TABLE_PREFIX=<prefix>`

**Default:** None (no prefix)

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_TABLE_PREFIX=api_              # Tables: api_users, api_posts, etc.

CS_2=admin:express-ts:8002
CS_2_TABLE_PREFIX=adm_              # Tables: adm_users, adm_logs, etc.
```

**Use Cases:**
- **Multi-Tenant Apps:** Separate tables per service
- **Namespace Isolation:** Prevent table name conflicts
- **Data Organization:** Logical grouping of related tables
- **Migration Safety:** Clear ownership of database objects

**Example Database Schema:**
```
Tables without prefix:
- users
- posts
- comments

With CS_1_TABLE_PREFIX=api_:
- api_users
- api_posts
- api_comments

With CS_2_TABLE_PREFIX=admin_:
- admin_users
- admin_logs
- admin_settings
```

---

### CS_N_REDIS_PREFIX - Redis Key Prefix

**Variable:** `CS_N_REDIS_PREFIX=<prefix>`

**Default:** None (no prefix)

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_REDIS_PREFIX=api:              # Keys: api:session:123, api:cache:user:1

CS_2=worker:bullmq-ts:8002
CS_2_REDIS_PREFIX=worker:           # Keys: worker:job:456, worker:queue:tasks
```

**Use Cases:**
- **Namespace Isolation:** Prevent key collisions between services
- **Cache Management:** Clear service-specific caches
- **Multi-Service Apps:** Shared Redis, isolated data
- **Debugging:** Easy identification of key ownership

**Example Redis Keys:**
```
Without prefix:
- session:abc123
- cache:user:1
- queue:jobs

With CS_1_REDIS_PREFIX=api::
- api:session:abc123
- api:cache:user:1
- api:queue:jobs

With CS_2_REDIS_PREFIX=worker::
- worker:session:xyz789
- worker:cache:job:1
- worker:queue:tasks
```

**Best Practices:**
- Use `:` as separator (Redis convention)
- Keep prefixes short but descriptive
- Match service names for clarity
- Document prefix usage in code

---

### CS_N_ENV - Service-Specific Environment Variables

**Variable:** `CS_N_ENV=KEY1=value1,KEY2=value2,...`

**Default:** None (only default env vars injected)

**Format:** Comma-separated KEY=value pairs

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_ENV=LOG_LEVEL=debug,RATE_LIMIT=100,CACHE_TTL=3600
```

**Multiple Variables:**
```bash
CS_1_ENV=NODE_ENV=production,LOG_LEVEL=info,MAX_CONNECTIONS=50,TIMEOUT=30000
```

**Use Cases:**
- **App-Specific Config:** Settings unique to this service
- **Feature Flags:** Enable/disable features per service
- **Integration Keys:** Third-party API keys
- **Performance Tuning:** Timeouts, limits, cache settings

**Default Environment Variables (Always Available):**

Every custom service automatically receives these environment variables:

```bash
# Environment
ENV=<from global ENV>
NODE_ENV=<from global ENV>
APP_ENV=<from global ENV>
ENVIRONMENT=<from global ENV>

# Project Info
PROJECT_NAME=<from global PROJECT_NAME>
BASE_DOMAIN=<from global BASE_DOMAIN>
DOCKER_NETWORK=<PROJECT_NAME>_network

# Service Info
SERVICE_NAME=<service_name>
SERVICE_PORT=<service_port>
PORT=<service_port>

# PostgreSQL
POSTGRES_HOST=postgres
POSTGRES_PORT=5432
POSTGRES_DB=<from global POSTGRES_DB>
POSTGRES_USER=<from global POSTGRES_USER>
POSTGRES_PASSWORD=<from global POSTGRES_PASSWORD>
DATABASE_URL=postgres://<user>:<pass>@postgres:5432/<db>

# Redis
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=<from global REDIS_PASSWORD>
REDIS_URL=redis://:<password>@redis:6379

# Hasura
HASURA_GRAPHQL_ENDPOINT=http://hasura:8080/v1/graphql
HASURA_ADMIN_SECRET=<from global HASURA_GRAPHQL_ADMIN_SECRET>
```

**Example Using Default + Custom Env:**
```bash
CS_1=api:express-ts:8001
CS_1_ENV=STRIPE_API_KEY=sk_test_PLACEHOLDER,SENDGRID_API_KEY=SG.abc

# Inside service:
# - Has DATABASE_URL (automatic)
# - Has REDIS_URL (automatic)
# - Has STRIPE_API_KEY (custom)
# - Has SENDGRID_API_KEY (custom)
```

---

### CS_N_DEPENDS_ON - Service Dependencies

**Variable:** `CS_N_DEPENDS_ON=<service1>,<service2>,...`

**Default:** `postgres,redis` (automatic dependencies)

**Format:** Comma-separated service names

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_DEPENDS_ON=postgres,redis,minio,meilisearch
```

**Use Cases:**
- **Startup Ordering:** Ensure dependencies start first
- **Health Checks:** Wait for services to be ready
- **Service Mesh:** Define service relationships
- **Dependency Graph:** Visualize service architecture

**Default Dependencies (Always Added):**
- `postgres` - Database must start first
- `redis` - Cache must be available
- `hasura` - If `HASURA_ENABLED=true`

**Custom Dependencies:**
```bash
# API depends on search and storage
CS_1=api:express-ts:8001
CS_1_DEPENDS_ON=postgres,redis,minio,meilisearch

# Worker depends on API service
CS_2=worker:bullmq-ts:8002
CS_2_DEPENDS_ON=postgres,redis,api
```

**Dependency Chain Example:**
```bash
# Database services
CS_1=api:express-ts:8001
CS_1_DEPENDS_ON=postgres,redis

# Worker depends on API
CS_2=worker:bullmq-ts:8002
CS_2_DEPENDS_ON=postgres,redis,api

# Startup order: postgres → redis → api → worker
```

---

### CS_N_VOLUMES - Custom Volume Mounts

**Variable:** `CS_N_VOLUMES=<host_path>:<container_path>[:<options>],...`

**Default:** `./services/<service_name>:/app` (development only)

**Format:** Comma-separated volume mount specifications

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_VOLUMES=./data:/app/data,./config:/app/config:ro

# Multiple volumes:
# - ./data → /app/data (read-write)
# - ./config → /app/config (read-only)
```

**Volume Options:**
- `:ro` - Read-only mount
- `:rw` - Read-write mount (default)
- `:z` - SELinux shared content label
- `:Z` - SELinux private content label

**Use Cases:**
- **Data Persistence:** Mount data directories
- **Configuration:** External config files
- **Logs:** Persistent log storage
- **Uploads:** User-uploaded files
- **Cache:** Shared cache directories

**Example with Named Volumes:**
```bash
# Define named volume in docker-compose.yml
CS_1=api:express-ts:8001
CS_1_VOLUMES=api_data:/app/data,api_logs:/app/logs

# Persist data across container recreations
```

**Environment-Specific Behavior:**

**Development (ENV=dev):**
- Automatic source code mount: `./services/<service_name>:/app`
- Enables hot-reload and live editing
- Volume exclusions for node_modules, dist, etc.

**Production (ENV=prod, staging):**
- NO automatic source mount
- Only explicitly defined volumes
- Code baked into image during build

**Example:**
```bash
# Development: Source code is mounted, hot-reload works
ENV=dev
CS_1=api:express-ts:8001
# Automatic: ./services/api:/app

# Production: Source code in image, no mount
ENV=prod
CS_1=api:express-ts:8001
# No automatic mount, only custom CS_1_VOLUMES used
```

---

### CS_N_NETWORKS - Network Configuration

**Variable:** `CS_N_NETWORKS=<network1>,<network2>,...`

**Default:** `${PROJECT_NAME}_network` (project default network)

**Format:** Comma-separated network names

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_NETWORKS=frontend_network,backend_network

# Service connects to multiple networks
```

**Use Cases:**
- **Network Segmentation:** Isolate services by trust level
- **Multi-Tier Architecture:** Frontend/backend/data tiers
- **Service Mesh:** Complex microservice networking
- **External Integration:** Connect to external networks

**Network Isolation Example:**
```bash
# Public-facing API
CS_1=api:express-ts:8001:api
CS_1_NETWORKS=public_network,backend_network

# Internal admin service
CS_2=admin:express-ts:8002
CS_2_NETWORKS=backend_network

# Database (backend only)
# postgres is on backend_network only

# Result:
# - api can talk to postgres (backend_network)
# - api can receive external traffic (public_network)
# - admin can talk to postgres (backend_network)
# - admin CANNOT receive external traffic
```

**Advanced Network Config:**
```bash
# Define custom networks in docker-compose.yml
networks:
  frontend:
    driver: bridge
  backend:
    driver: bridge
    internal: true  # No external access

# Use in services
CS_1=web:express-ts:3000:app
CS_1_NETWORKS=frontend

CS_2=api:express-ts:8001:api
CS_2_NETWORKS=frontend,backend

CS_3=worker:bullmq-ts:8002
CS_3_NETWORKS=backend
```

---

### CS_N_PORTS - Additional Port Mappings

**Variable:** `CS_N_PORTS=<host_port>:<container_port>,...`

**Default:** `${CS_N_PORT}:${CS_N_PORT}` (main service port)

**Format:** Comma-separated port mappings

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_PORTS=8001:8001,9229:9229,3000:3000

# Exposes:
# - 8001 (main app)
# - 9229 (Node.js debugger)
# - 3000 (additional service)
```

**Use Cases:**
- **Debugging:** Expose debugger ports
- **Metrics:** Prometheus metrics endpoint
- **Admin Interfaces:** Internal admin panels
- **gRPC + REST:** Multiple protocols on different ports
- **WebSockets:** Separate WebSocket port

**Common Port Patterns:**

**Node.js Debugging:**
```bash
CS_1=api:express-ts:8001
CS_1_PORTS=8001:8001,9229:9229         # Node debugger
```

**Prometheus Metrics:**
```bash
CS_1=api:express-ts:8001
CS_1_PORTS=8001:8001,9090:9090         # App + metrics
```

**gRPC + HTTP:**
```bash
CS_1=service:grpc:8001
CS_1_PORTS=8001:8001,50051:50051       # HTTP + gRPC
```

**Multiple Services in One Container:**
```bash
CS_1=app:express-ts:8001
CS_1_PORTS=8001:8001,8002:8002,8003:8003
# Main API (8001), Admin (8002), Metrics (8003)
```

**Important Notes:**
- Host ports must be unique across all services
- Container ports can overlap if on different networks
- Use environment-specific ports (dev vs prod)

---

### CS_N_RESTART_POLICY - Restart Behavior

**Variable:** `CS_N_RESTART_POLICY=<policy>`

**Default:** `unless-stopped`

**Options:**
- `no` - Never restart
- `always` - Always restart
- `on-failure` - Restart on non-zero exit
- `unless-stopped` - Restart unless manually stopped (default)

**Example:**
```bash
CS_1=api:express-ts:8001
CS_1_RESTART_POLICY=always              # Always restart, even after reboot

CS_2=worker:bullmq-ts:8002
CS_2_RESTART_POLICY=on-failure          # Only restart on crashes
```

**Use Cases:**
- **Production Services:** `always` for critical services
- **Development:** `unless-stopped` for manual control
- **One-Off Tasks:** `no` for migration scripts
- **Fault Tolerance:** `on-failure` for self-healing

**Restart Policy Details:**

**`unless-stopped` (default, recommended):**
- Restarts on crash
- Restarts after Docker daemon restart
- Does NOT restart if manually stopped
- Best for most services

**`always`:**
- Restarts on crash
- Restarts after Docker daemon restart
- Restarts even if manually stopped
- Use for absolutely critical services

**`on-failure`:**
- Only restarts on non-zero exit code
- Does not restart on manual stop
- Does not restart on clean shutdown
- Use for fault-tolerant services

**`no`:**
- Never restarts automatically
- Must manually start
- Use for one-off scripts/migrations

**Environment-Specific Policies:**
```bash
# Development: Manual control
ENV=dev
CS_1_RESTART_POLICY=unless-stopped

# Production: Always running
ENV=prod
CS_1_RESTART_POLICY=always
```

---

### CS_N_RATE_LIMIT - Request Rate Limiting

**Variable:** `CS_N_RATE_LIMIT=<requests_per_minute>`

**Default:** None (no rate limiting)

**Example:**
```bash
CS_1=api:express-ts:8001:api
CS_1_RATE_LIMIT=100                     # 100 requests per minute per IP

CS_2=public-api:fastapi:8002:public
CS_2_RATE_LIMIT=10                      # 10 requests per minute per IP (strict)
```

**Use Cases:**
- **API Protection:** Prevent abuse and DoS attacks
- **Fair Usage:** Ensure equitable resource access
- **Cost Control:** Limit expensive operations
- **SLA Enforcement:** Tier-based rate limits

**Rate Limiting Strategies:**

**Public API (Strict):**
```bash
CS_1=public-api:fastapi:8001:api
CS_1_RATE_LIMIT=60                      # 1 request/second average
```

**Internal API (Lenient):**
```bash
CS_2=internal-api:express-ts:8002:internal
CS_2_RATE_LIMIT=1000                    # Higher limit for internal use
```

**Admin API (No Limit):**
```bash
CS_3=admin-api:express-ts:8003:admin
# No CS_3_RATE_LIMIT = unlimited
```

**Implementation:**
- Applied at nginx level (before request reaches service)
- Per-IP address limiting
- Returns HTTP 429 (Too Many Requests) when exceeded
- Configurable burst allowance

---

### CS_N_WEBSOCKET - WebSocket Support

**Variable:** `CS_N_WEBSOCKET=true|false`

**Default:** `false`

**Example:**
```bash
CS_1=relay:socketio-ts:8001:relay
CS_1_WEBSOCKET=true                     # Enable WebSocket proxy support
```

**What it does:**
When enabled, the generated nginx config for this service includes WebSocket upgrade headers and a long-lived read timeout (86400s) for persistent connections:

```nginx
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection 'upgrade';
proxy_cache_bypass $http_upgrade;
proxy_read_timeout 86400;
```

Without this flag, the service uses standard HTTP proxying with a 60s read timeout.

**Use Cases:**
- **Real-time applications:** Chat, notifications, live updates
- **WebSocket relay:** Session forwarding, remote access
- **Live dashboards:** Streaming data visualization
- **Collaborative editing:** Multi-user real-time editing

**Example Configurations:**

**Socket.IO Real-Time Service:**
```bash
CS_1=realtime:socketio-ts:8001:ws
CS_1_WEBSOCKET=true
CS_1_MEMORY=512M
CS_1_REPLICAS=2
CS_1_REDIS_PREFIX=ws:
CS_1_ENV=CORS_ORIGIN=https://app.example.com
```

**REST API (no WebSocket needed):**
```bash
CS_2=api:express-ts:8002:api
# CS_2_WEBSOCKET not set — standard HTTP proxy (default)
```

---

### CS_N_DEV_DOMAIN - Development Domain Override

**Variable:** `CS_N_DEV_DOMAIN=<domain>`

**Default:** `${CS_N_ROUTE}.${BASE_DOMAIN}`

**Example:**
```bash
BASE_DOMAIN=example.com

CS_1=api:express-ts:8001:api
CS_1_DEV_DOMAIN=api.local.nself.org     # Dev uses local domain
# Production uses: api.example.com
```

**Use Cases:**
- **Local Development:** Use `.local.nself.org` for automatic SSL
- **Testing:** Separate test domains
- **Multi-Environment:** Different domains per environment
- **Debugging:** Easy environment identification

**Environment-Specific Domains:**
```bash
# .env.dev
ENV=dev
BASE_DOMAIN=example.com
CS_1=api:express-ts:8001:api
CS_1_DEV_DOMAIN=api.local.nself.org
# Actual domain: api.local.nself.org (automatic SSL)

# .env.staging
ENV=staging
BASE_DOMAIN=staging.example.com
CS_1=api:express-ts:8001:api
# Actual domain: api.staging.example.com

# .env.prod
ENV=prod
BASE_DOMAIN=example.com
CS_1=api:express-ts:8001:api
# Actual domain: api.example.com
```

---

### CS_N_PROD_DOMAIN - Production Domain Override

**Variable:** `CS_N_PROD_DOMAIN=<domain>`

**Default:** `${CS_N_ROUTE}.${BASE_DOMAIN}`

**Example:**
```bash
BASE_DOMAIN=example.com

CS_1=api:express-ts:8001:api
CS_1_PROD_DOMAIN=api.mycompany.com      # Production uses different domain
# Development uses: api.example.com
```

**Use Cases:**
- **Domain Migration:** Maintain old domain during transition
- **White-Label Apps:** Different production domains
- **Multi-Region:** Region-specific domains
- **Branding:** Different branding per environment

**Multi-Environment Domain Strategy:**
```bash
BASE_DOMAIN=example.com

CS_1=api:express-ts:8001:api
CS_1_DEV_DOMAIN=api.local.nself.org          # Development
CS_1_STAGING_DOMAIN=api.staging.example.com  # Staging (via .env.staging)
CS_1_PROD_DOMAIN=api.production.com          # Production (different domain)

# Environment-based domain resolution:
# - ENV=dev → api.local.nself.org
# - ENV=staging → api.staging.example.com
# - ENV=prod → api.production.com
```

---

## Available Templates

nself includes 40+ production-ready templates:

### JavaScript/TypeScript

| Template | Description | Use Case |
|----------|-------------|----------|
| `express-js`, `express-ts` | Express.js web framework | REST APIs, web apps |
| `fastify-js`, `fastify-ts` | High-performance framework | Fast APIs, microservices |
| `hono-js`, `hono-ts` | Ultrafast web framework | Edge computing, serverless |
| `nest-js`, `nest-ts` | Enterprise framework | Large apps, enterprise |
| `socketio-js`, `socketio-ts` | WebSocket server | Real-time apps, chat |
| `temporal-js`, `temporal-ts` | Workflow orchestration | Complex workflows |
| `bullmq-js`, `bullmq-ts` | Job queue workers | Background jobs |
| `trpc` | Type-safe APIs | Full-stack TypeScript |
| `bun` | Bun runtime | Fast builds, modern JS |
| `deno` | Deno runtime | Secure, modern runtime |

### Python

| Template | Description | Use Case |
|----------|-------------|----------|
| `fastapi` | Modern async framework | APIs, microservices |
| `flask` | Lightweight WSGI | Small apps, prototypes |
| `django-rest` | Django REST framework | Full-featured APIs |
| `celery` | Distributed task queue | Background jobs |
| `ray` | Distributed computing | ML, data processing |
| `agent-llm` | LLM agent service | AI agents |
| `agent-data` | Data processing | ETL, data pipelines |
| `agent-vision` | Computer vision | Image processing |

### Go

| Template | Description | Use Case |
|----------|-------------|----------|
| `gin` | HTTP web framework | Fast APIs |
| `echo` | High-performance framework | Microservices |
| `fiber` | Express-inspired | REST APIs |
| `grpc` | gRPC service | RPC services |

### Other Languages

| Template | Description | Language |
|----------|-------------|----------|
| `rails` | Ruby on Rails | Ruby |
| `actix-web` | Web framework | Rust |
| `spring-boot` | Spring Boot | Java |
| `aspnet` | ASP.NET Core | C# |
| `laravel` | Laravel framework | PHP |
| `phoenix` | Phoenix framework | Elixir |
| `vapor` | Server-side Swift | Swift |
| `ktor` | Web framework | Kotlin |

---

## Configuration Examples

### Example 1: Simple REST API

```bash
# Minimal configuration
CS_1=api:express-ts:8001:api

# Results in:
# - Service name: api
# - Template: Express TypeScript
# - Port: 8001
# - Route: api.example.com
# - Memory: 512M (default)
# - CPU: 0.5 cores (default)
# - Replicas: 1 (default)
```

### Example 2: High-Performance API with Scaling

```bash
# Production API with replicas
CS_1=api:fastapi:8001:api
CS_1_MEMORY=1G
CS_1_CPU=1.0
CS_1_REPLICAS=3
CS_1_RATE_LIMIT=100
CS_1_TABLE_PREFIX=api_
CS_1_REDIS_PREFIX=api:

# Results in:
# - 3 replicas for load balancing
# - 1GB RAM per replica
# - 1 CPU core per replica
# - 100 requests/min rate limit
# - Isolated database tables (api_*)
# - Isolated Redis keys (api:*)
```

### Example 3: Internal Background Worker

```bash
# BullMQ worker (internal only)
CS_2=worker:bullmq-ts:8002
CS_2_MEMORY=256M
CS_2_CPU=0.25
CS_2_REDIS_PREFIX=worker:
CS_2_ENV=QUEUE_NAME=jobs,CONCURRENCY=5

# Results in:
# - No external route (internal only)
# - 256MB RAM (efficient)
# - 0.25 CPU cores (low priority)
# - Custom Redis prefix
# - Queue configuration via env vars
```

### Example 4: Multi-Service Microservices Stack

```bash
# API Gateway
CS_1=gateway:express-ts:8001:api
CS_1_MEMORY=512M
CS_1_CPU=0.5
CS_1_REPLICAS=2
CS_1_RATE_LIMIT=200
CS_1_ENV=LOG_LEVEL=info

# Auth Service
CS_2=auth-service:fastapi:8002:auth
CS_2_MEMORY=256M
CS_2_CPU=0.25
CS_2_TABLE_PREFIX=auth_
CS_2_REDIS_PREFIX=auth:
CS_2_ENV=JWT_EXPIRY=3600

# User Service
CS_3=user-service:express-ts:8003
CS_3_MEMORY=512M
CS_3_CPU=0.5
CS_3_TABLE_PREFIX=user_
CS_3_DEPENDS_ON=postgres,redis,auth-service

# Payment Service
CS_4=payment-service:express-ts:8004
CS_4_MEMORY=1G
CS_4_CPU=1.0
CS_4_TABLE_PREFIX=pay_
CS_4_ENV=STRIPE_API_KEY=sk_test_PLACEHOLDER,PCI_COMPLIANT=true
CS_4_DEPENDS_ON=postgres,redis,user-service

# Background Workers
CS_5=email-worker:bullmq-ts:8005
CS_5_MEMORY=256M
CS_5_CPU=0.25
CS_5_REDIS_PREFIX=email:
CS_5_ENV=SENDGRID_API_KEY=SG.abc

CS_6=notification-worker:bullmq-ts:8006
CS_6_MEMORY=256M
CS_6_CPU=0.25
CS_6_REDIS_PREFIX=notify:
CS_6_ENV=FCM_KEY=AIza123
```

### Example 5: ML Pipeline with Data Processing

```bash
# ML Training Service
CS_1=ml-trainer:fastapi:8001:ml
CS_1_MEMORY=4G
CS_1_CPU=2.0
CS_1_VOLUMES=./models:/app/models,./datasets:/app/data:ro
CS_1_ENV=CUDA_VISIBLE_DEVICES=0,MODEL_PATH=/app/models
CS_1_DEPENDS_ON=postgres,redis,minio,mlflow

# Data Preprocessing
CS_2=data-processor:agent-data:8002
CS_2_MEMORY=2G
CS_2_CPU=1.5
CS_2_VOLUMES=./datasets:/app/data
CS_2_ENV=BATCH_SIZE=1000,PARALLEL_JOBS=4

# Inference API
CS_3=ml-api:fastapi:8003:inference
CS_3_MEMORY=2G
CS_3_CPU=1.0
CS_3_REPLICAS=3
CS_3_RATE_LIMIT=50
CS_3_VOLUMES=./models:/app/models:ro
CS_3_ENV=MODEL_PATH=/app/models/production.pkl
CS_3_DEPENDS_ON=ml-trainer
```

### Example 6: Real-Time Application

```bash
# WebSocket Server
CS_1=websocket:socketio-ts:8001:ws
CS_1_MEMORY=512M
CS_1_CPU=0.5
CS_1_REPLICAS=2
CS_1_REDIS_PREFIX=ws:
CS_1_ENV=CORS_ORIGIN=https://app.example.com,MAX_CONNECTIONS=1000

# REST API
CS_2=api:express-ts:8002:api
CS_2_MEMORY=512M
CS_2_CPU=0.5
CS_2_REPLICAS=2
CS_2_TABLE_PREFIX=api_
CS_2_ENV=WS_URL=ws://websocket:8001

# Message Queue Worker
CS_3=queue-worker:bullmq-ts:8003
CS_3_MEMORY=256M
CS_3_CPU=0.25
CS_3_REDIS_PREFIX=queue:
CS_3_ENV=EMIT_TO_WS=true,WS_URL=ws://websocket:8001
CS_3_DEPENDS_ON=postgres,redis,websocket
```

### Example 7: Multi-Region Setup

```bash
# .env.dev (Development)
ENV=dev
BASE_DOMAIN=local.nself.org
CS_1=api:express-ts:8001:api
CS_1_DEV_DOMAIN=api.local.nself.org

# .env.staging (Staging)
ENV=staging
BASE_DOMAIN=staging.example.com
CS_1=api:express-ts:8001:api
# Uses: api.staging.example.com

# .env.prod (US Production)
ENV=prod
BASE_DOMAIN=us.example.com
CS_1=api:express-ts:8001:api
CS_1_PROD_DOMAIN=api.example.com
CS_1_REPLICAS=5
CS_1_MEMORY=1G

# .env.prod.eu (EU Production)
ENV=prod
BASE_DOMAIN=eu.example.com
CS_1=api:express-ts:8001:api
CS_1_PROD_DOMAIN=api.eu.example.com
CS_1_REPLICAS=3
CS_1_MEMORY=1G
```

---

## Advanced Patterns

### Pattern 1: Service-to-Service Communication

Services communicate using internal Docker DNS:

```bash
# Service 1: API
CS_1=api:express-ts:8001

# Service 2: Worker (calls API internally)
CS_2=worker:bullmq-ts:8002
CS_2_ENV=API_URL=http://api:8001

# Inside worker code:
# fetch('http://api:8001/users')  ← Uses Docker DNS
```

See [Service-to-Service Communication Guide](../guides/SERVICE-TO-SERVICE-COMMUNICATION.md) for details.

### Pattern 2: Database-Per-Service

```bash
# User Service
CS_1=user-service:express-ts:8001
CS_1_TABLE_PREFIX=user_
CS_1_ENV=SCHEMA=user_schema

# Order Service
CS_2=order-service:express-ts:8002
CS_2_TABLE_PREFIX=order_
CS_2_ENV=SCHEMA=order_schema

# Each service has isolated tables
```

### Pattern 3: Event-Driven Architecture

```bash
# Event Publisher
CS_1=api:express-ts:8001:api
CS_1_REDIS_PREFIX=events:

# Event Consumer 1
CS_2=email-worker:bullmq-ts:8002
CS_2_REDIS_PREFIX=events:
CS_2_ENV=SUBSCRIBE_TO=user.created,user.updated

# Event Consumer 2
CS_3=analytics-worker:bullmq-ts:8003
CS_3_REDIS_PREFIX=events:
CS_3_ENV=SUBSCRIBE_TO=user.created,order.placed
```

### Pattern 4: API Gateway + Microservices

```bash
# Gateway (public-facing)
CS_1=gateway:express-ts:8001:api
CS_1_RATE_LIMIT=100
CS_1_ENV=ROUTES=user-service,product-service,order-service

# User Service (internal)
CS_2=user-service:express-ts:8002
CS_2_TABLE_PREFIX=user_

# Product Service (internal)
CS_3=product-service:express-ts:8003
CS_3_TABLE_PREFIX=product_

# Order Service (internal)
CS_4=order-service:express-ts:8004
CS_4_TABLE_PREFIX=order_

# Gateway routes requests to internal services
```

### Pattern 5: Blue-Green Deployment

```bash
# Blue (current production)
CS_1=api-blue:express-ts:8001:api
CS_1_REPLICAS=3

# Green (new version, testing)
CS_2=api-green:express-ts:8002:api-preview
CS_2_REPLICAS=1
CS_2_ENV=VERSION=2.0.0

# Switch traffic by changing CS_1_ROUTE after validation
```

---

## Best Practices

### Security

1. **Never commit secrets to CS_N_ENV**
   ```bash
   # ❌ BAD
   CS_1_ENV=API_KEY=secret123

   # ✅ GOOD - Use .env.secrets
   API_KEY=secret123  # In .env.secrets (gitignored)
   CS_1_ENV=API_KEY=${API_KEY}
   ```

2. **Use internal-only services when possible**
   ```bash
   # Workers, processors, internal services → no route
   CS_2=worker:bullmq-ts:8002  # No CS_2_ROUTE = internal only
   ```

3. **Apply rate limiting to public APIs**
   ```bash
   CS_1=api:express-ts:8001:api
   CS_1_RATE_LIMIT=100
   ```

### Performance

1. **Right-size resources**
   ```bash
   # Don't over-allocate
   CS_1=api:express-ts:8001
   CS_1_MEMORY=512M      # Not 4G for a simple API
   CS_1_CPU=0.5          # Not 4.0 for light load
   ```

2. **Use replicas for high-traffic services**
   ```bash
   CS_1=api:express-ts:8001:api
   CS_1_REPLICAS=3       # Load balance across 3 instances
   ```

3. **Optimize dependency chains**
   ```bash
   # Only depend on what you actually use
   CS_1_DEPENDS_ON=postgres,redis  # Not postgres,redis,minio,mlflow,...
   ```

### Organization

1. **Use consistent naming conventions**
   ```bash
   CS_1=user-service:express-ts:8001
   CS_2=user-worker:bullmq-ts:8002
   CS_3=user-processor:fastapi:8003
   # All start with "user-" for easy grouping
   ```

2. **Group related services numerically**
   ```bash
   # User services: CS_1-3
   CS_1=user-api:express-ts:8001
   CS_2=user-worker:bullmq-ts:8002
   CS_3=user-processor:fastapi:8003

   # Order services: CS_4-6
   CS_4=order-api:express-ts:8004
   CS_5=order-worker:bullmq-ts:8005
   CS_6=order-processor:fastapi:8006
   ```

3. **Document service purposes**
   ```bash
   # .env comments
   # User Management Stack
   CS_1=user-api:express-ts:8001          # User CRUD operations
   CS_2=user-worker:bullmq-ts:8002        # Async user tasks
   CS_3=user-sync:fastapi:8003            # Third-party sync
   ```

### Development

1. **Use environment-specific configs**
   ```bash
   # .env.dev
   CS_1_MEMORY=256M
   CS_1_REPLICAS=1

   # .env.prod
   CS_1_MEMORY=1G
   CS_1_REPLICAS=5
   ```

2. **Enable debugging in development**
   ```bash
   ENV=dev
   CS_1=api:express-ts:8001
   CS_1_PORTS=8001:8001,9229:9229  # Add debugger port
   CS_1_ENV=DEBUG=*,LOG_LEVEL=debug
   ```

3. **Use health checks for reliability**
   ```bash
   CS_1=api:express-ts:8001
   CS_1_HEALTHCHECK=/health  # Must implement /health endpoint
   ```

### Monitoring

1. **Expose metrics endpoints**
   ```bash
   CS_1=api:express-ts:8001:api
   CS_1_PORTS=8001:8001,9090:9090  # App + Prometheus metrics
   ```

2. **Use consistent logging**
   ```bash
   CS_1_ENV=LOG_LEVEL=info,LOG_FORMAT=json
   ```

3. **Set up health checks**
   ```bash
   CS_1_HEALTHCHECK=/health
   # Implement: GET /health → 200 OK
   ```

---

## Environment-Specific Configuration

### Development (.env.dev)

```bash
ENV=dev

# Lenient resource limits
CS_1=api:express-ts:8001:api
CS_1_MEMORY=256M
CS_1_CPU=0.25
CS_1_REPLICAS=1

# Debug mode
CS_1_ENV=DEBUG=*,LOG_LEVEL=debug

# Development domain
CS_1_DEV_DOMAIN=api.local.nself.org
```

### Staging (.env.staging)

```bash
ENV=staging

# Production-like resources
CS_1=api:express-ts:8001:api
CS_1_MEMORY=512M
CS_1_CPU=0.5
CS_1_REPLICAS=2

# Standard logging
CS_1_ENV=LOG_LEVEL=info

# Staging domain
BASE_DOMAIN=staging.example.com
# Uses: api.staging.example.com
```

### Production (.env.prod)

```bash
ENV=prod

# Full resources
CS_1=api:express-ts:8001:api
CS_1_MEMORY=1G
CS_1_CPU=1.0
CS_1_REPLICAS=5

# Production settings
CS_1_ENV=LOG_LEVEL=warn
CS_1_RATE_LIMIT=100

# Production domain
CS_1_PROD_DOMAIN=api.example.com
```

---

## Troubleshooting

### Service Not Starting

**Check logs:**
```bash
docker logs <project_name>_<service_name>
```

**Common issues:**
- Port conflict: Change `CS_N_PORT`
- Memory limit too low: Increase `CS_N_MEMORY`
- Missing dependencies: Check `CS_N_DEPENDS_ON`

### Service Not Accessible

**Check route configuration:**
```bash
nself urls  # List all routes
```

**Common issues:**
- Missing route: Set `CS_N_ROUTE`
- Wrong domain: Check `BASE_DOMAIN` and `CS_N_DEV_DOMAIN`
- Internal-only service: Add route or set `CS_N_PUBLIC=true`

### High Memory Usage

**Check resource allocation:**
```bash
docker stats  # Real-time resource usage
```

**Solutions:**
- Lower `CS_N_MEMORY`
- Increase `CS_N_REPLICAS` instead of memory
- Optimize application code

### Service Communication Failing

**Check DNS resolution:**
```bash
docker exec <container> ping <service_name>
```

**Common issues:**
- Wrong service name in URL
- Services on different networks
- Service not started yet (add to `CS_N_DEPENDS_ON`)

---

## Related Documentation

- [Service-to-Service Communication](../guides/SERVICE-TO-SERVICE-COMMUNICATION.md)
- [Environment Variables Reference](./ENVIRONMENT-VARIABLES.md)
- [Deployment Architecture](../guides/DEPLOYMENT-ARCHITECTURE.md)
- [Multi-App Setup](../guides/MULTI_APP_SETUP.md)

---

**Last Updated:** January 30, 2026
**nself Version:** 0.4.8+
