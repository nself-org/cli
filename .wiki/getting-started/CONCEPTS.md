# Core Concepts

Understand the fundamental concepts of nself that you'll use every day.

## What is nself?

nself is a self-hosted backend infrastructure platform. It provides:

- **Complete Backend Stack** - Database, GraphQL, authentication, storage, all working together
- **Zero Vendor Lock-in** - Own your infrastructure completely
- **Production-Ready** - Security, monitoring, backups built-in from day one
- **Developer-Friendly** - Simple commands, sensible defaults, fast development

## Core Services

### 1. PostgreSQL (Database)

Your persistent data store. Every nself project includes PostgreSQL.

```bash
# Access the database
nself db console

# Create a backup
nself db backup

# Run migrations
nself db migrate
```

### 2. Hasura GraphQL Engine

Automatically generates a GraphQL API from your database schema.

- **Automatic schema** - Create a table, get GraphQL resolvers
- **Subscriptions** - Real-time updates via WebSockets
- **Permissions** - Row-level security at the database level
- **Actions** - Custom resolvers for business logic

```bash
# Open Hasura console
nself admin
```

### 3. Authentication Service

Built-in user authentication with JWT tokens.

- **User management** - Create, update, delete users
- **JWT tokens** - Secure API authentication
- **Multi-factor auth** - Optional 2FA
- **OAuth integration** - Google, GitHub, etc.

See [Authentication Guide](../guides/AUTHENTICATION.md) for detailed setup.

### 4. Nginx Reverse Proxy

Routes all traffic, handles SSL/TLS termination.

- **Service discovery** - Routes `api.*` to Hasura, `auth.*` to auth service
- **SSL certificates** - Automatic HTTPS
- **Compression** - gzip compression for responses
- **Security headers** - X-Frame-Options, CSP, etc.

## Optional Services

Enable only what you need. Add to `.env`:

### Redis (Cache & Sessions)

```bash
REDIS_ENABLED=true
```

In-memory cache for performance:
- Session storage
- Rate limiting
- Real-time features

### MinIO (File Storage)

```bash
MINIO_ENABLED=true
```

S3-compatible object storage:
- User uploads
- Document storage
- Static assets

### Monitoring Bundle

```bash
MONITORING_ENABLED=true
```

Complete observability stack:
- **Prometheus** - Metrics collection
- **Grafana** - Dashboards and visualization
- **Loki** - Log aggregation
- **Tempo** - Distributed tracing

## Key Architectural Concepts

### Multi-Tenancy

nself is **multi-tenant by default**. Create isolated environments for:

- Different organizations
- Development/staging/production
- Different projects

```bash
# Create a new tenant
nself tenant create acme-corp --plan enterprise

# Switch to tenant
nself tenant switch acme-corp
```

Each tenant:
- Has its own database schema
- Isolated data
- Separate authentication realm
- Per-tenant customization

### Row-Level Security (RLS)

Control data access at the row level using PostgreSQL RLS and Hasura permissions.

**Example**: Users can only see their own posts

```sql
-- Database policy
CREATE POLICY user_posts ON posts
  USING (user_id = current_user_id());
```

```graphql
-- Hasura permission
posts.select: user_id = {{ X-Hasura-User-Id }}
```

Result: The GraphQL API only returns posts where `user_id` matches the authenticated user.

### Schema Migrations

Version control your database schema with migrations.

```bash
# Create a migration
nself db migrate create "add_posts_table"

# Apply migrations
nself db migrate

# Rollback
nself db migrate rollback
```

Migrations are stored in `migrations/` and version controlled.

### Environment Cascade

Configuration follows a cascade pattern. Each file overrides the previous:

```
.env.dev          # Committed to git (shared defaults)
   ↓
.env.local        # On your machine (your overrides)
   ↓
.env.staging      # On staging server
   ↓
.env.prod         # On production server
   ↓
.env.secrets      # Ultra-sensitive credentials
```

Example:

```bash
# .env.dev (committed)
DATABASE_NAME=myapp_dev
HASURA_ADMIN_SECRET=dev-secret

# .env.local (your machine only)
DATABASE_NAME=myapp_local
HASURA_ADMIN_SECRET=my-local-secret-123

# Result: .env.local overrides .env.dev
```

See [Cascading Configuration](../configuration/CASCADING-OVERRIDES.md) for details.

## Services in Docker

nself uses Docker Compose to orchestrate services:

```bash
# View running containers
docker ps

# View logs
docker logs postgres
docker logs hasura-engine
```

### Container Naming

Services follow the pattern:

```
<project-name>-<service-name>
```

Example with `PROJECT_NAME=my-app`:
- `my-app_postgres`
- `my-app_hasura-engine`
- `my-app_auth-service`
- `my-app_nginx-proxy`

### Container Networking

All containers on the same Docker network:

```
nself-network (or <project-name>-network)
```

Services communicate via hostname:
- `postgres:5432` - Database
- `hasura-engine:8080` - Hasura
- `auth-service:8080` - Auth

## Custom Services

Add your own services alongside the built-in services.

```bash
# In .env
CS_1=my-api:express-js:8001
CS_2=my-worker:bullmq-js:8002
```

When you run `nself build`:
1. Creates `services/my-api/` with Express template
2. Generates Dockerfile
3. Adds to docker-compose.yml
4. Routes `api.localhost` to port 8001

Edit your code, restart:

```bash
nself restart my-api
nself logs my-api -f
```

## Environment Variables

Configuration via environment variables:

```bash
# Required
PROJECT_NAME=my-app
ENV=dev

# Services
POSTGRES_DB=myapp_db
HASURA_GRAPHQL_ADMIN_SECRET=secret123

# Optional services (enable with true)
REDIS_ENABLED=true
MINIO_ENABLED=true
MONITORING_ENABLED=true

# Custom services
CS_1=api:express-js:8001

# Access
BASE_DOMAIN=localhost
NGINX_PORT=8080
```

## Common Workflows

### 1. Develop with Hasura

```bash
# Start services
nself start

# Open Hasura console
nself admin

# Create table in console
# Add permissions
# Write GraphQL queries

# Test in GraphQL playground
```

### 2. Add a Custom API

```bash
# Add to .env
CS_1=api:nestjs:8001

# Build and start
nself build
nself restart api

# Edit code in services/api/
# Restart to apply changes
nself restart api
```

### 3. Deploy to Production

```bash
# On production server
nself init --env prod
vim .env.prod

# Build and start
nself build
nself start

# Monitor
nself monitor
nself health
```

## Next Steps

Choose what to learn next:

1. **[Authentication](../guides/AUTHENTICATION.md)** - Set up users, JWT, OAuth
2. **[Database Guide](../guides/DATABASE-WORKFLOW.md)** - Tables, migrations, backups
3. **[Deployment](../guides/DEPLOYMENT-ARCHITECTURE.md)** - Production setup
4. **[Examples](../examples/README.md)** - Sample projects

---

**Key Takeaway**: nself gives you a complete, modern backend stack with minimal complexity. Focus on your application logic, not infrastructure.
