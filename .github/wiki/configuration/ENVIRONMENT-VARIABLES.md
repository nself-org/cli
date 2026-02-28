# Environment Variables Complete Reference

## Overview

This document provides a complete reference for all environment variables supported by nself v0.3.9+.

## Environment File Loading Order

Files are loaded in this specific cascade (later overrides earlier):

1. `.env.dev` - Team development defaults (always loaded first)
2. `.env.staging` - Staging overrides (only if ENV=staging)
3. `.env.prod` - Production config (only if ENV=prod)
4. `.env.secrets` - Sensitive production data (only if ENV=prod)
5. `.env` - Local overrides (ALWAYS loaded last, highest priority)

## Core Configuration

### Project Settings

| Variable | Default | Description | Required |
|----------|---------|-------------|----------|
| `PROJECT_NAME` | `my-app` | Project identifier (alphanumeric, hyphens) | Yes |
| `BASE_DOMAIN` | `local.nself.org` | Base domain for services | Yes |
| `ENV` | `dev` | Environment (dev, staging, prod) | Yes |
| `PROJECT_DESCRIPTION` | `""` | Project description | No |
| `ADMIN_EMAIL` | `admin@${BASE_DOMAIN}` | Administrator email | No |
| `AUTO_FIX` | `true` | Enable auto-fix for configuration issues | No |
| `DEBUG` | `false` | Enable debug logging | No |
| `VERBOSE` | `false` | Enable verbose output | No |

### Service Enable Flags

All core services default to `true` for backward compatibility:

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_ENABLED` | `true` | Enable PostgreSQL database |
| `HASURA_ENABLED` | `true` | Enable Hasura GraphQL |
| `AUTH_ENABLED` | `true` | Enable authentication service |
| `STORAGE_ENABLED` | `true` | Enable storage service (MinIO) |
| `NSELF_ADMIN_ENABLED` | `false` | Enable admin UI |
| `REDIS_ENABLED` | `false` | Enable Redis cache |
| `FUNCTIONS_ENABLED` | `false` | Enable serverless functions |
| `SEARCH_ENABLED` | `false` | Enable search service |
| `SERVICES_ENABLED` | `false` | Enable custom microservices |

### Monitoring Services

All monitoring services default to `false`:

| Variable | Default | Description |
|----------|---------|-------------|
| `PROMETHEUS_ENABLED` | `false` | Enable Prometheus metrics |
| `GRAFANA_ENABLED` | `false` | Enable Grafana dashboards |
| `LOKI_ENABLED` | `false` | Enable Loki log aggregation |
| `VECTOR_ENABLED` | `false` | Enable Vector log shipping |
| `TEMPO_ENABLED` | `false` | Enable Tempo tracing |
| `JAEGER_ENABLED` | `false` | Enable Jaeger tracing |

## Service Configuration

### PostgreSQL Database

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_VERSION` | `16-alpine` | PostgreSQL Docker image version |
| `POSTGRES_PORT` | `5432` | PostgreSQL port |
| `POSTGRES_DB` | `${PROJECT_NAME}` | Database name |
| `POSTGRES_USER` | `postgres` | Database user |
| `POSTGRES_PASSWORD` | `<generated>` | Database password |
| `POSTGRES_HOST` | `postgres` | Database hostname |
| `POSTGRES_SSL_MODE` | `disable` | SSL mode (disable, require, verify-ca, verify-full) |
| `POSTGRES_MAX_CONNECTIONS` | `100` | Maximum connections |
| `POSTGRES_SHARED_BUFFERS` | `256MB` | Shared buffer size |
| `POSTGRES_EFFECTIVE_CACHE_SIZE` | `1GB` | Effective cache size |
| `POSTGRES_WORK_MEM` | `4MB` | Work memory per operation |
| `POSTGRES_MAINTENANCE_WORK_MEM` | `64MB` | Maintenance work memory |
| `POSTGRES_EXTENSIONS` | `<60+ extensions>` | Enabled PostgreSQL extensions |

### Hasura GraphQL

