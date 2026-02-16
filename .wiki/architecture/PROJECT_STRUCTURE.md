# nself Project Directory Structure

This guide explains the complete directory structure of an nself project, using the demo setup as an example. Understanding this structure will help you navigate and customize your Backend-as-a-Service deployment.

## Complete Project Directory Tree

```
project-root/                          # Your project directory (e.g., demo-app)
│
├── _backup/                           # Auto-backups from nself operations
│   └── [timestamps]/                  # Timestamped backup folders with docker-compose versions
│
├── auth/                              # nHost Auth service configuration
│   └── config/                        # Auth service config files (JWT, providers, etc.)
│
├── functions/                         # Serverless functions directory
│   ├── dist/                          # Compiled functions output
│   └── src/                           # Function source code
│
├── hasura/                           # Hasura GraphQL engine configuration
│   ├── metadata/                      # Hasura metadata (tables, relationships, permissions)
│   └── migrations/                    # Database migrations managed by Hasura
│
├── logs/                              # Application and service logs
│
├── monitoring/                        # Observability stack configuration
│   ├── grafana/                       # Grafana dashboards and provisioning
│   │   └── provisioning/              # Auto-provisioned dashboards and datasources
│   ├── loki/                          # Loki log aggregation config
│   ├── prometheus/                    # Prometheus metrics collection config
│   ├── promtail/                      # Promtail log shipper config
│   └── tempo/                         # Tempo distributed tracing config
│
├── nginx/                             # Nginx reverse proxy configuration
│   ├── conf.d/                        # Individual service proxy configs
│   │   ├── auth.conf                  # Routes auth.domain → Auth service
│   │   ├── default.conf               # Main domain and catch-all routing
│   │   ├── hasura.conf                # Routes api.domain → Hasura GraphQL
│   │   ├── mailpit.conf               # Routes mail.domain → MailPit UI
│   │   └── storage.conf               # Routes storage.domain → MinIO S3
│   ├── ssl/                           # SSL certificate symlinks
│   └── nginx.conf                     # Main Nginx configuration
│
├── postgres/                          # PostgreSQL database configuration
│   └── init/                          # Database initialization scripts
│       ├── 00-init.sql                # Initial database and user setup
│       ├── 01-extensions.sql          # Enable PostgreSQL extensions
│       ├── 02-schemas.sql             # Create database schemas
│       ├── 10-hasura.sql              # Hasura-specific setup
│       └── 20-auth.sql                # Auth service database setup
│
├── services/                          # Custom backend services (from templates)
│   ├── bullmq_worker/                 # BullMQ job queue worker service
│   ├── express_api/                   # Express.js REST API service
│   ├── go_grpc/                       # Go gRPC service
│   └── python_api/                    # Python FastAPI ML/data service
│
├── ssl/                               # SSL certificates storage
│   └── certificates/
│       ├── localhost/                 # Local dev certificates
│       └── [domain]/                  # Production domain certificates
│
├── storage/                           # MinIO/S3 file storage
│   ├── temp/                          # Temporary file uploads
│   └── uploads/                       # Persistent file storage
│
├── .volumes/                          # Docker volume mount points (hidden)
│   ├── postgres/                      # PostgreSQL data persistence
│   ├── redis/                         # Redis data persistence
│   └── minio/                         # MinIO object storage data
│
├── docker-compose.yml                 # Main Docker Compose orchestration file
├── .env                               # Active environment configuration
├── .env.dev                           # Development environment settings
├── .env.staging                       # Staging environment settings (optional)
├── .env.prod                          # Production environment settings (optional)
├── .env.example                       # Example environment template
└── .gitignore                         # Git ignore rules
```

## Directory Descriptions

### Core Infrastructure

**`_backup/`** - Automatic backups created by nself during operations. Each timestamped folder contains previous versions of configuration files, useful for rollback scenarios.

**`.volumes/`** - Docker volume mount points for persistent data storage. This hidden directory contains subdirectories for each stateful service (PostgreSQL, Redis, MinIO) ensuring data persists across container restarts.

### Service Configuration

**`auth/`** - Configuration for the authentication service, including JWT settings, OAuth providers, email templates, and security rules.

**`hasura/`** - GraphQL engine configuration containing:
- `metadata/` - Table definitions, relationships, permissions, and remote schemas
- `migrations/` - Versioned database schema changes

**`functions/`** - Serverless functions that extend your backend capabilities:
- `src/` - TypeScript/JavaScript source code
- `dist/` - Compiled production-ready functions

### Custom Services

**`services/`** - Your custom backend services generated from templates:
- Each subdirectory represents a microservice
- Contains Dockerfile, source code, and configuration
- Services can be in any language (Node.js, Python, Go, Rust, etc.)
- Automatically integrated with the rest of the stack

### Routing & Networking

**`nginx/`** - Reverse proxy configuration that routes all traffic:
- `conf.d/` - Individual service routing rules
- `ssl/` - Symbolic links to active SSL certificates
- `nginx.conf` - Main configuration with security headers and optimization

**`ssl/certificates/`** - SSL/TLS certificates for HTTPS:
- `localhost/` - Self-signed certificates for local development
- Domain-specific folders for staging/production certificates

### Data Layer

**`postgres/init/`** - Database initialization scripts executed in order:
- `00-init.sql` - Database and user creation
- `01-extensions.sql` - Enable extensions (uuid, pgcrypto, postgis, etc.)
- `02-schemas.sql` - Create logical schemas for service isolation
- `10-hasura.sql` - Hasura-specific tables and functions
- `20-auth.sql` - Authentication service schema

