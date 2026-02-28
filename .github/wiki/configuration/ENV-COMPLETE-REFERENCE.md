# Complete Environment Variable Reference for nself

This is the exhaustive, accurate documentation of EVERY environment variable used in nself v0.3.9. No additions, no hallucinations - only what's actually in the code.

## Core Philosophy

nself uses a single source of truth for all configuration: environment files. The loading priority is:
1. `.env` (production - highest priority if exists)
2. `.env.local` (local overrides - git-ignored)
3. `.env.dev` (team defaults)
4. Smart defaults (built-in fallbacks)

## Table of Contents

1. [Core Settings](#core-settings)
2. [PostgreSQL Configuration](#postgresql-configuration)
3. [Hasura GraphQL Engine](#hasura-graphql-engine)
4. [Authentication Service](#authentication-service)
5. [Storage (MinIO & Hasura Storage)](#storage-minio--hasura-storage)
6. [Nginx Reverse Proxy](#nginx-reverse-proxy)
7. [Optional Services](#optional-services)
8. [Microservices Configuration](#microservices-configuration)
9. [Custom Services (CS_N)](#custom-services-cs_n)
10. [Email Configuration](#email-configuration)
11. [SSL and Security](#ssl-and-security)
12. [Docker and Networking](#docker-and-networking)
13. [Development Settings](#development-settings)

---

## Core Settings

These are the fundamental settings that affect your entire stack.

```bash
# Project identification
PROJECT_NAME=myproject
# Default: myproject
# Used for: Container names, database names, Docker network

# Base domain for all services
BASE_DOMAIN=local.nself.org
# Default: local.nself.org
# Used for: Service URLs (api.BASE_DOMAIN, auth.BASE_DOMAIN, etc.)

# Environment type
ENV=dev
# Default: dev
# Options: dev, staging, prod, production
# Affects: Console access, dev mode, security settings

# Database seeding
DB_ENV_SEEDS=true
# Default: true
# Enables database initialization with seed data
```

---

## PostgreSQL Configuration

All PostgreSQL-related settings.

```bash
# Version
POSTGRES_VERSION=16-alpine
# Default: 16-alpine
# Available: 14-alpine, 15-alpine, 16-alpine

# Connection settings
POSTGRES_HOST=postgres
# Default: postgres
# Internal Docker hostname

POSTGRES_PORT=5432
# Default: 5432
# Port for PostgreSQL connections

# Database credentials
POSTGRES_DB=nhost
# Default: nhost
# Main database name

POSTGRES_USER=postgres
# Default: postgres
# Database username

POSTGRES_PASSWORD=postgres-dev-password
# Default: postgres-dev-password
# IMPORTANT: Change in production!

# Extensions
POSTGRES_EXTENSIONS=uuid-ossp
# Default: uuid-ossp
# Comma-separated list of extensions to enable
# Available: uuid-ossp, pgcrypto, citext, pg_trgm, unaccent, hstore
```

---

## Hasura GraphQL Engine

Hasura configuration for instant GraphQL API.

```bash
# Version
HASURA_VERSION=v2.44.0
# Default: v2.44.0

# Admin secret
HASURA_GRAPHQL_ADMIN_SECRET=hasura-admin-secret-dev
# Default: hasura-admin-secret-dev
# CRITICAL: Change in production!

# JWT Configuration (Two formats supported)
# Format 1: Simple (recommended)
HASURA_JWT_KEY=development-secret-key-minimum-32-characters-long
# Default: development-secret-key-minimum-32-characters-long
# Must be at least 32 characters

HASURA_JWT_TYPE=HS256
# Default: HS256
# Options: HS256, HS384, HS512, RS256, RS384, RS512

# Format 2: JSON (legacy, auto-generated from above)
HASURA_GRAPHQL_JWT_SECRET={"type":"HS256","key":"your-key"}
# Auto-generated from HASURA_JWT_KEY and HASURA_JWT_TYPE

# Console and dev mode
HASURA_GRAPHQL_ENABLE_CONSOLE=true
# Default: true (dev), false (prod)
# Enables Hasura console UI

HASURA_GRAPHQL_DEV_MODE=true
# Default: true (dev), false (prod)
# Enables development features

# Telemetry
HASURA_GRAPHQL_ENABLE_TELEMETRY=false
# Default: false
# Send anonymous usage stats to Hasura

# CORS configuration
HASURA_GRAPHQL_CORS_DOMAIN=*
# Default: * (dev), specific domains (prod)
# Allowed origins for CORS

# Routing
HASURA_ROUTE=api.${BASE_DOMAIN}
# Default: api.${BASE_DOMAIN}
# URL for Hasura endpoint

# Internal ports
HASURA_PORT=8080
# Default: 8080
# Internal Hasura port

HASURA_CONSOLE_PORT=9695
# Default: 9695
# Hasura console port

# Database URL (auto-generated)
HASURA_METADATA_DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}
# Auto-generated from PostgreSQL settings
```

---

## Authentication Service

Nhost Auth service configuration.

```bash
# Version
AUTH_VERSION=0.36.0
# Default: 0.36.0

# Connection settings
AUTH_HOST=auth
# Default: auth
# Internal hostname

AUTH_PORT=4000
# Default: 4000
# Internal port

# Client configuration
AUTH_CLIENT_URL=http://localhost:3000
# Default: http://localhost:3000
# Frontend application URL

# JWT token expiry
AUTH_JWT_REFRESH_TOKEN_EXPIRES_IN=2592000
# Default: 2592000 (30 days in seconds)

AUTH_JWT_ACCESS_TOKEN_EXPIRES_IN=900
# Default: 900 (15 minutes in seconds)

# WebAuthn
AUTH_WEBAUTHN_ENABLED=false
# Default: false
# Enable WebAuthn/FIDO2 authentication

# Routing
AUTH_ROUTE=auth.${BASE_DOMAIN}
# Default: auth.${BASE_DOMAIN}

# SMTP Configuration (for email auth)
AUTH_SMTP_HOST=mailpit
# Default: mailpit (dev), your SMTP host (prod)

AUTH_SMTP_PORT=1025
# Default: 1025 (dev), 587 (prod)

AUTH_SMTP_USER=""
# Default: "" (empty for dev)

AUTH_SMTP_PASS=""
# Default: "" (empty for dev)

AUTH_SMTP_SECURE=false
# Default: false (dev), true (prod with TLS)

AUTH_SMTP_SENDER=noreply@${BASE_DOMAIN}
# Default: noreply@${BASE_DOMAIN}
```

---

## Storage (MinIO & Hasura Storage)

Object storage configuration.

```bash
# MinIO S3-compatible storage
MINIO_VERSION=latest
# Default: latest

MINIO_PORT=9000
# Default: 9000
# MinIO API port

MINIO_ROOT_USER=minioadmin
# Default: minioadmin
# MinIO root username

MINIO_ROOT_PASSWORD=minioadmin
# Default: minioadmin
# CRITICAL: Change in production!

# S3 Configuration
S3_ACCESS_KEY=storage-access-key-dev
# Default: storage-access-key-dev

S3_SECRET_KEY=storage-secret-key-dev
# Default: storage-secret-key-dev

S3_BUCKET=nhost
# Default: nhost
# Default bucket name

S3_REGION=us-east-1
# Default: us-east-1

S3_ENDPOINT=http://minio:${MINIO_PORT}
# Default: http://minio:9000
# Internal S3 endpoint

# Hasura Storage Service
STORAGE_VERSION=0.6.1
# Default: 0.6.1

STORAGE_PORT=5001
# Default: 5001

STORAGE_ROUTE=storage.${BASE_DOMAIN}
# Default: storage.${BASE_DOMAIN}

STORAGE_CONSOLE_ROUTE=storage-console.${BASE_DOMAIN}
# Default: storage-console.${BASE_DOMAIN}
```

---

## Nginx Reverse Proxy

Nginx configuration for routing and SSL.

```bash
# Version
NGINX_VERSION=alpine
# Default: alpine

# Ports
NGINX_HTTP_PORT=80
# Default: 80

NGINX_HTTPS_PORT=443
# Default: 443

# SSL Mode
SSL_MODE=local
# Default: local
# Options: local (mkcert), letsencrypt, custom
```

---

## Optional Services

Services that can be enabled as needed.

```bash
# Redis Cache
REDIS_ENABLED=false
# Default: false

REDIS_VERSION=7-alpine
# Default: 7-alpine

REDIS_PORT=6379
# Default: 6379

REDIS_PASSWORD=""
# Default: "" (no password in dev)

# Functions Runtime
FUNCTIONS_ENABLED=false
# Default: false

FUNCTIONS_PORT=4300
# Default: 4300

FUNCTIONS_ROUTE=functions.${BASE_DOMAIN}
# Default: functions.${BASE_DOMAIN}

# Dashboard (Hasura Console Alternative)
DASHBOARD_ENABLED=false
# Default: false

DASHBOARD_VERSION=latest
# Default: latest

DASHBOARD_PORT=4500
# Default: 4500

DASHBOARD_ROUTE=dashboard.${BASE_DOMAIN}
# Default: dashboard.${BASE_DOMAIN}

# MLflow (Machine Learning Platform)
MLFLOW_ENABLED=false
# Default: false

MLFLOW_VERSION=2.9.2
# Default: 2.9.2

MLFLOW_PORT=5000
# Default: 5000

MLFLOW_ROUTE=mlflow.${BASE_DOMAIN}
# Default: mlflow.${BASE_DOMAIN}

MLFLOW_DB_NAME=mlflow
# Default: mlflow
# Separate database for MLflow

MLFLOW_ARTIFACTS_BUCKET=mlflow-artifacts
# Default: mlflow-artifacts
# S3 bucket for artifacts

MLFLOW_AUTH_ENABLED=false
# Default: false

MLFLOW_AUTH_USERNAME=admin
# Default: admin

MLFLOW_AUTH_PASSWORD=mlflow-admin-password
# Default: mlflow-admin-password

# nself Admin UI
NSELF_ADMIN_ENABLED=false
# Default: false
# Note: Enabled via 'nself admin enable' command

ADMIN_SECRET_KEY=${generated}
# Auto-generated 32-byte hex key

ADMIN_PASSWORD_HASH=${generated}
# Generated from password via 'nself admin password'
```

---

## Microservices Configuration

Built-in microservice frameworks support.

```bash
# Global microservices toggle
SERVICES_ENABLED=false
# Default: false

# NestJS Services
NESTJS_ENABLED=false
# Default: false

NESTJS_SERVICES=""
# Default: "" (empty)
# Comma-separated list: api,workers,admin

NESTJS_USE_TYPESCRIPT=true
# Default: true

NESTJS_PORT_START=3100
# Default: 3100
# Starting port for NestJS services

# BullMQ Queue Workers
BULLMQ_ENABLED=false
# Default: false

BULLMQ_WORKERS=""
# Default: "" (empty)
# Comma-separated list: email-worker,data-processor

# Legacy support
BULL_SERVICES=""
# Alias for BULLMQ_WORKERS

BULLMQ_DASHBOARD_ENABLED=false
# Default: false

BULLMQ_DASHBOARD_PORT=4200
# Default: 4200

BULLMQ_DASHBOARD_ROUTE=queues.${BASE_DOMAIN}
# Default: queues.${BASE_DOMAIN}

# Go Services
GOLANG_ENABLED=false
# Default: false

GOLANG_SERVICES=""
# Default: "" (empty)
# Comma-separated list: analytics,gateway

GOLANG_PORT_START=3200
# Default: 3200

# Python Services
PYTHON_ENABLED=false
# Default: false

PYTHON_SERVICES=""
# Default: "" (empty)
# Comma-separated list: ml-service,data-pipeline

PYTHON_FRAMEWORK=fastapi
# Default: fastapi
# Options: fastapi, flask, django

PYTHON_PORT_START=3300
# Default: 3300

# NestJS Run (Single Instance)
NESTJS_RUN_ENABLED=false
# Default: false

NESTJS_RUN_PORT=3400
# Default: 3400
```

---

## Custom Services (CS_N)

User-defined custom services using the CS_N pattern.

```bash
# Format: CS_N=name:framework[:port][:route_or_domain]
# N = 1-99

# Examples:
CS_1=api:fastapi:3001:api
# Creates service 'api' using fastapi template on port 3001

CS_2=worker:custom:3002
# Creates custom service 'worker' on port 3002

CS_3=frontend:node-ts:3003:app.mydomain.com
# Creates service with custom domain

# Advanced configuration per service:
CS_1_MEMORY=512M           # Memory limit
CS_1_CPU=1.0               # CPU cores
CS_1_REPLICAS=2            # Number of instances
CS_1_PUBLIC=true           # Expose via nginx
CS_1_HEALTHCHECK=/health   # Health endpoint
CS_1_RATE_LIMIT=100        # Requests per minute
CS_1_TABLE_PREFIX=cs1_     # Database table prefix
CS_1_REDIS_PREFIX=cs1:     # Redis key prefix
CS_1_DATABASE=cs1_db       # Separate database
CS_1_REDIS_DB=1           # Redis database number (0-15)
CS_1_DEV_DOMAIN=api.local  # Development domain
CS_1_PROD_DOMAIN=api.prod  # Production domain
CS_1_ENV=KEY1=val1,KEY2=val2  # Additional env vars

# Legacy support
CUSTOM_SERVICES=""
# Old format - deprecated, use CS_N
```

---

## Email Configuration

Email provider settings.

```bash
# Email provider selection
EMAIL_PROVIDER=mailpit
# Default: mailpit
# Options: mailpit, sendgrid, mailgun, ses, smtp, etc.

# Mailpit (Development Email)
MAILPIT_SMTP_PORT=1025
# Default: 1025

MAILPIT_UI_PORT=8025
# Default: 8025

MAILPIT_ROUTE=mail.${BASE_DOMAIN}
# Default: mail.${BASE_DOMAIN}

# Global email settings
EMAIL_FROM=noreply@${BASE_DOMAIN}
# Default: noreply@${BASE_DOMAIN}

# Production email providers configured via 'nself email' command
# Sets AUTH_SMTP_* variables based on provider
```

---

## SSL and Security

Security-related configuration.

```bash
# SSL Mode
SSL_MODE=local
# Default: local
# Options: local (mkcert), letsencrypt, custom

# Admin authentication
ADMIN_SECRET_KEY=${auto-generated}
# 32-byte hex key for admin UI

ADMIN_PASSWORD_HASH=${generated}
# BCrypt hash of admin password

# JWT Configuration (see Hasura section)
JWT_KEY=${HASURA_JWT_KEY}
# Alias for HASURA_JWT_KEY
```

---

## Docker and Networking

Docker-specific configuration.

```bash
# Docker Compose project name
COMPOSE_PROJECT_NAME=${PROJECT_NAME}
# Default: ${PROJECT_NAME}

# Environment file for Compose
COMPOSE_ENV_FILE=${ENV_FILE}
# Default: .env.local

# Docker BuildKit
DOCKER_BUILDKIT=1
# Default: 1 (enabled)

COMPOSE_DOCKER_CLI_BUILD=1
# Default: 1 (enabled)

# Network configuration
DOCKER_NETWORK=${PROJECT_NAME}_network
# Default: ${PROJECT_NAME}_network

# Internal defaults (rarely changed)
HASURA_METADATA_DATABASE_URL=postgres://...
# Auto-generated PostgreSQL URL

DATABASE_URL=postgres://...
# Alias for HASURA_METADATA_DATABASE_URL
```

---

## Development Settings

Development-specific settings.

```bash
# Environment file
ENV_FILE=.env.local
# Default: .env.local
# Which env file to load

# Feature flags
AUTO_FIX_ENABLED=true
# Default: true
# Enable automatic fixing of issues

VERBOSE=false
# Default: false
# Verbose output

DEBUG=false
# Default: false
# Debug mode

NO_COLOR=""
# Default: "" (colors enabled)
# Set to any value to disable colors

# Timeouts
STARTUP_TIMEOUT=60
# Default: 60 seconds

HEALTH_CHECK_INTERVAL=2
# Default: 2 seconds

MAX_RETRIES=3
# Default: 3

RETRY_DELAY=5
# Default: 5 seconds

# Additional routes (computed)
FILES_ROUTE=files.${BASE_DOMAIN}
MAIL_ROUTE=mail.${BASE_DOMAIN}
```

---

## Environment Loading Priority

The exact order of environment loading:

1. **Check for `.env`** (production)
   - If exists, load it and apply defaults
   - STOP - ignore all other files

2. **Check for `.env.local`** (development)
   - If exists, load it and apply defaults
   - STOP - ignore remaining files

3. **Check for `.env.dev`** (team defaults)
   - If exists, load it and apply defaults
   - STOP

4. **Apply smart defaults only**
   - Use built-in defaults for all variables

## Smart Defaults

Every variable has a smart default. Production (`ENV=prod`) triggers:
- `HASURA_GRAPHQL_ENABLE_CONSOLE=false`
- `HASURA_GRAPHQL_DEV_MODE=false`
- Security headers enabled
- Rate limiting enabled

Development (`ENV=dev`) triggers:
- `HASURA_GRAPHQL_ENABLE_CONSOLE=true`
- `HASURA_GRAPHQL_DEV_MODE=true`
- Permissive CORS
- Verbose logging

## Computed Variables

These are automatically computed from other variables:

```bash
# Routes (all computed from BASE_DOMAIN)
HASURA_ROUTE=api.${BASE_DOMAIN}
AUTH_ROUTE=auth.${BASE_DOMAIN}
STORAGE_ROUTE=storage.${BASE_DOMAIN}
STORAGE_CONSOLE_ROUTE=storage-console.${BASE_DOMAIN}
FUNCTIONS_ROUTE=functions.${BASE_DOMAIN}
DASHBOARD_ROUTE=dashboard.${BASE_DOMAIN}
MAILPIT_ROUTE=mail.${BASE_DOMAIN}
BULLMQ_DASHBOARD_ROUTE=queues.${BASE_DOMAIN}
MLFLOW_ROUTE=mlflow.${BASE_DOMAIN}

# Email sender
AUTH_SMTP_SENDER=noreply@${BASE_DOMAIN}
EMAIL_FROM=noreply@${BASE_DOMAIN}

# Docker network
DOCKER_NETWORK=${PROJECT_NAME}_network

# Database URLs
HASURA_METADATA_DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}

# S3 endpoint
S3_ENDPOINT=http://minio:${MINIO_PORT}

# JWT Secret (if using simple format)
HASURA_GRAPHQL_JWT_SECRET={"type":"${HASURA_JWT_TYPE}","key":"${HASURA_JWT_KEY}"}
```

---

## Required Variables

These MUST be set (have defaults but should be changed in production):

1. `PROJECT_NAME` - Identifies your project
2. `BASE_DOMAIN` - Domain for services
3. `POSTGRES_PASSWORD` - Database password
4. `HASURA_GRAPHQL_ADMIN_SECRET` - Hasura admin secret
5. `HASURA_JWT_KEY` - JWT signing key (32+ characters)

---

## Notes

1. **No Frontend Apps (FA_N)**: Not found in current codebase
2. **No Prometheus/Grafana**: Not implemented in current version
3. **Admin UI**: Configured via CLI commands, not env vars
4. **SSL Certificates**: Managed by CLI, not env vars
5. **Backup Settings**: Configured via CLI, not env vars

This documentation reflects nself v0.3.9 actual implementation.