# nself build

**Category**: Core Commands

Generate all configuration files and service scaffolding for your nself project.

## Overview

The `build` command transforms your `.env` configuration into production-ready service configurations, including:
- Docker Compose orchestration
- Nginx reverse proxy and SSL
- Service scaffolding from templates
- Monitoring dashboards
- Database initialization

**Features**:
- ✅ Hot reload (no restart required for most changes)
- ✅ Idempotent (safe to run multiple times)
- ✅ Smart detection (only regenerates what changed)
- ✅ Preserves user modifications
- ✅ Validates before generating

## Usage

```bash
nself build [OPTIONS]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `--force` | Force regenerate all files | false |
| `--clean` | Remove generated files before building | false |
| `--validate-only` | Validate .env without generating | false |
| `-v, --verbose` | Show detailed output | false |
| `-h, --help` | Show help message | - |

## What Gets Generated

### Core Files

```
docker-compose.yml          # Main orchestration (25+ services)
```

### Nginx Configuration

```
nginx/
├── nginx.conf             # Main Nginx config
├── includes/
│   ├── security-headers.conf
│   ├── gzip.conf
│   └── ssl-params.conf
└── sites/
    ├── api.conf           # api.domain → Hasura
    ├── auth.conf          # auth.domain → Auth
    ├── admin.conf         # admin.domain → nself Admin
    ├── storage.conf       # storage.domain → MinIO
    └── [custom].conf      # Custom service routes
```

### SSL Certificates

```
ssl/
├── cert.pem              # Self-signed cert (dev)
└── key.pem               # Private key
```

### PostgreSQL Initialization

```
postgres/
└── init/
    └── 00-init.sql       # Database, schemas, extensions
```

### Custom Services

```
services/
├── [service-1]/
│   ├── Dockerfile        # From template
│   ├── package.json      # Dependencies
│   ├── src/
│   │   └── index.js      # Entry point
│   └── .env              # Service-specific vars
├── [service-2]/
└── ...
```

### Monitoring Configuration

```
monitoring/
├── prometheus/
│   └── prometheus.yml    # Metrics collection
├── grafana/
│   ├── provisioning/
│   │   ├── datasources/
│   │   └── dashboards/
│   └── dashboards/
│       ├── system.json
│       ├── postgres.json
│       └── redis.json
├── loki/
│   └── loki-config.yaml  # Log aggregation
└── alertmanager/
    └── alertmanager.yml  # Alert routing
```

## Build Process

### Step-by-Step Flow

```
1. Load .env configuration
   ↓
2. Validate configuration
   → Check required variables
   → Validate ports (no conflicts)
   → Verify service dependencies
   ↓
3. Generate docker-compose.yml
   → Core services (4)
   → Optional services (7)
   → Monitoring bundle (10)
   → Custom services (CS_1 to CS_10)
   ↓
4. Generate Nginx configs
   → Main nginx.conf
   → Route definitions per service
   → SSL configuration
   ↓
5. Generate SSL certificates
   → Self-signed for dev/staging
   → Placeholder for production
   ↓
6. Generate service scaffolding
   → Copy templates for CS_1..CS_10
   → Replace placeholders
   → Preserve user modifications
   ↓
7. Generate monitoring configs
   → Prometheus scrape configs
   → Grafana dashboards
   → Loki pipeline
   ↓
8. Validation
   → Verify all files generated
   → Check file permissions
   → Test configuration syntax
```

## Examples

### Basic Build

```bash
nself build
```

**Output**:
```
→ Loading configuration from .env
✓ Configuration validated

→ Generating docker-compose.yml
  ✓ Core services: 4
  ✓ Optional services: 3 (Redis, MinIO, Admin)
  ✓ Custom services: 2 (api, worker)
  Total: 9 services

→ Generating Nginx configuration
  ✓ Main config: nginx/nginx.conf
  ✓ Routes: 7 routes configured
  ✓ SSL: Self-signed certificates

→ Generating service scaffolding
  ✓ api (Express.js) - services/api/
  ✓ worker (BullMQ) - services/worker/

✓ Build complete! (2.3s)

Next step:
  nself start
```

### Force Rebuild

```bash
nself build --force
```

**Use when**:
- Configuration significantly changed
- Generated files corrupted
- Need to reset to template defaults

**Warning**: Overwrites ALL generated files (preserves user services).

### Clean Build

```bash
nself build --clean
```

**Removes before building**:
- docker-compose.yml
- nginx/ directory
- ssl/ directory
- Monitoring configs

**Preserves**:
- services/ directory (user code)
- hasura/ directory (metadata)
- db/ directory (migrations/seeds)

### Validate Only

```bash
nself build --validate-only
```

**Output**:
```
→ Validating .env configuration

✓ Required variables present
✓ Port availability checked
✓ Service dependencies satisfied
✓ Domain format valid