**`storage/`** - File storage managed by MinIO (S3-compatible):
- `uploads/` - User-uploaded files
- `temp/` - Temporary files cleared periodically

### Observability

**`monitoring/`** - Complete observability stack configuration:
- `grafana/` - Dashboards and data source provisioning
- `prometheus/` - Metrics collection rules and targets
- `loki/` - Log aggregation configuration
- `tempo/` - Distributed tracing setup
- `promtail/` - Log shipping from containers to Loki

**`logs/`** - Application logs from all services, useful for debugging and audit trails.

## Environment Files

nself supports multiple environment configurations:

```
.env                  # Active configuration (symlink or copy)
.env.dev              # Development settings (local development)
.env.staging          # Staging settings (pre-production testing)
.env.prod             # Production settings (live environment)
.env.example          # Template with all available options
```

### Environment Selection

- **Development**: Uses `.env.dev` with debug enabled, relaxed security
- **Staging**: Uses `.env.staging` with production-like settings for testing
- **Production**: Uses `.env.prod` with security hardening, optimizations

The active environment is determined by:
1. Direct `.env` file (highest priority)
2. `.env.local` (local overrides)
3. `.env.[environment]` based on ENV variable
4. `.env.example` as fallback template

## File Patterns

### Template Files (`*.template`)

Files ending in `.template` contain placeholders that are replaced during build:
- `{{SERVICE_NAME}}` - Name of the service
- `{{SERVICE_PORT}}` - Port number for the service
- `{{PROJECT_NAME}}` - Your project name
- `{{BASE_DOMAIN}}` - Base domain for routing

### Configuration Files

- `*.conf` - Nginx configuration files
- `*.yml`/`*.yaml` - Docker Compose and service configurations
- `*.sql` - Database initialization and migration scripts
- `*.json` - Service metadata and package definitions

## Key Concepts

### Service Isolation

Each service runs in its own container with isolated:
- Network namespace (communication via Docker network)
- File system (only specified volumes are shared)
- Process space (independent process trees)
- Resource limits (CPU/memory constraints)

### Data Persistence

Stateful data is stored in `.volumes/` ensuring:
- Data survives container restarts
- Easy backup and restoration
- Consistent permissions and ownership

### Multi-Tenancy Support

The structure supports multiple frontend applications through:
- Table prefixing in PostgreSQL
- Separate auth scopes
- Isolated storage buckets
- Independent Redis namespaces

### Security Layers

- SSL/TLS encryption for all services
- Network isolation via Docker networks
- Secret management via environment variables
- Rate limiting and security headers in Nginx

## Best Practices

1. **Never commit `.env` files** - Use `.env.example` as template
2. **Keep sensitive data in `.volumes/`** - Excluded from version control
3. **Use `services/` for custom code** - Templates ensure consistency
4. **Monitor `_backup/` size** - Clean old backups periodically
5. **Organize migrations chronologically** - Use numbered prefixes
6. **Document service dependencies** - In service README files

## Common Operations

### Adding a New Service
1. Define in `.env` file using `CS_N` variables:
   ```bash
   CS_1=my_service:express-js:8001
   ```
2. Run `nself build` to generate from template
3. Customize code in `services/my_service/`
4. Rebuild with `nself start`

### Environment Promotion
```bash
# Development to Staging
cp .env.dev .env.staging
# Edit .env.staging with staging values

# Staging to Production
cp .env.staging .env.prod
# Edit .env.prod with production values
```

### Backup Critical Data
```bash
# Backup database
docker exec [project]_postgres pg_dump -U postgres [database] > backup.sql

# Backup volumes
tar -czf volumes-backup.tar.gz .volumes/

# Backup configurations
tar -czf config-backup.tar.gz nginx/ postgres/init/ services/
```

## Stack Overview

A complete nself deployment includes 30+ integrated services:

### Core Services
- **PostgreSQL** - Primary database with extensions
- **Hasura** - GraphQL API engine
- **Auth** - Authentication and authorization
- **Storage** - MinIO S3-compatible object storage
- **Nginx** - Reverse proxy and SSL termination

### Optional Services
- **Redis** - Caching and session storage
- **BullMQ** - Job queue and background processing
- **MeiliSearch** - Full-text search engine
- **MailPit** - Email testing (dev) / SMTP (prod)
- **Functions** - Serverless compute

### Monitoring Stack
- **Prometheus** - Metrics collection
- **Grafana** - Visualization and dashboards
- **Loki** - Log aggregation
- **Tempo** - Distributed tracing
- **Alertmanager** - Alert routing and management

### Custom Services
- Up to 10 custom microservices from templates
- Support for any programming language
- Automatic integration with infrastructure

## Related Documentation

- [nself Directory Structure](DIRECTORY_STRUCTURE.md) - nself tool structure
- [Environment Configuration](../configuration/ENVIRONMENT-VARIABLES.md) - Detailed env variable reference
- [Service Templates](../reference/SERVICE_TEMPLATES.md) - Available service templates
- [Docker Compose](BUILD_ARCHITECTURE.md) - Container orchestration details
- [SSL Configuration](../configuration/SSL.md) - Certificate management
- [Backup & Recovery](../guides/BACKUP-RECOVERY.md) - Data protection strategies
- [Custom Services](../services/SERVICES_CUSTOM.md) - Building custom microservices
