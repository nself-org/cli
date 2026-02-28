# Configuration Documentation

Complete reference for configuring ɳSelf projects.

---

## Quick Navigation

### Environment Variables

| Document | Description | Audience |
|----------|-------------|----------|
| [ENVIRONMENT-VARIABLES.md](ENVIRONMENT-VARIABLES.md) | Complete environment variable reference | All users |
| [ENV-COMPLETE-REFERENCE.md](ENV-COMPLETE-REFERENCE.md) | Detailed .env file documentation | Advanced users |
| [CUSTOM-SERVICES-ENV-VARS.md](CUSTOM-SERVICES-ENV-VARS.md) | Custom service (CS_N) configuration | Backend developers |

### Command Options

| Document | Description |
|----------|-------------|
| [START-COMMAND-OPTIONS.md](START-COMMAND-OPTIONS.md) | `nself start` configuration and options |

### Service Configuration

| Document | Description |
|----------|-------------|
| [Admin-UI.md](Admin-UI.md) | ɳAdmin UI configuration |
| [SSL.md](SSL.md) | SSL certificate configuration |

---

## Getting Started

### New Users

1. Start with [ENVIRONMENT-VARIABLES.md](ENVIRONMENT-VARIABLES.md)
   - Core concepts
   - Required variables
   - Common configurations

2. Then review [CUSTOM-SERVICES-ENV-VARS.md](CUSTOM-SERVICES-ENV-VARS.md)
   - Adding custom services
   - Service templates
   - Configuration examples

3. Finally check [START-COMMAND-OPTIONS.md](START-COMMAND-OPTIONS.md)
   - Health check configuration
   - Start modes
   - Troubleshooting

### Advanced Users

Jump directly to:
- [ENV-COMPLETE-REFERENCE.md](ENV-COMPLETE-REFERENCE.md) - Every variable documented
- [CUSTOM-SERVICES-ENV-VARS.md](CUSTOM-SERVICES-ENV-VARS.md) - Deep dive into CS_N

---

## Configuration Overview

### File Structure

```
project/
├── .env                    # Local overrides (highest priority, gitignored)
├── .env.dev                # Development (shared, committed)
├── .env.staging            # Staging (on staging server)
├── .env.prod               # Production (on prod server)
└── .secrets                # Production secrets (generated on server, SSH-synced)
```

### Load Order (Later Overrides Earlier)

```
.env.dev → .env.staging → .env.prod → .secrets → .env
```

### Core Configuration Sections

1. **Project Settings**
   - `PROJECT_NAME` - Your project identifier
   - `BASE_DOMAIN` - Your domain
   - `ENV` - Environment (dev, staging, prod)

2. **Required Services (Always Enabled)**
   - PostgreSQL configuration
   - Hasura GraphQL configuration
   - Auth service configuration
   - Nginx configuration

3. **Optional Services (Enable as Needed)**
   - `REDIS_ENABLED=true`
   - `MINIO_ENABLED=true`
   - `FUNCTIONS_ENABLED=true`
   - `MLFLOW_ENABLED=true`
   - `MAILPIT_ENABLED=true`
   - `MEILISEARCH_ENABLED=true`
   - `NSELF_ADMIN_ENABLED=true`

4. **Monitoring Bundle**
   - `MONITORING_ENABLED=true` (enables all 10 services)
   - Individual service toggles available

5. **Custom Services (CS_N)**
   - Add your own backend services
   - 40+ templates available
   - Full configuration control

6. **Frontend Applications**
   - External app routing
   - Multi-app support

---

## Common Configuration Patterns

### Minimal Development Setup

```bash
# .env.dev
PROJECT_NAME=myapp
BASE_DOMAIN=local.nself.org
ENV=dev

POSTGRES_DB=myapp_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=dev-password

HASURA_GRAPHQL_ADMIN_SECRET=dev-secret
HASURA_JWT_KEY=dev-jwt-key-minimum-32-characters
```

### Full-Featured Development

```bash
# .env.dev
PROJECT_NAME=myapp
BASE_DOMAIN=local.nself.org
ENV=dev

# Database
POSTGRES_DB=myapp_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=dev-password

# Hasura
HASURA_GRAPHQL_ADMIN_SECRET=dev-secret
HASURA_JWT_KEY=dev-jwt-key-minimum-32-characters

# Enable optional services
REDIS_ENABLED=true
MINIO_ENABLED=true
FUNCTIONS_ENABLED=true
NSELF_ADMIN_ENABLED=true
MAILPIT_ENABLED=true
MEILISEARCH_ENABLED=true

# Monitoring
MONITORING_ENABLED=true

# Custom services
CS_1=api:express-ts:8001:api
CS_2=worker:bullmq-ts:8002
```