| Variable | Default | Description |
|----------|---------|-------------|
| `HASURA_VERSION` | `v2.44.0` | Hasura Docker image version |
| `HASURA_GRAPHQL_PORT` | `8080` | GraphQL API port |
| `HASURA_CONSOLE_PORT` | `3000` | Console port |
| `HASURA_GRAPHQL_ADMIN_SECRET` | `<generated>` | Admin secret key |
| `HASURA_GRAPHQL_JWT_SECRET` | `<generated>` | JWT secret configuration |
| `HASURA_GRAPHQL_DATABASE_URL` | `<computed>` | PostgreSQL connection URL |
| `HASURA_GRAPHQL_ENABLE_CONSOLE` | `true` | Enable Hasura console |
| `HASURA_GRAPHQL_ENABLE_TELEMETRY` | `false` | Enable telemetry |
| `HASURA_GRAPHQL_DEV_MODE` | `true` | Development mode |
| `HASURA_GRAPHQL_CORS_DOMAIN` | `*` | CORS allowed domains |
| `HASURA_GRAPHQL_UNAUTHORIZED_ROLE` | `public` | Default unauthorized role |
| `HASURA_GRAPHQL_ENABLE_ALLOWLIST` | `false` | Enable query allowlist |
| `HASURA_GRAPHQL_ENABLE_REMOTE_SCHEMA_PERMISSIONS` | `true` | Remote schema permissions |
| `HASURA_PROJECT_DIR` | `hasura` | Path to the Hasura project directory (contains config.yaml and metadata/). Override if your hasura directory is not at the default `hasura/` path. |

### Authentication Service

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_VERSION` | `v0.36.0` | Nhost Auth version |
| `AUTH_PORT` | `4000` | Auth service port |
| `AUTH_CLIENT_URL` | `http://localhost:3000` | Client application URL |
| `AUTH_SMTP_HOST` | `mailpit` | SMTP hostname |
| `AUTH_SMTP_PORT` | `1025` | SMTP port |
| `AUTH_SMTP_USER` | `""` | SMTP username |
| `AUTH_SMTP_PASS` | `""` | SMTP password |
| `AUTH_SMTP_SECURE` | `false` | Use TLS for SMTP |
| `AUTH_SMTP_SENDER` | `noreply@${BASE_DOMAIN}` | Email sender address |
| `AUTH_JWT_SECRET` | `<generated>` | JWT signing secret |
| `AUTH_ACCESS_TOKEN_EXPIRES_IN` | `900` | Access token expiry (seconds) |
| `AUTH_REFRESH_TOKEN_EXPIRES_IN` | `2592000` | Refresh token expiry (seconds) |
| `AUTH_ANONYMOUS_ENABLED` | `false` | Allow anonymous authentication |
| `AUTH_SIGNUP_ENABLED` | `true` | Allow user registration |
| `AUTH_PASSWORD_MIN_LENGTH` | `8` | Minimum password length |
| `AUTH_PASSWORD_REQUIRE_UPPERCASE` | `false` | Require uppercase in password |
| `AUTH_PASSWORD_REQUIRE_LOWERCASE` | `false` | Require lowercase in password |
| `AUTH_PASSWORD_REQUIRE_NUMBER` | `false` | Require number in password |
| `AUTH_PASSWORD_REQUIRE_SPECIAL` | `false` | Require special character |
| `AUTH_MFA_ENABLED` | `false` | Enable multi-factor authentication |
| `AUTH_WEBAUTHN_ENABLED` | `false` | Enable WebAuthn/Passkeys |
| `AUTH_EMAIL_VERIFICATION_REQUIRED` | `false` | Require email verification |

### Storage Service (MinIO/S3)

