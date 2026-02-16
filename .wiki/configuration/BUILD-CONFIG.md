# Build Configuration Guide

Configure how nself builds your Docker Compose stack and generates service configurations.

## Overview

The `nself build` command:

1. Reads `.env` files
2. Validates configuration
3. Generates `docker-compose.yml`
4. Creates service configurations
5. Sets up Nginx routes
6. Generates SSL certificates

## Build Process

### Quick Build

```bash
nself build
```

This uses your current `.env` file and generates all configs.

### Clean Build

```bash
nself build --clean
```

Removes old generated files and builds fresh.

### Validate Only

```bash
nself build --validate
```

Checks configuration without generating files.

### Dry Run

```bash
nself build --dry-run
```

Shows what would be generated without actually creating files.

## Configuration Files

### .env.dev (Base Config)

The base configuration, committed to git. All developers use this as their starting point.

```bash
# Project Identification
PROJECT_NAME=my-app
ENV=dev
BASE_DOMAIN=localhost

# Required Services
POSTGRES_DB=myapp_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=change-me

# Hasura
HASURA_GRAPHQL_ADMIN_SECRET=change-me
HASURA_GRAPHQL_ENABLE_CONSOLE=true

# Server
NGINX_PORT=8080

# Optional Services
REDIS_ENABLED=false
MINIO_ENABLED=false
```

### .env.local (Developer Overrides)

Your local machine overrides. Gitignored, never committed.

```bash
# Override only what you need
POSTGRES_PASSWORD=my-local-password
HASURA_GRAPHQL_ADMIN_SECRET=my-local-secret
REDIS_ENABLED=true
```

## Service Configuration

### Enable Optional Services

Set `*_ENABLED=true` in `.env`:

```bash
# Cache & Sessions
REDIS_ENABLED=true
REDIS_PORT=6379
REDIS_MEMORY=256mb

# File Storage
MINIO_ENABLED=true
MINIO_PORT=9000
MINIO_ADMIN_USER=minioadmin
MINIO_ADMIN_PASSWORD=change-me

# Monitoring
MONITORING_ENABLED=true
```

### Custom Service Configuration

Add your own services using `CS_N`:

```bash
# Format: CS_N=service_name:template:port

# Express.js API
CS_1=api:express-js:8001

# Node.js Worker
CS_2=worker:bullmq-js:8002

# Python API
CS_3=ml-api:fastapi:8003

# Go gRPC Service
CS_4=grpc:go-grpc:8004
```

Available templates:
- **Node.js**: express-js, nestjs, fastify, koa
- **Python**: fastapi, flask, django
- **Go**: gin, echo, fiber, grpc
- **Java**: spring-boot, quarkus
- **Rust**: actix, rocket, axum
- And 30+ more...

### Frontend Applications

Route external applications (running outside Docker):

```bash
# Frontend App 1
FRONTEND_APP_1_NAME=web
FRONTEND_APP_1_PORT=3000
FRONTEND_APP_1_ROUTE=app

# Frontend App 2
FRONTEND_APP_2_NAME=admin
FRONTEND_APP_2_PORT=3001
FRONTEND_APP_2_ROUTE=admin
```

Result:
- `app.localhost:8080` → `localhost:3000`
- `admin.localhost:8080` → `localhost:3001`

## Generated Files

After `nself build`, the following files are created:

### docker-compose.yml

Main orchestration file defining all services:

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: myapp_db
    volumes:
      - postgres_data:/var/lib/postgresql/data

  hasura:
    image: hasura/graphql-engine:latest
    depends_on:
      - postgres
    environment:
      HASURA_GRAPHQL_DATABASE_URL: postgres://...
      HASURA_GRAPHQL_ADMIN_SECRET: change-me

  # ... more services
```

### .env.computed

Auto-generated computed values (do NOT edit):

```bash
# Generated database URL
DATABASE_URL=postgresql://postgres:password@postgres:5432/myapp_db

# Generated docker network name
DOCKER_NETWORK=my-app_network

# Generated service names
POSTGRES_HOST=postgres
HASURA_HOST=hasura
```

### nginx/

Nginx configuration files:

```
nginx/
├── nginx.conf              # Main config
├── includes/
│   ├── gzip.conf          # Compression
│   ├── security.conf      # Security headers
│   └── proxy.conf         # Proxy settings
├── sites/
│   ├── api.conf           # Hasura routing
│   ├── auth.conf          # Auth service routing
│   └── custom.conf        # Custom service routing
└── ssl/
    ├── cert.pem           # Self-signed certificate
    └── key.pem            # Private key
```

### services/

Generated custom service directories:

```
services/
├── api/                   # Generated from CS_1
│   ├── Dockerfile
│   ├── package.json
│   ├── src/
│   │   └── index.js
│   └── .dockerignore
├── worker/               # Generated from CS_2
│   ├── Dockerfile
│   ├── requirements.txt
│   └── app.py
```

## Build Options

### Docker Compose Version

Specify Docker Compose format version in `.env`:

```bash
# Default: 3.8 (recommended)
DOCKER_COMPOSE_VERSION=3.8
```

### Network Configuration

```bash
# Network name (auto-generated from PROJECT_NAME)
# my-app_network

