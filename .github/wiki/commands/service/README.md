# nself service

**Category**: Service Management Commands

Manage optional nself services including storage, email, search, and custom services.

## Overview

All service operations use `nself service <type> <action>` for managing optional and custom services.

**Service Types**:
- admin - nself Admin UI
- storage - MinIO S3 storage
- email - Email service
- search - Search engine (MeiliSearch/Typesense)
- redis - Redis cache
- functions - Serverless functions
- mlflow - ML experiment tracking
- realtime - Realtime subscriptions

## Command Structure

```bash
nself service <type> <action> [options]
```

**Common Actions**:
- `enable` - Enable service
- `disable` - Disable service
- `start` - Start service
- `stop` - Stop service
- `restart` - Restart service
- `status` - Check service status
- `logs` - View service logs
- `config` - Configure service
- `url` - Get service URL

## Service Types

### nself service admin

Manage nself Admin web UI.

**Actions**:
```bash
nself service admin enable        # Enable admin UI
nself service admin disable       # Disable admin UI
nself service admin start         # Start admin UI
nself service admin open          # Open in browser
nself service admin url           # Get admin URL
nself service admin config        # Configure admin
```

**Configuration**:
```bash
NSELF_ADMIN_ENABLED=true
NSELF_ADMIN_PORT=3001
NSELF_ADMIN_ROUTE=admin
```

**URL**: `https://admin.localhost`

### nself service storage

Manage MinIO S3-compatible object storage.

**Actions**:
```bash
nself service storage enable       # Enable MinIO
nself service storage disable      # Disable MinIO
nself service storage start        # Start MinIO
nself service storage create-bucket <name>  # Create bucket
nself service storage list-buckets # List buckets
nself service storage set-policy   # Set bucket policy
nself service storage url          # Get storage URL
```

**Configuration**:
```bash
MINIO_ENABLED=true
MINIO_PORT=9000
MINIO_CONSOLE_PORT=9001
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin
```

**URLs**:
- S3 API: `https://storage.localhost`
- Console: `https://minio.localhost`

**Examples**:
```bash
# Enable storage
nself service storage enable

# Create bucket
nself service storage create-bucket uploads

# Set public policy
nself service storage set-policy uploads public
```

### nself service email

Manage email service (MailPit for dev, SMTP for production).

**Actions**:
```bash
nself service email enable        # Enable email service
nself service email disable       # Disable email service
nself service email test          # Send test email
nself service email config        # Configure SMTP
nself service email logs          # View email logs
```

**Development (MailPit)**:
```bash
MAILPIT_ENABLED=true
MAILPIT_SMTP_PORT=1025
MAILPIT_UI_PORT=8025
```

**Production (SMTP)**:
```bash
EMAIL_PROVIDER=smtp
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USER=apikey
SMTP_PASSWORD=your-api-key
SMTP_FROM=noreply@example.com
```

**URL**: `https://mail.localhost` (MailPit UI)

**Examples**:
```bash
# Enable MailPit in dev
nself service email enable

# Test email
nself service email test --to user@example.com

# Configure SMTP for production
nself service email config --provider smtp \
  --host smtp.sendgrid.net \
  --user apikey \
  --from noreply@example.com
```

### nself service search

Manage search engine (MeiliSearch or Typesense).

**Actions**:
```bash
nself service search enable       # Enable search
nself service search disable      # Disable search
nself service search create-index <name>  # Create index
nself service search list-indexes # List indexes
nself service search import       # Import documents
nself service search config       # Configure search
```

**MeiliSearch**:
```bash
MEILISEARCH_ENABLED=true
MEILISEARCH_PORT=7700
MEILISEARCH_MASTER_KEY=your-master-key
```

**Typesense** (alternative):
```bash
TYPESENSE_ENABLED=true
TYPESENSE_PORT=8108
TYPESENSE_API_KEY=your-api-key
```

**URL**: `https://search.localhost`

**Examples**:
```bash
# Enable MeiliSearch
nself service search enable

# Create index
nself service search create-index products

# Import documents
nself service search import products < products.json
```

### nself service redis

Manage Redis cache and sessions.

**Actions**:
```bash
nself service redis enable        # Enable Redis
nself service redis disable       # Disable Redis
nself service redis start         # Start Redis
nself service redis cli           # Open Redis CLI
nself service redis flush         # Flush all data
nself service redis info          # Show Redis info
```

**Configuration**:
```bash
REDIS_ENABLED=true
REDIS_PORT=6379
REDIS_PASSWORD=your-redis-password
REDIS_MAXMEMORY=256mb
REDIS_EVICTION_POLICY=allkeys-lru
```

**Examples**:
```bash
# Enable Redis
nself service redis enable

# Open Redis CLI
nself service redis cli

# Flush cache
nself service redis flush --confirm
```

### nself service functions

Manage serverless functions runtime.

**Actions**:
```bash
nself service functions enable    # Enable functions
nself service functions disable   # Disable functions
nself service functions list      # List functions
nself service functions create    # Create function
nself service functions deploy    # Deploy function
nself service functions invoke    # Invoke function
nself service functions logs      # View function logs
```