| Variable | Default | Description |
|----------|---------|-------------|
| `STORAGE_VERSION` | `v0.6.1` | Hasura Storage version |
| `STORAGE_PORT` | `9001` | Storage service port |
| `MINIO_PORT` | `9000` | MinIO API port |
| `MINIO_CONSOLE_PORT` | `9001` | MinIO console port |
| `MINIO_ROOT_USER` | `minio` | MinIO root username |
| `MINIO_ROOT_PASSWORD` | `<generated>` | MinIO root password |
| `S3_ACCESS_KEY` | `${MINIO_ROOT_USER}` | S3 access key |
| `S3_SECRET_KEY` | `${MINIO_ROOT_PASSWORD}` | S3 secret key |
| `S3_ENDPOINT` | `http://minio:9000` | S3 endpoint URL |
| `S3_BUCKET` | `${PROJECT_NAME}` | Default S3 bucket |
| `S3_REGION` | `us-east-1` | S3 region |
| `STORAGE_MAX_FILE_SIZE` | `52428800` | Max file size (bytes) |
| `STORAGE_ALLOWED_MIME_TYPES` | `image/*,video/*,application/pdf` | Allowed MIME types |
| `STORAGE_PRESIGNED_URL_EXPIRES_IN` | `600` | Presigned URL expiry (seconds) |

### Admin UI

| Variable | Default | Description |
|----------|---------|-------------|
| `NSELF_ADMIN_VERSION` | `v0.0.7` | Admin UI version |
| `NSELF_ADMIN_PORT` | `3021` | Admin UI port |
| `NSELF_ADMIN_AUTH_PROVIDER` | `basic` | Auth provider (basic, oauth2) |
| `ADMIN_USERNAME` | `admin` | Admin username |
| `ADMIN_PASSWORD` | `<generated>` | Admin password (plain) |
| `ADMIN_PASSWORD_HASH` | `<generated>` | Admin password hash |
| `ADMIN_SECRET_KEY` | `<generated>` | Admin session secret |
| `ADMIN_2FA_ENABLED` | `false` | Enable 2FA for admin |
| `ADMIN_SESSION_TIMEOUT` | `3600` | Session timeout (seconds) |