### Production Setup

```bash
# .env.prod (on production server)
PROJECT_NAME=myapp
BASE_DOMAIN=example.com
ENV=prod

# Database (use strong passwords!)
POSTGRES_DB=myapp_prod
POSTGRES_USER=postgres
POSTGRES_PASSWORD=<generated-strong-password>

# Hasura
HASURA_GRAPHQL_ADMIN_SECRET=<generated-strong-secret>
HASURA_JWT_KEY=<generated-strong-key-minimum-32-characters>

# Production services
REDIS_ENABLED=true
MINIO_ENABLED=true
MONITORING_ENABLED=true

# SSL
SSL_MODE=letsencrypt

# Custom services (production-tuned)
CS_1=api:express-ts:8001:api
CS_1_REPLICAS=3
CS_1_MEMORY=1G
CS_1_CPU=1.0
CS_1_RATE_LIMIT=100
```

---

## Environment-Specific Configurations

### Development (.env.dev)

**Purpose:** Shared team defaults, fast iteration

**Characteristics:**
- Debug mode enabled
- Verbose logging
- Lenient security
- Auto-reload enabled
- Mock data allowed

**Example:**
```bash
ENV=dev
DEBUG=true
LOG_LEVEL=debug
HASURA_GRAPHQL_DEV_MODE=true
HASURA_GRAPHQL_ENABLE_CONSOLE=true
```

---

### Staging (.env.staging)

**Purpose:** Production-like testing

**Characteristics:**
- Similar to production
- Full logging
- SSL preferred
- Daily backups
- Realistic data

**Example:**
```bash
ENV=staging
BASE_DOMAIN=staging.example.com
SSL_MODE=letsencrypt
BACKUP_ENABLED=true
BACKUP_SCHEDULE="0 2 * * *"
```

---

### Production (.env.prod)

**Purpose:** Live production environment

**Characteristics:**
- Strict security
- Minimal logging
- SSL required
- Encryption enabled
- Daily backups + PITR
- Audit logging

**Example:**
```bash
ENV=prod
BASE_DOMAIN=example.com
SSL_MODE=letsencrypt
LOG_LEVEL=warn
HASURA_GRAPHQL_DEV_MODE=false
HASURA_GRAPHQL_ENABLE_CONSOLE=false
BACKUP_ENABLED=true
BACKUP_SCHEDULE="0 2 * * *"
```

---

## Custom Services (CS_N) Quick Reference

### Basic Format

```bash
CS_N=service_name:template_type[:port][:route]
```

### Available Templates (40+)

**JavaScript/TypeScript:**
- `express-js`, `express-ts` - Express.js
- `fastify-js`, `fastify-ts` - Fastify
- `nest-js`, `nest-ts` - NestJS
- `hono-js`, `hono-ts` - Hono
- `bullmq-js`, `bullmq-ts` - BullMQ workers
- `socketio-js`, `socketio-ts` - WebSockets
- `temporal-js`, `temporal-ts` - Workflows
- `trpc` - Type-safe APIs
- `bun` - Bun runtime
- `deno` - Deno runtime

**Python:**
- `fastapi` - FastAPI
- `flask` - Flask
- `django-rest` - Django REST
- `celery` - Task queue
- `ray` - Distributed computing
- `agent-llm` - LLM agents
- `agent-data` - Data processing
- `agent-vision` - Computer vision

