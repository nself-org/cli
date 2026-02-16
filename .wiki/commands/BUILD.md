# nself build - Configuration Generation

**Version 0.9.9** | Generate Docker Compose, Nginx, and service configurations

---

## Overview

The `nself build` command reads your `.env` configuration and generates all files needed to run your nself project:

- **docker-compose.yml** - Container orchestration
- **nginx/** - Reverse proxy configuration
- **postgres/** - Database initialization scripts
- **ssl/** - SSL certificates for HTTPS
- **services/** - Custom service code from templates
- **monitoring/** - Prometheus, Grafana, Loki configuration

---

## Table of Contents

- [Basic Usage](#basic-usage)
- [What Gets Generated](#what-gets-generated)
- [Options Reference](#options-reference)
- [Build Process](#build-process)
- [Configuration Modules](#configuration-modules)
- [Custom Services](#custom-services)
- [SSL Certificates](#ssl-certificates)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Basic Usage

```bash
# Standard build
nself build

# Verbose output
nself build --verbose

# Force regenerate all files
nself build --force

# Build specific components only
nself build --only nginx
nself build --only compose
nself build --only ssl
```

---

## What Gets Generated

### docker-compose.yml

The main orchestration file containing all enabled services:

```yaml
# Example generated docker-compose.yml
version: "3.8"
services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./postgres/init:/docker-entrypoint-initdb.d
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER}"]
      interval: 5s
      timeout: 5s
      retries: 5

  hasura:
    image: hasura/graphql-engine:v2.36.0
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      HASURA_GRAPHQL_DATABASE_URL: postgres://...
      HASURA_GRAPHQL_ADMIN_SECRET: ${HASURA_GRAPHQL_ADMIN_SECRET}
    # ... more configuration

  # Additional services based on .env configuration
```

### Nginx Configuration

```
nginx/
├── nginx.conf                    # Main configuration
├── includes/
│   ├── security-headers.conf     # Security headers
│   ├── gzip.conf                 # Compression settings
│   ├── proxy-params.conf         # Proxy parameters
│   ├── rate-limiting.conf        # Rate limit zones
│   └── ssl-params.conf           # SSL/TLS settings
└── sites/
    ├── default.conf              # Default server
    ├── hasura.conf               # GraphQL API
    ├── auth.conf                 # Authentication
    ├── minio.conf                # Storage (if enabled)
    ├── grafana.conf              # Monitoring (if enabled)
    ├── custom-api.conf           # Custom services
    └── webhooks.conf             # Plugin webhooks
```

### PostgreSQL Initialization

```
postgres/
└── init/
    └── 00-init.sql               # Extensions, roles, base schema
```

### SSL Certificates

```
ssl/
└── certificates/
    └── localhost/                # Or your domain
        ├── fullchain.pem         # Certificate chain
        └── privkey.pem           # Private key
```

### Custom Services

```
services/
├── express_api/                  # CS_1 from template
│   ├── Dockerfile
│   ├── package.json
│   ├── src/
│   │   └── index.js
│   └── ...
├── bullmq_worker/                # CS_2 from template
└── ...
```

### Monitoring (if enabled)

```
monitoring/
├── prometheus/
│   └── prometheus.yml
├── grafana/
│   ├── provisioning/
│   │   ├── dashboards/
│   │   └── datasources/
│   └── dashboards/
├── loki/
│   └── loki-config.yaml
├── promtail/
│   └── promtail-config.yaml
└── alertmanager/
    └── alertmanager.yml
```

---

## Options Reference

| Option | Short | Description |
|--------|-------|-------------|
| `--verbose` | `-v` | Show detailed output |
| `--force` | `-f` | Regenerate all files |
| `--only <component>` | `-o` | Build specific component |
| `--no-ssl` | | Skip SSL certificate generation |
| `--no-services` | | Skip custom service generation |
| `--dry-run` | | Show what would be generated |
| `--validate` | | Validate configuration without building |

### Component Options

```bash
# Build only specific components
nself build --only compose     # Just docker-compose.yml
nself build --only nginx       # Just nginx configuration
nself build --only ssl         # Just SSL certificates
nself build --only services    # Just custom service templates
nself build --only monitoring  # Just monitoring configuration
nself build --only postgres    # Just PostgreSQL init scripts
```

---

## Build Process

### Step-by-Step Flow

```
1. Load Configuration
   ├── Read .env file
   ├── Apply defaults for missing values
   └── Validate required settings

2. Generate Docker Compose
   ├── Add required services (4)
   ├── Add enabled optional services
   ├── Add monitoring if enabled
   ├── Add custom services (CS_1-CS_10)
   ├── Configure networks and volumes
   └── Write docker-compose.yml

3. Generate Nginx Configuration
   ├── Create main nginx.conf
   ├── Generate includes (security, gzip, etc.)
   ├── Create site configs for each service
   ├── Configure SSL settings
   └── Generate upstream definitions

4. Generate SSL Certificates
   ├── Check for existing certificates
   ├── Generate self-signed if needed
   ├── Configure certificate paths
   └── Trust certificates (macOS)

5. Generate PostgreSQL Scripts
   ├── Create database init script
   ├── Configure extensions
   └── Set up roles and permissions

6. Generate Custom Services
   ├── Copy template files
   ├── Replace placeholders
   ├── Configure service-specific settings
   └── Generate Dockerfiles

7. Generate Monitoring (if enabled)
   ├── Configure Prometheus targets
   ├── Set up Grafana dashboards
   ├── Configure Loki log collection
   └── Set up alert rules
```

### Environment Validation

The build process validates:

- Required environment variables are set
- Port numbers don't conflict
- Service dependencies are met
- Template files exist for custom services
- SSL certificates are valid (if using external)

---

## Configuration Modules

### Required Services

Always included in docker-compose.yml:

| Service | Image | Purpose |
|---------|-------|---------|
| postgres | postgres:15 | Database |
| hasura | hasura/graphql-engine:v2.36.0 | GraphQL API |
| auth | nhost/hasura-auth:0.24.0 | Authentication |
| nginx | nginx:1.25-alpine | Reverse proxy |

### Optional Services

Included when `*_ENABLED=true`:

| Variable | Service | Image |
|----------|---------|-------|
| `REDIS_ENABLED` | redis | redis:7-alpine |
| `MINIO_ENABLED` | minio | minio/minio:latest |
| `MAILPIT_ENABLED` | mailpit | axllent/mailpit:latest |
| `MEILISEARCH_ENABLED` | meilisearch | getmeili/meilisearch:v1.6 |
| `FUNCTIONS_ENABLED` | functions | nhost/functions:latest |
| `MLFLOW_ENABLED` | mlflow | ghcr.io/mlflow/mlflow:v2.10.0 |
| `NSELF_ADMIN_ENABLED` | nself-admin | ghcr.io/acamarata/nself-admin:latest |

### Monitoring Bundle

When `MONITORING_ENABLED=true`, includes all 10 services:

| Service | Image | Purpose |
|---------|-------|---------|
| prometheus | prom/prometheus:v2.48.0 | Metrics collection |
| grafana | grafana/grafana:10.2.3 | Visualization |
| loki | grafana/loki:2.9.3 | Log aggregation |
| promtail | grafana/promtail:2.9.3 | Log shipping |
| tempo | grafana/tempo:2.3.1 | Distributed tracing |
| alertmanager | prom/alertmanager:v0.26.0 | Alert routing |
| cadvisor | gcr.io/cadvisor/cadvisor:v0.47.2 | Container metrics |
| node-exporter | prom/node-exporter:v1.7.0 | System metrics |
| postgres-exporter | prometheuscommunity/postgres-exporter:v0.15.0 | PostgreSQL metrics |
| redis-exporter | oliver006/redis_exporter:v1.56.0 | Redis metrics |

---

## Custom Services

### Template System

Custom services are generated from templates in `src/templates/services/`:

```
src/templates/services/
├── javascript/
│   ├── express-js/
│   ├── fastify-js/
│   ├── nestjs-ts/
│   ├── hono-js/
│   └── bullmq-js/
├── python/
│   ├── fastapi/
│   ├── flask/
│   ├── django/
│   └── celery/
├── go/
│   ├── gin/
│   ├── fiber/
│   ├── echo/
│   └── grpc/
└── rust/
    ├── actix-web/
    └── axum/
```

### Configuration Format

```bash
# Format: CS_N=name:template:port
CS_1=api:express-js:3001
CS_2=worker:bullmq-js:3002
CS_3=grpc:go-grpc:50051
CS_4=ml:python-fastapi:8000
```

### Template Processing

1. **Copy template files** to `services/<name>/`
2. **Replace placeholders:**
   - `{{SERVICE_NAME}}` → Service name
   - `{{SERVICE_PORT}}` → Port number
   - `{{PROJECT_NAME}}` → Project name
3. **Generate Dockerfile** if not present
4. **Configure docker-compose.yml** entry

### First-Time Only

Template files are only generated on first build. Subsequent builds preserve your customizations:

```bash
# First build - creates services/api/
nself build

# Edit services/api/src/index.js
# ...

# Second build - preserves your changes
nself build
```

---

## SSL Certificates

### Self-Signed (Development)

By default, build generates self-signed certificates for local development:

```bash
nself build
# Generates ssl/certificates/localhost/
```

### Auto-Trust (macOS)

On macOS, certificates are automatically added to the system keychain:

```bash
nself trust   # Adds certificate to keychain
```

### Custom Certificates

For production, provide your own certificates:

```bash
# In .env
SSL_CERT_PATH=/path/to/fullchain.pem
SSL_KEY_PATH=/path/to/privkey.pem
```

### Let's Encrypt

For automatic Let's Encrypt certificates:

```bash
# In .env
LETSENCRYPT_ENABLED=true
LETSENCRYPT_EMAIL=admin@yourdomain.com
BASE_DOMAIN=yourdomain.com
```

---

## Examples

### Standard Development Build

```bash
cd myapp
nself build

# Output:
# ✓ Loading configuration
# ✓ Generating docker-compose.yml (12 services)
# ✓ Generating nginx configuration
# ✓ Generating SSL certificates
# ✓ Generating PostgreSQL init scripts
# ✓ Build complete!
#
# Next: nself start
```

### Verbose Build

```bash
nself build --verbose

# Shows detailed output for each step:
# [nginx] Generating main configuration
# [nginx] Creating security-headers.conf
# [nginx] Creating hasura.conf
# [nginx] Creating auth.conf
# ...
```

### Rebuild After Configuration Change

```bash
# Edit .env to enable Redis
echo "REDIS_ENABLED=true" >> .env

# Rebuild
nself build --force
```

### Build Only Nginx

```bash
# After nginx configuration changes
nself build --only nginx

# Then reload nginx
docker exec myapp_nginx nginx -s reload
```

### Validate Without Building

```bash
nself build --validate

# Output:
# ✓ Configuration valid
# ✓ All required variables set
# ✓ No port conflicts
# ✓ All templates available
```

---

## Troubleshooting

### "Missing required variable"

```bash
# Check what's missing
nself build --validate

# Add missing variables to .env
echo "HASURA_GRAPHQL_ADMIN_SECRET=your-secret" >> .env
nself build
```

### "Port conflict detected"

```bash
# Check for conflicts
nself build --validate

# Change conflicting port in .env
# Example: change CS_1 port from 3001 to 3002
```

### "Template not found"

```bash
# List available templates
ls src/templates/services/

# Check template name spelling
CS_1=api:express-js:3001  # Correct
CS_1=api:expressjs:3001   # Wrong - missing hyphen
```

### "Permission denied on SSL"

```bash
# Ensure ssl directory is writable
chmod 755 ssl/
nself build

# Or skip SSL regeneration
nself build --no-ssl
```

### Build Succeeds But Services Won't Start

```bash
# Validate docker-compose.yml
docker compose config

# Check for syntax errors
docker compose config --quiet && echo "Valid" || echo "Invalid"
```

---

## Generated File Locations

| File/Directory | Purpose |
|----------------|---------|
| `docker-compose.yml` | Main orchestration file |
| `nginx/nginx.conf` | Nginx main configuration |
| `nginx/sites/*.conf` | Per-service routing |
| `nginx/includes/*.conf` | Shared nginx includes |
| `postgres/init/00-init.sql` | Database initialization |
| `ssl/certificates/` | SSL certificates |
| `services/` | Custom service code |
| `monitoring/prometheus/` | Prometheus configuration |
| `monitoring/grafana/` | Grafana dashboards |
| `monitoring/loki/` | Loki configuration |

---

## Related Commands

- [init](INIT.md) - Initialize new project
- [start](START.md) - Start services
- [stop](STOP.md) - Stop services
- [status](STATUS.md) - Check service status
- [urls](URLS.md) - View service URLs

---

*Last Updated: January 2026 | Version 0.9.9*