### Redis Cache

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_VERSION` | `7-alpine` | Redis version |
| `REDIS_PORT` | `6379` | Redis port |
| `REDIS_PASSWORD` | `<generated>` | Redis password |
| `REDIS_MAX_MEMORY` | `256mb` | Maximum memory |
| `REDIS_EVICTION_POLICY` | `allkeys-lru` | Eviction policy |
| `REDIS_PERSISTENCE` | `false` | Enable persistence |

### Search Services

| Variable | Default | Description |
|----------|---------|-------------|
| `SEARCH_ENGINE` | `meilisearch` | Search engine (meilisearch, typesense, zinc, elasticsearch, opensearch, sonic) |
| `SEARCH_PORT` | `<engine-specific>` | Search service port |
| **Meilisearch** | | |
| `MEILISEARCH_VERSION` | `v1.5` | Meilisearch version |
| `MEILISEARCH_PORT` | `7700` | Meilisearch port |
| `MEILISEARCH_MASTER_KEY` | `<generated>` | Master key |
| `MEILISEARCH_ENV` | `development` | Environment |
| **Typesense** | | |
| `TYPESENSE_VERSION` | `0.25.1` | Typesense version |
| `TYPESENSE_PORT` | `8108` | Typesense port |
| `TYPESENSE_API_KEY` | `<generated>` | API key |
| **Zinc** | | |
| `ZINC_VERSION` | `latest` | Zinc version |
| `ZINC_PORT` | `4080` | Zinc port |
| `ZINC_ADMIN_USER` | `admin` | Admin username |
| `ZINC_ADMIN_PASSWORD` | `<generated>` | Admin password |
| **Elasticsearch** | | |
| `ELASTICSEARCH_VERSION` | `8.11.0` | Elasticsearch version |
| `ELASTICSEARCH_PORT` | `9200` | Elasticsearch port |
| `ELASTICSEARCH_DISCOVERY_TYPE` | `single-node` | Discovery type |
| **OpenSearch** | | |
| `OPENSEARCH_VERSION` | `2.11.0` | OpenSearch version |
| `OPENSEARCH_PORT` | `9200` | OpenSearch port |
| `OPENSEARCH_DASHBOARD_PORT` | `5601` | Dashboard port |
| `OPENSEARCH_INITIAL_ADMIN_PASSWORD` | `<generated>` | Admin password |
| **Sonic** | | |
| `SONIC_VERSION` | `v1.4.0` | Sonic version |
| `SONIC_PORT` | `1491` | Sonic port |
| `SONIC_PASSWORD` | `<generated>` | Password |

## Email Providers

### Development (MailPit)

| Variable | Default | Description |
|----------|---------|-------------|
| `MAILPIT_VERSION` | `latest` | MailPit version |
| `MAILPIT_SMTP_PORT` | `1025` | SMTP port |
| `MAILPIT_UI_PORT` | `8025` | Web UI port |

### Production Email Providers

Support for 16+ email providers. Common configuration:

| Variable | Default | Description |
|----------|---------|-------------|
| `EMAIL_PROVIDER` | `mailpit` | Email provider |
| `AUTH_SMTP_HOST` | `<provider-specific>` | SMTP host |
| `AUTH_SMTP_PORT` | `587` | SMTP port |
| `AUTH_SMTP_USER` | `<required>` | SMTP username |
| `AUTH_SMTP_PASS` | `<required>` | SMTP password |
| `AUTH_SMTP_SECURE` | `true` | Use TLS |
| `AUTH_SMTP_SENDER` | `noreply@${BASE_DOMAIN}` | Sender address |

Supported providers:
- SendGrid
- AWS SES
- Mailgun
- Postmark
- Gmail
- Office365
- Brevo (Sendinblue)
- Resend
- SparkPost
- Mandrill
- Elastic Email
- SMTP2GO
- MailerSend
- Mailchimp Transactional
- Custom SMTP

## Frontend Applications

Support for multiple frontend applications:

### Individual Format (Preferred for Wizard)

| Variable | Example | Description |
|----------|---------|-------------|
| `FRONTEND_APP_COUNT` | `2` | Number of frontend apps |
| `FRONTEND_APP_1_NAME` | `web` | App 1 name |
| `FRONTEND_APP_1_PORT` | `3001` | App 1 port |
| `FRONTEND_APP_1_PREFIX` | `app` | App 1 URL prefix |
| `FRONTEND_APP_2_NAME` | `admin` | App 2 name |
| `FRONTEND_APP_2_PORT` | `3002` | App 2 port |
| `FRONTEND_APP_2_PREFIX` | `admin` | App 2 URL prefix |

### Compact Format (Alternative)

| Variable | Example | Description |
|----------|---------|-------------|
| `FRONTEND_APPS` | `web:3001:app,admin:3002:admin` | Comma-separated app definitions |

## Internal Routes

Route a custom subdomain to a Docker-internal service (e.g., `api.sites.localhost` → `hasura:8080`). These configs survive `nself build` rebuilds because they are generated deterministically from `.env`.

Up to 20 routes supported (`INTERNAL_ROUTE_1_*` through `INTERNAL_ROUTE_20_*`).

### Per-Route Variables

For each route N (1–20):

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `INTERNAL_ROUTE_N_NAME` | Yes | — | Route identifier. Also used as subdomain if `_SUBDOMAIN` is not set. |
| `INTERNAL_ROUTE_N_SUBDOMAIN` | No | Same as `_NAME` | Subdomain portion of the URL (e.g., `api.sites` produces `api.sites.{BASE_DOMAIN}`). |
| `INTERNAL_ROUTE_N_TARGET` | No | `hasura:8080` | Docker-internal upstream (host:port, e.g., `hasura:8080`, `redis:6379`). |
| `INTERNAL_ROUTE_N_RATE_ZONE` | No | `general` | Nginx rate limit zone (e.g., `graphql_api`, `general`). |
| `INTERNAL_ROUTE_N_WEBSOCKET` | No | `false` | Set to `true` to add WebSocket upgrade headers (`Upgrade`, `Connection`). |

### Example

```bash
# .env — route api.sites.localhost → hasura:8080
INTERNAL_ROUTE_1_NAME=api-sites
INTERNAL_ROUTE_1_SUBDOMAIN=api.sites
INTERNAL_ROUTE_1_TARGET=hasura:8080
INTERNAL_ROUTE_1_RATE_ZONE=graphql_api