**Configuration**:
```bash
FUNCTIONS_ENABLED=true
FUNCTIONS_PORT=3100
FUNCTIONS_RUNTIME=nodejs20  # nodejs20, python311, go
```

**Examples**:
```bash
# Enable functions
nself service functions enable

# Create function
nself service functions create hello-world --runtime nodejs20

# Deploy function
nself service functions deploy hello-world

# Invoke function
nself service functions invoke hello-world --data '{"name":"World"}'
```

### nself service mlflow

Manage MLflow for ML experiment tracking.

**Actions**:
```bash
nself service mlflow enable       # Enable MLflow
nself service mlflow disable      # Disable MLflow
nself service mlflow start        # Start MLflow
nself service mlflow url          # Get MLflow URL
nself service mlflow list-runs    # List experiment runs
```

**Configuration**:
```bash
MLFLOW_ENABLED=true
MLFLOW_PORT=5000
MLFLOW_BACKEND_STORE=postgresql
MLFLOW_ARTIFACT_STORE=s3  # Requires MinIO
```

**Requirements**:
- MinIO must be enabled (artifact storage)

**URL**: `https://mlflow.localhost`

**Examples**:
```bash
# Enable MLflow (also enables MinIO)
nself service mlflow enable

# Open MLflow UI
nself service mlflow open
```

### nself service realtime

Manage realtime subscriptions and websockets.

**Actions**:
```bash
nself service realtime enable     # Enable realtime
nself service realtime disable    # Disable realtime
nself service realtime status     # Check status
nself service realtime connections # Show active connections
```

**Configuration**:
```bash
REALTIME_ENABLED=true
REALTIME_PORT=4001
```

**Note**: Hasura includes built-in realtime subscriptions. This service is for additional realtime features.

## Service Dependencies

Some services require others to be enabled:

```
mlflow → requires → minio (artifact storage)
functions → requires → minio (code storage)
email → optional → redis (rate limiting)
```

**Auto-enable dependencies**:
```bash
# Enabling MLflow automatically enables MinIO
nself service mlflow enable
# ✓ Enabled: minio (required by mlflow)
# ✓ Enabled: mlflow
```

## Common Workflows

### Enable Multiple Services

```bash
# Enable storage and search
nself service storage enable
nself service search enable

# Or in .env
MINIO_ENABLED=true
MEILISEARCH_ENABLED=true

# Then rebuild
nself build && nself restart
```

### Configure Service

```bash
# Set storage credentials
nself service storage config --user admin --password secure-password

# Set search API key
nself service search config --master-key secure-key

# Configure email
nself service email config --provider smtp --host smtp.sendgrid.net
```

### Service Health Check

```bash
# Check all services
nself status

# Check specific service
nself service storage status
nself service redis status
```

### Service URLs

```bash
# Get all service URLs
nself urls

# Get specific service URL
nself service storage url
nself service admin url
```

## Service Configuration

### Enable/Disable in .env

```bash
# Enable services
MINIO_ENABLED=true
REDIS_ENABLED=true
MEILISEARCH_ENABLED=true
NSELF_ADMIN_ENABLED=true

# Disable services
MLFLOW_ENABLED=false
FUNCTIONS_ENABLED=false
```

### Port Configuration

```bash
# Default ports
MINIO_PORT=9000
MINIO_CONSOLE_PORT=9001
REDIS_PORT=6379
MEILISEARCH_PORT=7700
MLFLOW_PORT=5000
FUNCTIONS_PORT=3100
NSELF_ADMIN_PORT=3001
```

### Resource Limits

```bash
# Redis memory limit
REDIS_MAXMEMORY=256mb

# MinIO storage limit
MINIO_MAX_STORAGE=10GB
```

## Troubleshooting

### Service Won't Start

```bash
# Check status
nself status <service>

# View logs
nself logs <service>

# Check dependencies
nself service <type> status

# Restart service
nself service <type> restart
```

### Port Conflicts

```bash
# Check port usage
lsof -i :9000

# Change port in .env
MINIO_PORT=9001

# Rebuild and restart
nself build && nself restart
```

### Permission Issues

```bash
# Check MinIO credentials
nself service storage config --show

# Reset credentials
nself service storage config --reset

# Check Redis password
nself config get REDIS_PASSWORD
```

## Service Monitoring

### Health Checks

```bash
# All services
nself health

# Specific service
nself health minio
nself health redis
```

### Resource Usage

```bash
# View resource usage
nself status --resources

# Monitor in real-time
nself monitor
```

### Service Logs

```bash
# View logs
nself logs minio
nself logs redis

# Follow logs
nself logs -f meilisearch
```

## Related Commands

- `nself build` - Rebuild service configs
- `nself start` - Start all services
- `nself status` - Check service status
- `nself urls` - Get service URLs
- `nself config` - Manage configuration

## See Also

- [Service Configuration Guide](../../guides/SERVICES.md)
- [Storage Guide](../../guides/STORAGE.md)
- [Email Configuration](../../guides/EMAIL.md)
- [Search Setup](../../guides/SEARCH.md)
- [Functions Guide](../../guides/FUNCTIONS.md)
