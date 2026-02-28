# Services Documentation

Complete guide to all services available in ɳSelf.

## Overview

- **[Services Overview](SERVICES.md)** - All available services
- **[Service Comparison](SERVICE-COMPARISON.md)** - Decision matrix and resource guide
- **[Service Reference](SERVICE_REFERENCE.md)** - Complete service reference
- **[Service Templates](SERVICE-TEMPLATES.md)** - 40+ custom service templates

## Service Categories

### Required Services (4)

Always running - the core of ɳSelf:

- **[Required Services](SERVICES_REQUIRED.md)** - Complete reference
  - **PostgreSQL** - Primary database
  - **Hasura GraphQL Engine** - Auto-generated GraphQL API
  - **Auth** - Authentication service (nHost Auth)
  - **Nginx** - Reverse proxy and SSL termination

**Configuration**: These are always enabled. Configure via `.env` variables.

### Optional Services (7)

Enable as needed with `*_ENABLED=true`:

- **[Optional Services](SERVICES_OPTIONAL.md)** - Complete reference
  - **nself-admin** - Web management UI (`NSELF_ADMIN_ENABLED=true`)
  - **MinIO** - S3-compatible storage (`MINIO_ENABLED=true`)
  - **Redis** - Cache and sessions (`REDIS_ENABLED=true`)
  - **Functions** - Serverless functions (`FUNCTIONS_ENABLED=true`)
  - **MLflow** - ML experiment tracking (`MLFLOW_ENABLED=true`)
  - **Mail** - Email service (`MAILPIT_ENABLED=true` for dev)
  - **Search** - Search service (`MEILISEARCH_ENABLED=true`)

### Monitoring Bundle (10)

Enable all with `MONITORING_ENABLED=true`:

- **[Monitoring Bundle](MONITORING-BUNDLE.md)** - Complete observability stack
  - **Prometheus** - Metrics collection
  - **Grafana** - Visualization dashboards
  - **Loki** - Log aggregation
  - **Promtail** - Log shipping (required for Loki)
  - **Tempo** - Distributed tracing
  - **Alertmanager** - Alert routing
  - **cAdvisor** - Container metrics
  - **Node Exporter** - System metrics
  - **Postgres Exporter** - Database metrics
  - **Redis Exporter** - Redis metrics

**Note**: The monitoring bundle is all-or-nothing. Enable all 10 services, then optionally disable individual ones.

### Custom Services (Unlimited)

Build from 40+ templates:

- **[Custom Services](SERVICES_CUSTOM.md)** - Build your own services
- **[Service Templates](SERVICE-TEMPLATES.md)** - Available templates

**Configuration**: `CS_N=service_name:template_type:port`

#### Available Templates by Language

**JavaScript/TypeScript**
- Express, Fastify, NestJS, Hono, Koa, BullMQ

**Python**
- FastAPI, Flask, Django, Celery, aiohttp

**Go**
- Gin, Fiber, Echo, gRPC, Chi

**Rust**
- Actix, Rocket, Axum, Warp

**Java**
- Spring Boot, Quarkus, Micronaut

**PHP**
- Laravel, Symfony, Slim

**Ruby**
- Rails, Sinatra, Sidekiq

**C#**
- ASP.NET Core, ServiceStack

**Elixir**
- Phoenix

**Other**
- Static files, Webhooks, Schedulers

## Service-Specific Documentation

### Core Services

- **[PostgreSQL](SERVICES_REQUIRED.md#postgresql)** - Database configuration
- **[Hasura](SERVICES_REQUIRED.md#hasura)** - GraphQL setup
- **[Auth](SERVICES_REQUIRED.md#auth)** - Authentication config
- **[Nginx](SERVICES_REQUIRED.md#nginx)** - Reverse proxy setup

### Optional Services

- **[nself-admin](NSELF_ADMIN.md)** - Admin dashboard
- **[Search Services](SEARCH.md)** - Search configuration
  - [MeiliSearch](SEARCH.md#meilisearch)
  - [Typesense](TYPESENSE.md)

### Demo Configuration

- **[Demo Setup](DEMO_SETUP.md)** - Full demo with 25 services

## Service Management

### Enabling Services

**Required Services** - Always enabled:
```bash
# No configuration needed - they just work
```

**Optional Services** - Enable individually:
```bash
# In .env file:
REDIS_ENABLED=true
MINIO_ENABLED=true
NSELF_ADMIN_ENABLED=true
```

**Monitoring Bundle** - Enable all at once:
```bash
# In .env file:
MONITORING_ENABLED=true

# Optionally disable specific services:
TEMPO_ENABLED=false  # Example: disable tracing
```

**Custom Services** - Define in .env:
```bash
CS_1=api:express-js:8001
CS_2=worker:bullmq-js:8002
CS_3=grpc:grpc:8003
```

### Service Lifecycle

```bash
# Generate configurations
nself build

# Start all enabled services
nself start

# Check service status
nself status

# View service URLs
nself urls

# View logs
nself logs [service-name]

# Execute in container
nself exec [service-name] [command]

# Stop services
nself stop
```

## Service URLs

After `nself start`, access services at:

**Required Services**
- API: `api.local.nself.org`
- Auth: `auth.local.nself.org`

**Optional Services** (when enabled)
- Admin: `admin.local.nself.org`
- MinIO: `minio.local.nself.org`
- Functions: `functions.local.nself.org`
- Mail (dev): `mail.local.nself.org`
- Search: `search.local.nself.org`
- MLflow: `mlflow.local.nself.org`

**Monitoring** (when enabled)
- Grafana: `grafana.local.nself.org`
- Prometheus: `prometheus.local.nself.org`
- Alertmanager: `alertmanager.local.nself.org`

**Custom Services**
- Based on service name: `[service-name].local.nself.org`

## Service Configuration

### Port Mapping

**Internal Ports** - Used inside Docker network:
- PostgreSQL: 5432
- Hasura: 8080
- Auth: 4000
- Nginx: 80, 443

**External Ports** - Access from host machine:
- Define in `.env` for external access
- Example: `POSTGRES_PORT=5433` (host) → 5432 (container)

### Environment Variables

Each service has specific configuration variables. See:
- [Environment Variables](../configuration/ENVIRONMENT-VARIABLES.md)
- [Custom Services Env Vars](../configuration/CUSTOM-SERVICES-ENV-VARS.md)

## Service Count Summary

### Total Docker Containers (Demo)
- Required: 4
- Optional: 7 (when all enabled)
- Monitoring: 10 (when bundle enabled)
- Custom: 4 (demo configuration)
- **Total: 25 containers**

### Total Routes (Demo)
- Required services: 2 routes
- Optional services: 13 routes (when all enabled)
- Custom services: 3 routes (demo)
- Frontend apps: 2 routes (external)
- Application root: 1 route
- **Total: 21 routes**

---

**[← Back to Documentation Home](../README.md)**