# Route ws.sites.localhost → a WebSocket service
INTERNAL_ROUTE_2_NAME=ws-sites
INTERNAL_ROUTE_2_SUBDOMAIN=ws.sites
INTERNAL_ROUTE_2_TARGET=myservice:4000
INTERNAL_ROUTE_2_WEBSOCKET=true
```

After adding routes: `nself build && nself restart nginx`

Generated config: `nginx/sites/internal-<name>.conf`

## Custom Microservices

### Service Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVICES_ENABLED` | `false` | Enable custom services |
| `NESTJS_SERVICES` | `""` | NestJS service names |
| `EXPRESS_SERVICES` | `""` | Express service names |
| `FASTIFY_SERVICES` | `""` | Fastify service names |
| `GOLANG_SERVICES` | `""` | Go service names |
| `PYTHON_SERVICES` | `""` | Python service names |
| `RUST_SERVICES` | `""` | Rust service names |

### Individual Service Configuration

For each custom service N (1-10):

| Variable | Example | Description |
|----------|---------|-------------|
| `CS_N_NAME` | `api` | Service name |
| `CS_N_PORT` | `4001` | Service port |
| `CS_N_TYPE` | `nodejs` | Service type |
| `CS_N_MEMORY` | `512M` | Memory limit |
| `CS_N_CPU` | `0.5` | CPU limit |
| `CS_N_REPLICAS` | `1` | Number of replicas |
| `CS_N_HEALTHCHECK` | `/health` | Health check endpoint |
| `CS_N_ENV_VARS` | `NODE_ENV=production` | Additional env vars |

## Backup Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKUP_ENABLED` | `false` | Enable automated backups |
| `BACKUP_SCHEDULE` | `0 2 * * *` | Cron schedule |
| `BACKUP_RETENTION_DAYS` | `30` | Days to retain backups |
| `BACKUP_TYPE` | `local` | Backup type (local, s3, gcs, azure) |
| `BACKUP_PATH` | `./backups` | Local backup path |
| `BACKUP_S3_BUCKET` | `""` | S3 bucket for backups |
| `BACKUP_S3_PREFIX` | `backups/` | S3 key prefix |
| `BACKUP_S3_REGION` | `us-east-1` | S3 region |
| `BACKUP_S3_ACCESS_KEY` | `""` | S3 access key |
| `BACKUP_S3_SECRET_KEY` | `""` | S3 secret key |
| `BACKUP_COMPRESS` | `true` | Compress backups |
| `BACKUP_ENCRYPT` | `false` | Encrypt backups |
| `BACKUP_ENCRYPTION_KEY` | `""` | Encryption key |
| `BACKUP_GFS_ENABLED` | `false` | Enable GFS retention |
| `BACKUP_GFS_WEEKLY` | `4` | Weekly backups to keep |
| `BACKUP_GFS_MONTHLY` | `6` | Monthly backups to keep |
| `BACKUP_GFS_YEARLY` | `2` | Yearly backups to keep |

## SSL/TLS Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SSL_ENABLED` | `true` | Enable SSL |
| `SSL_PROVIDER` | `mkcert` | SSL provider (mkcert, letsencrypt, custom) |
| `SSL_CERT_PATH` | `./ssl/cert.pem` | Certificate path |
| `SSL_KEY_PATH` | `./ssl/key.pem` | Private key path |
| `SSL_AUTO_RENEW` | `true` | Auto-renew certificates |
| `LETSENCRYPT_EMAIL` | `${ADMIN_EMAIL}` | Let's Encrypt email |
| `LETSENCRYPT_STAGING` | `false` | Use Let's Encrypt staging |