Configuration valid ✓
```

## Idempotency and Preservation

### Safe to Run Multiple Times

The build command intelligently preserves user modifications:

**Always regenerated** (configuration-derived):
- docker-compose.yml
- nginx/nginx.conf
- nginx/sites/*.conf
- monitoring/*.yml

**Generated once, then preserved** (user-modifiable):
- services/[name]/src/ (user code)
- services/[name]/package.json (after first install)
- Custom scripts added by user

**Never overwritten**:
- .env (user configuration)
- hasura/metadata/ (Hasura state)
- db/migrations/ (user migrations)

### Example: Adding a Custom Service

```bash
# First build - creates scaffold
nself build
# Creates: services/api/src/index.js

# User modifies code
vi services/api/src/index.js

# Configuration change (add Redis)
echo "REDIS_ENABLED=true" >> .env

# Second build - preserves user code
nself build
# ✓ Adds Redis to docker-compose.yml
# ✓ Adds Redis route to Nginx
# ✓ Preserves services/api/src/index.js modifications
```

## Hot Reload vs Restart Required

### Hot Reload (No Restart)

These changes apply immediately after `nself build`:
- Nginx route additions (new services)
- SSL certificate updates
- Monitoring dashboard additions
- Custom service scaffolding (new services only)

**Apply without restart**:
```bash
nself build
# Changes take effect immediately for running services
```

### Restart Required

These changes require `nself restart`:
- Core service configuration (database, Hasura)
- Service enable/disable changes
- Port changes
- Environment variable changes

**Apply with restart**:
```bash
nself build
nself restart
```

## Configuration Validation

### Validation Checks

1. **Required Variables**
   ```
   ✓ PROJECT_NAME present
   ✓ BASE_DOMAIN present
   ✓ Database credentials present
   ✓ Hasura admin secret present
   ```

2. **Port Conflicts**
   ```
   ✓ 5432 (PostgreSQL) available
   ✓ 8080 (Hasura) available
   ✓ 4000 (Auth) available
   ✓ 80/443 (Nginx) available
   ```

3. **Service Dependencies**
   ```
   ✓ Redis enabled → Redis Exporter enabled (if monitoring)
   ✓ MLflow enabled → MinIO enabled (storage required)
   ✓ Functions enabled → MinIO enabled (storage required)
   ```

4. **Template Availability**
   ```
   ✓ CS_1 template exists: express-js
   ✓ CS_2 template exists: bullmq-js
   ```

### Validation Errors

**Missing required variable**:
```
✗ Configuration validation failed

Missing required variables:
  - PROJECT_NAME
  - HASURA_GRAPHQL_ADMIN_SECRET

Please set these in .env
```

**Port conflict**:
```
✗ Port conflict detected

Port 5432 already in use by: postgresql
  → Change POSTGRES_PORT in .env
  → Or stop conflicting service
```

**Invalid template**:
```
✗ Custom service error

CS_1=api:invalid-template:8001
Template 'invalid-template' not found

Available templates:
  - express-js, nestjs, fastify
  - python-fastapi, python-flask
  - go-gin, go-fiber
  - rust-axum, rust-actix
```

## Advanced Usage

### Custom Build Directory

```bash
BUILD_DIR=custom/path nself build
```

### Skip Validation

```bash
SKIP_VALIDATION=true nself build
# ⚠️ Not recommended - may generate invalid configs
```

### Debug Mode

```bash
nself build --verbose
```

**Shows**:
- Detailed template processing
- File-by-file generation
- Placeholder replacements
- Service dependency resolution

## Troubleshooting

### Build fails with syntax error

**Error**:
```
✗ docker-compose.yml: syntax error at line 42
```

**Solution**:
```bash
# Validate .env
nself build --validate-only

# Check for special characters in values
# Wrap values with spaces/special chars in quotes:
PROJECT_NAME="my app"  # Wrong
PROJECT_NAME="myapp"   # Right
```

### Custom service not generated

**Error**:
```
✗ services/api/ not created
```

**Check**:
```bash
# Verify .env has CS_N variable
grep "^CS_1=" .env

# Verify format
CS_1=api:express-js:8001
#    └─┬─┘ └───┬───┘ └─┬─┘
#    name  template  port

# Valid template?
ls src/templates/services/javascript/express-js/
```

### Nginx config invalid

**Error**:
```
✗ nginx: configuration file nginx.conf test failed
```

**Solution**:
```bash
# Test Nginx config
docker run --rm -v $PWD/nginx:/etc/nginx nginx nginx -t

# Check for syntax errors in nginx/nginx.conf
# Fix and rebuild
nself build --force
```

## Build Hooks

### Pre-Build Hook

```bash
# .nself-hooks/pre-build.sh
#!/bin/bash
echo "Running pre-build validations..."
# Custom validation logic
```

### Post-Build Hook

```bash
# .nself-hooks/post-build.sh
#!/bin/bash
echo "Build complete! Copying additional files..."
# Custom post-processing
```

**Enable hooks**:
```bash
ENABLE_HOOKS=true nself build
```

## Related Commands

- `nself init` - Create initial .env before building
- `nself start` - Start services after building
- `nself config show` - View current configuration
- `nself config validate` - Validate configuration only
- `nself doctor` - Diagnose build issues

## See Also

- [nself init](init.md)
- [nself start](start.md)
- [Configuration Reference](../../configuration/README.md)
- [Custom Services Guide](../../guides/CUSTOM-SERVICES.md)
- [Template System](../../guides/TEMPLATES.md)
