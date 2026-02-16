# nself Service Reference

## Complete Service Inventory

This is the authoritative reference for all services available in nself, organized by category.

---

## Required Services (4)

These services are **always enabled** and form the core infrastructure:

1. **PostgreSQL** - Primary database
   - Port: 5432
   - URL: Internal only
   - Extensions: uuid-ossp, pgcrypto, pg_trgm, pgvector, PostGIS, hstore

2. **Hasura GraphQL Engine** - GraphQL API layer
   - Port: 8080
   - URL: `https://api.<domain>`
   - Console: `https://api.<domain>/console`

3. **nHost Auth Service** - Authentication & authorization
   - Port: 4000
   - URL: `https://auth.<domain>`
   - JWT-based authentication

4. **Nginx** - Reverse proxy & SSL termination
   - Ports: 80, 443
   - Handles all routing
   - SSL/TLS termination

---

## Optional Services (7)

### THE AUTHORITATIVE LIST - EXACTLY THESE 7

1. **nself Admin** - Web-based management UI
   - Port: 3021
   - URL: `https://admin.<domain>`
   - Complete project management interface

2. **MinIO** - S3-compatible object storage
   - Port: 9000 (API), 9001 (Console)
   - URL: `https://minio.<domain>`
   - Console access for file management

3. **Redis** - In-memory cache & sessions
   - Port: 6379
   - Used by: Auth, Custom services
   - No public route (internal only)

4. **Functions** - Serverless functions runtime
   - Port: 3008
   - URL: `https://functions.<domain>`
   - JavaScript/TypeScript functions

5. **MLflow** - ML experiment tracking
   - Port: 5005
   - URL: `https://mlflow.<domain>`
   - Model registry included

6. **Mail Service** (Provider-based)
   - **MailPit** (development) - Ports: 1025 (SMTP), 8025 (UI)
   - **SMTP** (production) - Various providers
   - **SendGrid/SES/etc** - API-based providers
   - URL: `https://mail.<domain>` (when UI available)

7. **Search Service** (Provider-based)
   - **MeiliSearch** - Port: 7700
   - **Typesense** - Alternative option
   - **Sonic** - Lightweight option
   - URL: `https://search.<domain>`

### NO OTHER OPTIONAL SERVICES

**IMPORTANT:** If you need additional functionality like:
- BullMQ Dashboard → Implement as CS_N custom service
- Webhook processing → Implement as CS_N custom service
- Additional APIs → Implement as CS_N custom service
- Database admin tools → Use nself-admin or implement as CS_N
- CMS platforms → Implement as CS_N custom service

**DO NOT add these as built-in services. They must be custom services.**

---

## Monitoring Bundle (10 Services)

Complete observability stack that can be enabled with `MONITORING_ENABLED=true`:

1. **Prometheus** - Metrics database
   - Port: 9090
   - URL: `https://prometheus.<domain>`

2. **Grafana** - Visualization & dashboards
   - Port: 3006
   - URL: `https://grafana.<domain>`

3. **Loki** - Log aggregation
   - Port: 3100
   - Internal only

4. **Promtail** - Log shipping agent
   - Port: 9080
   - Internal only

5. **Tempo** - Distributed tracing
   - Port: 3200
   - Internal only

6. **Alertmanager** - Alert routing
   - Port: 9093
   - URL: `https://alertmanager.<domain>`

7. **cAdvisor** - Container metrics
   - Port: 8080
   - Metrics: `:8080/metrics`

8. **Node Exporter** - System metrics
   - Port: 9100
   - Metrics: `:9100/metrics`

9. **Postgres Exporter** - PostgreSQL metrics
   - Port: 9187
   - Metrics: `:9187/metrics`

10. **Redis Exporter** - Redis metrics
    - Port: 9121
    - Metrics: `:9121/metrics`

---

## Custom Services (Up to 10)

You can define up to 10 custom services using templates:

- **CS_1** through **CS_10**
- 40+ templates available
- Languages: JavaScript, TypeScript, Python, Go, Rust, Ruby, PHP, Java, C#, Elixir
- Frameworks: Express, Fastify, NestJS, FastAPI, Flask, Django, Gin, Fiber, Echo

### Demo Configuration (4 Custom Services)

The demo uses these custom services:

1. **CS_1: express_api** - Express.js REST API
   - Template: express-js
   - Port: 8001
   - Route: `express-api`

2. **CS_2: bullmq_worker** - BullMQ job worker
   - Template: bullmq-js
   - Port: 8002
   - No public route (internal)

3. **CS_3: go_grpc** - Go gRPC service
   - Template: grpc
   - Port: 8003
   - Route: `grpc-api`

4. **CS_4: python_api** - Python FastAPI service
   - Template: fastapi
   - Port: 8004
   - Route: `ml-api`

---

## Frontend Applications (External)

Frontend applications run outside Docker and connect via nginx routing:

- **FRONTEND_APP_1** through **FRONTEND_APP_10**
- Frameworks: Next.js, React, Vue, Angular, Svelte
- Remote schemas for GraphQL integration

### Demo Configuration (2 Frontend Apps)

1. **Frontend App 1**
   - System Name: app1
   - Port: 3000
   - Route: `https://app1.<domain>`
   - Framework: Next.js

2. **Frontend App 2**
   - System Name: app2
   - Port: 3001
   - Route: `https://app2.<domain>`
   - Framework: Next.js

---

## Service Counts by Configuration

### Minimal Setup (Dev)
- Required: 4
- Optional: 2 (Redis, MailPit)
- **Total: 6 services**

### Standard Setup (Staging)
- Required: 4
- Optional: 4 (Redis, MinIO, Mail, Search)
- **Total: 8 services**

### Production Setup
- Required: 4
- Optional: 5-6 (Redis, MinIO, Functions, Mail, Search, nself Admin)
- Monitoring: 10
- Custom: 2-4
- **Total: 21-24 services**

### Full Demo Setup
- Required: 4
- Optional: 7 (all optional services enabled)
- Monitoring: 10
- Custom: 4
- Frontend: 2 (external)
- **Total: 25 services + 2 frontend apps**

---

## Port Allocation Ranges

- **PostgreSQL**: 5432
- **Redis**: 6379
- **Hasura**: 8080
- **Auth**: 4000
- **MinIO**: 9000-9001
- **Custom Services**: 8001-8010
- **Frontend Apps**: 3000-3009
- **nself Admin**: 3021
- **Monitoring**: 9090-9199, 3100-3200
- **Search**: 7700-7799
- **ML Services**: 5000-5099

---

## Quick Reference Commands

```bash
# View all configured services
nself status

# View all service URLs
nself urls

# Check for route conflicts
nself urls --check-conflicts

# View service logs
nself logs <service-name>

# Scale a service
docker compose scale <service>=<count>
```

---

## Environment Variable Prefixes

- `POSTGRES_*` - PostgreSQL configuration
- `HASURA_*` - Hasura configuration
- `AUTH_*` - Authentication service
- `NGINX_*` - Nginx configuration
- `CS_*` - Custom services (CS_1 through CS_10)
- `FRONTEND_APP_*` - Frontend applications
- `*_ENABLED` - Service enable flags
- `*_PORT` - Service ports
- `*_ROUTE` - Service routes

---

## Related Documentation

- [Required Services Detail](SERVICES_REQUIRED.md)
- [Optional Services Detail](SERVICES_OPTIONAL.md)
- [Monitoring Bundle Detail](MONITORING-BUNDLE.md)
- [Custom Services Guide](SERVICES_CUSTOM.md)
- [Demo Setup](DEMO_SETUP.md)
- [nself Admin](NSELF_ADMIN.md)