## Nginx Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `NGINX_VERSION` | `alpine` | Nginx version |
| `NGINX_PORT` | `80` | HTTP port |
| `NGINX_SSL_PORT` | `443` | HTTPS port |
| `NGINX_CLIENT_MAX_BODY_SIZE` | `100M` | Max request body size |
| `NGINX_PROXY_TIMEOUT` | `60s` | Proxy timeout |
| `NGINX_KEEPALIVE_TIMEOUT` | `65` | Keepalive timeout |
| `NGINX_WORKER_PROCESSES` | `auto` | Worker processes |
| `NGINX_WORKER_CONNECTIONS` | `1024` | Worker connections |
| `NGINX_GZIP_ENABLED` | `true` | Enable gzip compression |
| `NGINX_CACHE_ENABLED` | `false` | Enable caching |
| `NGINX_RATE_LIMIT_ENABLED` | `false` | Enable rate limiting |
| `NGINX_RATE_LIMIT` | `10r/s` | Rate limit |

## Monitoring Configuration

### Prometheus

| Variable | Default | Description |
|----------|---------|-------------|
| `PROMETHEUS_VERSION` | `latest` | Prometheus version |
| `PROMETHEUS_PORT` | `9090` | Prometheus port |
| `PROMETHEUS_RETENTION_DAYS` | `15` | Data retention |
| `PROMETHEUS_SCRAPE_INTERVAL` | `15s` | Scrape interval |

### Grafana

| Variable | Default | Description |
|----------|---------|-------------|
| `GRAFANA_VERSION` | `latest` | Grafana version |
| `GRAFANA_PORT` | `3300` | Grafana port |
| `GRAFANA_ADMIN_USER` | `admin` | Admin username |
| `GRAFANA_ADMIN_PASSWORD` | `<generated>` | Admin password |
| `GRAFANA_ANONYMOUS_ENABLED` | `false` | Allow anonymous access |

### Loki

| Variable | Default | Description |
|----------|---------|-------------|
| `LOKI_VERSION` | `latest` | Loki version |
| `LOKI_PORT` | `3100` | Loki port |
| `LOKI_RETENTION_DAYS` | `7` | Log retention |

## Docker Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DOCKER_REGISTRY` | `docker.io` | Docker registry |
| `DOCKER_NAMESPACE` | `""` | Docker namespace |
| `DOCKER_NETWORK_NAME` | `${PROJECT_NAME}_network` | Network name |
| `DOCKER_NETWORK_DRIVER` | `bridge` | Network driver |
| `DOCKER_RESTART_POLICY` | `unless-stopped` | Restart policy |
| `DOCKER_LOG_DRIVER` | `json-file` | Log driver |
| `DOCKER_LOG_MAX_SIZE` | `10m` | Max log size |
| `DOCKER_LOG_MAX_FILE` | `3` | Max log files |

## Development Tools

| Variable | Default | Description |
|----------|---------|-------------|
| `HOT_RELOAD_ENABLED` | `true` | Enable hot reload |
| `MOCK_DATA_ENABLED` | `false` | Enable mock data |
| `SEED_DATA_ENABLED` | `false` | Enable seed data |
| `DEBUG_LOGGING` | `false` | Enable debug logs |
| `PERFORMANCE_MONITORING` | `false` | Enable performance monitoring |

## Security Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SECURITY_HEADERS_ENABLED` | `true` | Enable security headers |
| `CSP_ENABLED` | `false` | Enable Content Security Policy |
| `CORS_ENABLED` | `true` | Enable CORS |
| `CORS_ORIGINS` | `*` | Allowed CORS origins |
| `API_RATE_LIMIT` | `1000` | API rate limit (per hour) |
| `INTRUSION_DETECTION` | `false` | Enable intrusion detection |
| `WAF_ENABLED` | `false` | Enable Web Application Firewall |

## Performance Tuning

| Variable | Default | Description |
|----------|---------|-------------|
| `ENABLE_QUERY_CACHING` | `true` | Enable query caching |
| `CACHE_TTL` | `3600` | Cache TTL (seconds) |
| `CONNECTION_POOL_SIZE` | `20` | Database connection pool |
| `MAX_CONCURRENT_REQUESTS` | `100` | Max concurrent requests |
| `ENABLE_COMPRESSION` | `true` | Enable response compression |
| `ENABLE_HTTP2` | `true` | Enable HTTP/2 |
| `ENABLE_HTTP3` | `false` | Enable HTTP/3 (QUIC) |

