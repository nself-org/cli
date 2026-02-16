#!/usr/bin/env bash

# variables-documentation.sh - Generate documentation for runtime variables

# Generate runtime variables documentation
generate_runtime_variables_doc() {

set -euo pipefail

  local output_file="${1:-RUNTIME_VARIABLES.md}"

  cat >"$output_file" <<'EOF'
# Runtime Variables Reference

This file documents all runtime variables used in the generated configuration files.
These variables allow the same build artifacts to work across dev, staging, and production environments.

## Overview

All runtime variables follow the pattern: `${VARIABLE_NAME:-default_value}`

This allows environment-specific configuration without rebuilding.

## Core Variables

### Project Configuration
- `${PROJECT_NAME}` - Project identifier used for container names
- `${ENV:-dev}` - Environment (dev/staging/prod)
- `${BASE_DOMAIN:-localhost}` - Base domain for all services
- `${DOCKER_NETWORK}` - Docker network name

### SSL/TLS Configuration
- `${SSL_CERT_NAME:-localhost}` - SSL certificate directory name
- `${SSL_ENABLED:-true}` - Enable/disable SSL

## Service-Specific Variables

### PostgreSQL Database
- `${POSTGRES_PORT:-5432}` - PostgreSQL port
- `${POSTGRES_USER:-postgres}` - Database user
- `${POSTGRES_PASSWORD:-postgres}` - Database password
- `${POSTGRES_DB:-${PROJECT_NAME}}` - Database name
- `${POSTGRES_MAX_CONNECTIONS:-200}` - Max connections

### Hasura GraphQL Engine
- `${HASURA_PORT:-8080}` - Hasura port
- `${HASURA_ROUTE:-api}` - Nginx route subdomain
- `${HASURA_GRAPHQL_ADMIN_SECRET}` - Admin secret
- `${HASURA_GRAPHQL_JWT_SECRET}` - JWT secret
- `${HASURA_GRAPHQL_UNAUTHORIZED_ROLE:-anonymous}` - Default role
- `${HASURA_DEV_MODE:-false}` - Development mode
- `${HASURA_LOG_LEVEL:-info}` - Log level

### Authentication Service
- `${AUTH_PORT:-4000}` - Auth service port
- `${AUTH_ROUTE:-auth}` - Nginx route subdomain
- `${AUTH_JWT_SECRET:-${JWT_SECRET:-change-me}}` - JWT secret
- `${AUTH_SERVER_URL:-http://localhost:4000}` - Server URL
- `${AUTH_CLIENT_URL:-http://localhost:3000}` - Client URL
- `${AUTH_SMTP_HOST:-mailpit}` - SMTP host
- `${AUTH_SMTP_PORT:-1025}` - SMTP port
- `${AUTH_SMTP_SECURE:-false}` - SMTP SSL
- `${AUTH_SMTP_SENDER:-noreply@${BASE_DOMAIN:-localhost}}` - From address
- `${AUTH_ACCESS_TOKEN_EXPIRES_IN:-900}` - Access token TTL
- `${AUTH_REFRESH_TOKEN_EXPIRES_IN:-2592000}` - Refresh token TTL
- `${AUTH_LOG_LEVEL:-info}` - Log level

### Nginx
- `${NGINX_PORT:-80}` - HTTP port
- `${NGINX_SSL_PORT:-443}` - HTTPS port
- `${NGINX_MAX_BODY_SIZE:-100M}` - Max upload size

### Optional Services

#### Redis
- `${REDIS_PORT:-6379}` - Redis port
- `${REDIS_PASSWORD}` - Redis password (optional)
- `${REDIS_VERSION:-7-alpine}` - Docker image version

#### MinIO Object Storage
- `${MINIO_PORT:-9000}` - MinIO API port
- `${MINIO_CONSOLE_PORT:-9001}` - Console port
- `${MINIO_ROOT_USER:-minioadmin}` - Root username
- `${MINIO_ROOT_PASSWORD:-minioadmin}` - Root password
- `${MINIO_DEFAULT_BUCKETS:-uploads}` - Default buckets to create
- `${MINIO_REGION:-us-east-1}` - AWS region
- `${STORAGE_ROUTE:-storage}` - Nginx route for S3 API
- `${STORAGE_CONSOLE_ROUTE:-storage-console}` - Console route

#### nself Admin
- `${ADMIN_ROUTE:-admin}` - Nginx route subdomain
- `${NSELF_ADMIN_VERSION:-latest}` - Docker image version

#### Functions Runtime
- `${FUNCTIONS_PORT:-3008}` - Functions port
- `${FUNCTIONS_ROUTE:-functions}` - Nginx route

#### MLflow
- `${MLFLOW_PORT:-5000}` - MLflow port
- `${MLFLOW_ROUTE:-mlflow}` - Nginx route
- `${MLFLOW_BACKEND_STORE_URI}` - Backend store URI
- `${MLFLOW_ARTIFACT_ROOT}` - Artifact storage

#### Mailpit
- `${MAILPIT_SMTP_PORT:-1025}` - SMTP port
- `${MAILPIT_UI_PORT:-8025}` - Web UI port

#### MeiliSearch
- `${MEILISEARCH_PORT:-7700}` - API port
- `${MEILISEARCH_MASTER_KEY}` - Master key

### Monitoring Stack

#### Prometheus
- `${PROMETHEUS_PORT:-9090}` - Prometheus port
- `${PROMETHEUS_ROUTE:-prometheus}` - Nginx route
- `${PROMETHEUS_RETENTION_TIME:-15d}` - Data retention

#### Grafana
- `${GRAFANA_PORT:-3000}` - Grafana port (internal)
- `${GRAFANA_ROUTE:-grafana}` - Nginx route
- `${GRAFANA_ADMIN_USER:-admin}` - Admin username
- `${GRAFANA_ADMIN_PASSWORD:-admin}` - Admin password
- `${GRAFANA_PLUGINS:-}` - Additional plugins

#### Loki
- `${LOKI_PORT:-3100}` - Loki port
- `${LOKI_RETENTION_PERIOD:-168h}` - Log retention

#### Tempo
- `${TEMPO_PORT:-3200}` - Tempo port

#### Alertmanager
- `${ALERTMANAGER_PORT:-9093}` - Alertmanager port

#### Exporters
- `${CADVISOR_PORT:-8080}` - cAdvisor port
- `${NODE_EXPORTER_PORT:-9100}` - Node exporter port
- `${POSTGRES_EXPORTER_PORT:-9187}` - PostgreSQL exporter port
- `${REDIS_EXPORTER_PORT:-9121}` - Redis exporter port

### Custom Services

Custom services use the CS_N pattern:
- `${CS_N_ROUTE:-service_name}` - Nginx route for custom service N
- `${CS_N_PORT}` - Port for custom service N
- `${CS_N_MEMORY:-512M}` - Memory limit
- `${CS_N_CPU:-0.5}` - CPU limit
- `${CS_N_REPLICAS:-1}` - Number of replicas

### Frontend Applications

Frontend apps use the FRONTEND_APP_N pattern:
- `${FRONTEND_APP_N_ROUTE:-appN}` - Nginx route for frontend app N
- `${FRONTEND_APP_N_PORT:-3000}` - Port for frontend app N
- `${FRONTEND_APP_N_API_ROUTE:-api.appN}` - API route for remote schema
- `${FRONTEND_APP_N_API_PORT:-4001}` - API port for remote schema

## Environment-Specific Examples

### Development
```bash
BASE_DOMAIN=localhost
FRONTEND_APP_1_ROUTE=app1
# Results in: app1.localhost
```

### Staging
```bash
BASE_DOMAIN=staging.example.com
FRONTEND_APP_1_ROUTE=app1
# Results in: app1.staging.example.com
```

### Production
```bash
BASE_DOMAIN=example.com
FRONTEND_APP_1_ROUTE=www  # Override default
# Results in: www.example.com
```

## Usage

1. Build once: `nself build`
2. Set environment variables for your target environment
3. Start services: `nself start`

The same docker-compose.yml and nginx configurations work across all environments!

## Total Runtime Variables

A full demo configuration uses approximately **76 runtime variables**, enabling complete flexibility without rebuilding.

---
Generated by nself build
EOF

  echo "✓ Runtime variables documentation generated: $output_file"
}

# Export function
export -f generate_runtime_variables_doc