**Other Languages:**
- `gin`, `echo`, `fiber`, `grpc` (Go)
- `rails`, `sinatra` (Ruby)
- `actix-web` (Rust)
- `spring-boot` (Java)
- `aspnet` (C#)
- `laravel` (PHP)
- `phoenix` (Elixir)
- `vapor` (Swift)
- `ktor` (Kotlin)

### Configuration Variables

Every CS_N service supports:

```bash
CS_N=service_name:template:port:route

# Optional configuration
CS_N_PORT=<port>                    # Override port
CS_N_ROUTE=<route>                  # External route
CS_N_MEMORY=<size>                  # Memory limit (default: 512M)
CS_N_CPU=<cores>                    # CPU limit (default: 0.5)
CS_N_REPLICAS=<count>               # Instances (default: 1)
CS_N_PUBLIC=<true|false>            # External access
CS_N_HEALTHCHECK=<path>             # Health endpoint (default: /health)
CS_N_TABLE_PREFIX=<prefix>          # Database table prefix
CS_N_REDIS_PREFIX=<prefix>          # Redis key prefix
CS_N_ENV=KEY=val,KEY2=val2          # Custom env vars
CS_N_DEPENDS_ON=svc1,svc2           # Dependencies
CS_N_VOLUMES=host:container         # Volume mounts
CS_N_NETWORKS=net1,net2             # Network config
CS_N_PORTS=host:container           # Additional ports
CS_N_RESTART_POLICY=<policy>        # Restart behavior
CS_N_RATE_LIMIT=<requests/min>      # Rate limiting
CS_N_DEV_DOMAIN=<domain>            # Dev domain override
CS_N_PROD_DOMAIN=<domain>           # Prod domain override
```

**See [CUSTOM-SERVICES-ENV-VARS.md](CUSTOM-SERVICES-ENV-VARS.md) for complete documentation.**

---

## Related Documentation

### Guides
- [Service-to-Service Communication](../guides/SERVICE-TO-SERVICE-COMMUNICATION.md)
- [Multi-App Setup](../guides/MULTI_APP_SETUP.md)
- [Deployment Architecture](../guides/DEPLOYMENT-ARCHITECTURE.md)
- [Database Workflow](../guides/DATABASE-WORKFLOW.md)

### Commands
- [init](../commands/INIT.md) - Project initialization
- [build](../commands/BUILD.md) - Configuration generation
- [start](../commands/START.md) - Starting services
- [env](../commands/ENV.md) - Environment management

### Services
- [Services Overview](../services/SERVICES.md)
- [Custom Services](../services/SERVICES_CUSTOM.md)
- [Service Templates](../services/SERVICE-TEMPLATES.md)

---

## Configuration Troubleshooting

### Service Not Starting

**Check:**
1. Environment variables are set
2. Port conflicts (`docker ps`)
3. Dependency order (`CS_N_DEPENDS_ON`)
4. Resource limits (`CS_N_MEMORY`, `CS_N_CPU`)

**Debug:**
```bash
docker logs ${PROJECT_NAME}_service-name
```

---

### Route Not Working

**Check:**
1. `CS_N_ROUTE` is set
2. `BASE_DOMAIN` is correct
3. nginx is running
4. DNS resolution

**Debug:**
```bash
nself urls                    # List all routes
nself urls --check-conflicts  # Check for conflicts
```

---

### Database Connection Failing

**Check:**
1. `POSTGRES_HOST=postgres` (not localhost)
2. `POSTGRES_PORT=5432` (internal port)
3. Credentials match
4. Database created

**Debug:**
```bash
docker exec -it ${PROJECT_NAME}_postgres psql -U ${POSTGRES_USER} -d ${POSTGRES_DB}
```

---

### Redis Connection Failing

**Check:**
1. `REDIS_ENABLED=true`
2. `REDIS_HOST=redis` (not localhost)
3. `REDIS_PORT=6379`
4. Password matches (if set)

**Debug:**
```bash
docker exec -it ${PROJECT_NAME}_redis redis-cli ping
```

---

## Best Practices

### Security

1. **Never commit secrets**
   ```bash
   # Use .env (gitignored) for local secrets
   # Use .secrets for production secrets (generated on server)
   ```

2. **Use strong passwords in production**
   ```bash
   POSTGRES_PASSWORD=$(openssl rand -hex 32)
   HASURA_GRAPHQL_ADMIN_SECRET=$(openssl rand -hex 32)
   ```

3. **Enable SSL in production**
   ```bash
   SSL_MODE=letsencrypt
   ```

---

### Performance

1. **Right-size resources**
   ```bash
   CS_1_MEMORY=512M  # Not 4G for a simple API
   CS_1_CPU=0.5      # Not 4.0 for light load
   ```

2. **Use replicas for high-traffic**
   ```bash
   CS_1_REPLICAS=3
   ```

3. **Enable monitoring**
   ```bash
   MONITORING_ENABLED=true
   ```

---

### Organization

1. **Use consistent naming**
   ```bash
   CS_1=user-api:express-ts:8001
   CS_2=user-worker:bullmq-ts:8002
   CS_3=user-processor:fastapi:8003
   ```

2. **Document your services**
   ```bash
   # User Management Stack
   CS_1=user-api:express-ts:8001        # User CRUD operations
   CS_2=user-worker:bullmq-ts:8002      # Async user tasks
   ```

3. **Use table prefixes**
   ```bash
   CS_1_TABLE_PREFIX=user_
   CS_2_TABLE_PREFIX=order_
   ```

---

**Last Updated:** January 30, 2026
**nself Version:** 0.4.8+