## Deprecated Variables (Backward Compatibility)

These variables are maintained for backward compatibility but mapped to new names:

| Old Variable | New Variable | Notes |
|--------------|--------------|-------|
| `NADMIN_ENABLED` | `NSELF_ADMIN_ENABLED` | Use new name |
| `MINIO_ENABLED` | `STORAGE_ENABLED` | Both work |
| `DB_BACKUP_ENABLED` | `BACKUP_ENABLED` | Use new name |
| `DB_BACKUP_SCHEDULE` | `BACKUP_SCHEDULE` | Use new name |
| `DB_BACKUP_RETENTION_DAYS` | `BACKUP_RETENTION_DAYS` | Use new name |

## Variable Validation Rules

### Required Variables
These must be set for nself to function:
- `PROJECT_NAME` - Must be alphanumeric with hyphens
- `BASE_DOMAIN` - Must be valid domain format

### Generated Variables
These are auto-generated if not provided:
- All passwords and secrets
- API keys
- JWT secrets
- Admin credentials

### Computed Variables
These are computed from other variables:
- `HASURA_GRAPHQL_DATABASE_URL` - Built from PostgreSQL settings
- `AUTH_DATABASE_URL` - Built from PostgreSQL settings
- `STORAGE_DATABASE_URL` - Built from PostgreSQL settings

### Validation Patterns
- **Ports**: Must be 1-65535, not in use
- **Passwords**: Minimum 12 characters for generated
- **Project Name**: `^[a-z][a-z0-9-]*$`
- **Domain**: Valid domain or IP format
- **Email**: Valid email format

## Best Practices

### Security
1. Never commit `.env` or `.env.secrets` to version control
2. Use strong, unique passwords for production
3. Enable 2FA where available
4. Rotate secrets regularly
5. Use environment-specific files

### Performance
1. Start only needed services
2. Set appropriate resource limits
3. Enable caching where beneficial
4. Use connection pooling
5. Monitor resource usage

### Development
1. Use `.env.dev` for team defaults
2. Keep `.env` for personal overrides
3. Document custom variables
4. Use consistent naming conventions
5. Validate variables before use

### Production
1. Use `.env.prod` and `.env.secrets`
2. Enable monitoring services
3. Configure backups
4. Use proper SSL certificates
5. Set resource limits

## Environment-Specific Configurations

### Development (`ENV=dev`)
```bash
# Optimized for development
HASURA_GRAPHQL_DEV_MODE=true
HASURA_GRAPHQL_ENABLE_CONSOLE=true
HOT_RELOAD_ENABLED=true
DEBUG_LOGGING=true
SSL_PROVIDER=mkcert
EMAIL_PROVIDER=mailpit
```

### Staging (`ENV=staging`)
```bash
# Production-like but with debugging
HASURA_GRAPHQL_DEV_MODE=false
HASURA_GRAPHQL_ENABLE_CONSOLE=true
HOT_RELOAD_ENABLED=false
DEBUG_LOGGING=true
SSL_PROVIDER=letsencrypt
LETSENCRYPT_STAGING=true
```

### Production (`ENV=prod`)
```bash
# Optimized for production
HASURA_GRAPHQL_DEV_MODE=false
HASURA_GRAPHQL_ENABLE_CONSOLE=false
HOT_RELOAD_ENABLED=false
DEBUG_LOGGING=false
SSL_PROVIDER=letsencrypt
LETSENCRYPT_STAGING=false
BACKUP_ENABLED=true
MONITORING_ENABLED=true
```

## Related Documentation

- [Configuration Overview](README.md) - Configuration guide
- [Search Services](../services/SEARCH.md) - Search services configuration
- [Backup Guide](../guides/BACKUP-RECOVERY.md) - Backup configuration
- [Troubleshooting](../guides/TROUBLESHOOTING.md) - Common issues
- [Architecture](../architecture/ARCHITECTURE.md) - System architecture