# Change network driver
DOCKER_NETWORK_DRIVER=bridge

# Enable external network
DOCKER_NETWORK_EXTERNAL=false
```

### Volume Configuration

```bash
# Persistent data directory
VOLUMES_PATH=/var/lib/nself/volumes

# Database backup directory
BACKUP_PATH=/var/backups/nself

# Database size limit
POSTGRES_MAX_SIZE=100gb
```

## Port Management

### Nginx Port

```bash
# Default: 8080
NGINX_PORT=8080

# Custom port
NGINX_PORT=3000
```

### Service Ports

Each service has a default port (can be overridden):

```bash
# Redis port
REDIS_PORT=6379

# MinIO port
MINIO_PORT=9000

# Custom services
CS_1=api:express-js:8001
CS_2=worker:bullmq-js:8002
```

### Port Forwarding

nself binds services to `127.0.0.1` (localhost only) by default.

For external access, use Nginx routing:

```bash
# In .env
BASE_DOMAIN=myapp.com    # Routes via HTTPS

# Nginx automatically exposes:
# - hasura.myapp.com → Hasura
# - auth.myapp.com → Auth service
# - api.myapp.com → Custom services
```

## Build Customization

### Custom Dockerfile

For custom services, edit the generated Dockerfile:

```bash
# Generated at:
services/api/Dockerfile

# Edit it:
vim services/api/Dockerfile

# Rebuild
nself build
```

Generated files are **not overwritten** on subsequent builds (preserves your changes).

### Environment Variable Files

Build uses this cascade for environment variables:

1. `.env.dev` (committed, shared)
2. `.env.local` (on your machine)
3. `.env.staging` (on staging server only)
4. `.env.prod` (on production server only)
5. `.env.secrets` (ultra-sensitive, server only)

Later files override earlier ones.

### Docker Image Versions

Specify versions in `.env`:

```bash
# PostgreSQL version
POSTGRES_VERSION=15

# Hasura version
HASURA_VERSION=v2.40.0

# Redis version
REDIS_VERSION=7

# MinIO version
MINIO_VERSION=latest
```

## Build Validation

### Check Configuration

```bash
nself config validate
```

Validates:
- All required variables are set
- Port conflicts detected
- Service dependencies valid
- Custom service templates exist

### Dry Run Build

```bash
nself build --dry-run
```

Shows exactly what would be generated without creating files.

### View Generated Config

```bash
# Show docker-compose.yml
cat docker-compose.yml

# Show Nginx configuration
cat nginx/nginx.conf

# View computed variables
cat .env.computed
```

## Build Error Resolution

### Port Already in Use

```
Error: Port 8080 already in use
```

**Solution**: Change Nginx port in `.env`:

```bash
NGINX_PORT=8081
nself build
nself start
```

### Service Dependency Error

```
Error: Service 'postgres' not found
```

**Solution**: Make sure `POSTGRES_DB` is set in `.env`.

### Custom Service Template Not Found

```
Error: Template 'express-js' not found
```

**Solution**: Use `nself templates list` to see available templates.

### Environment Variable Not Set

```
Error: Required variable HASURA_GRAPHQL_ADMIN_SECRET not set
```

**Solution**: Add the variable to `.env`:

```bash
HASURA_GRAPHQL_ADMIN_SECRET=your-secret-here
nself build
```

## Build Performance

### Faster Builds

```bash
# Build specific services only
nself build --only postgres,hasura

# Skip validation
nself build --skip-validate

# Parallel build (use all CPU cores)
nself build --parallel
```

### Large File Handling

If you have large custom services:

```bash
# Ignore large files in build
DOCKER_IGNORE_PATHS='node_modules,venv,.git'
nself build
```

## Troubleshooting

### Build Takes Too Long

1. Check disk space: `df -h`
2. Check Docker: `docker system df`
3. Clean unused images: `docker system prune`

### Generated Files Missing

```bash
# Verify build completed successfully
nself build --verbose

# Rebuild from scratch
rm docker-compose.yml
nself build --clean
```

### Nginx Not Routing Correctly

```bash
# Check generated nginx config
cat nginx/sites/*.conf

# Verify service names
docker ps | grep my-app

# Test connection
curl -v http://localhost:8080/graphql
```

## Next Steps

- [Cascading Configuration](../configuration/CASCADING-OVERRIDES.md) - Environment management
- [Start Services](../commands/START.md) - Launch your stack
- [Deployment](../guides/DEPLOYMENT-ARCHITECTURE.md) - Production deployment

---

**Key Takeaway**: `nself build` generates production-ready Docker Compose configurations from your `.env` files. Customize before building, then the generated files are yours to modify as needed.
