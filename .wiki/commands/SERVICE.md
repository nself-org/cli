# nself service - Optional Service Management

**Version**: 0.4.7+ | **Status**: Available

---

## Overview

The `nself service` command provides unified management for all optional services in nself. It consolidates individual service commands (`email`, `search`, `functions`, `mlflow`, etc.) into a single coherent interface.

---

## Subcommands

### General Operations

```bash
nself service list                     # List all optional services
nself service enable <service>         # Enable a service
nself service disable <service>        # Disable a service
nself service status [service]         # Show service status
nself service restart <service>        # Restart a service
nself service logs <service>           # View service logs
```

### Email Service

```bash
nself service email test               # Send test email
nself service email inbox              # Open MailPit inbox
nself service email config             # Email configuration
```

### Search Service

```bash
nself service search index             # Reindex all data
nself service search query <term>      # Run search query
nself service search stats             # Index statistics
```

### Serverless Functions

```bash
nself service functions deploy         # Deploy all functions
nself service functions invoke <fn>    # Invoke a function
nself service functions logs [fn]      # View function logs
nself service functions list           # List functions
```

### ML Tracking (MLflow)

```bash
nself service mlflow ui                # Open MLflow UI
nself service mlflow experiments       # List experiments
nself service mlflow runs              # List runs
nself service mlflow artifacts         # Browse artifacts
```

### Object Storage (MinIO)

```bash
nself service storage buckets          # List buckets
nself service storage upload           # Upload file
nself service storage download         # Download file
nself service storage presign          # Generate presigned URL
```

### Cache (Redis)

```bash
nself service cache stats              # Redis statistics
nself service cache flush              # Flush cache
nself service cache keys               # List keys
```

---

## Optional Services

nself supports 7 optional service types:

| Service | Description | Enable Variable |
|---------|-------------|-----------------|
| **nself-admin** | Web management UI | `NSELF_ADMIN_ENABLED=true` |
| **MinIO** | S3-compatible storage | `MINIO_ENABLED=true` |
| **Redis** | Cache and sessions | `REDIS_ENABLED=true` |
| **Functions** | Serverless runtime | `FUNCTIONS_ENABLED=true` |
| **MLflow** | ML experiment tracking | `MLFLOW_ENABLED=true` |
| **MailPit** | Development email | `MAILPIT_ENABLED=true` |
| **MeiliSearch** | Full-text search | `MEILISEARCH_ENABLED=true` |

---

## Service Management

### List Services

```bash
$ nself service list
Optional Services
=================
SERVICE         STATUS      PORT      URL
nself-admin     enabled     3001      admin.local.nself.org
minio           enabled     9000      minio.local.nself.org
redis           enabled     6379      -
functions       enabled     5555      functions.local.nself.org
mlflow          disabled    -         -
mailpit         enabled     8025      mail.local.nself.org
meilisearch     enabled     7700      search.local.nself.org
```

### Enable/Disable Services

```bash
# Enable a service
$ nself service enable mlflow
Enabling MLflow...
  Adding MLFLOW_ENABLED=true to .env
  Rebuilding configuration...
  Starting MLflow container...
MLflow enabled and running at: http://mlflow.local.nself.org

# Disable a service
$ nself service disable mlflow
Disabling MLflow...
  Stopping MLflow container...
  Setting MLFLOW_ENABLED=false
MLflow disabled
```

### Service Status

```bash
$ nself service status redis
Redis Status
============
Status: Running
Container: myapp_redis
Port: 6379
Memory Usage: 45MB / 256MB
Connected Clients: 3
Keys: 1,247
Uptime: 2d 4h 32m
```

---

## Email Service

### Configuration

```bash
$ nself service email config
Email Configuration
===================
Provider: mailpit (development)
SMTP Host: mailpit
SMTP Port: 1025
Web Interface: http://mail.local.nself.org

Production providers available:
  - sendgrid
  - mailgun
  - ses
  - smtp
```

### Test Email

```bash
$ nself service email test
Sending test email...
To: test@example.com
From: noreply@local.nself.org
Subject: nself Test Email

Email sent successfully!
View in MailPit: http://mail.local.nself.org
```

### Open Inbox

```bash
$ nself service email inbox
Opening MailPit inbox in browser...
```

---

## Search Service

### Indexing

```bash
# Reindex all data
$ nself service search index
Indexing data to MeiliSearch...
  users: 1,247 documents indexed
  products: 5,892 documents indexed
  posts: 3,421 documents indexed
Indexing complete!

# Index specific table
$ nself service search index --table products
```

### Query

```bash
$ nself service search query "ergonomic keyboard"
Search Results (23 matches)
===========================
1. Ergonomic Mechanical Keyboard Pro (score: 0.95)
2. Split Ergonomic Keyboard (score: 0.89)
3. Wireless Ergonomic Keyboard Set (score: 0.84)
...
```

### Statistics

```bash
$ nself service search stats
MeiliSearch Statistics
======================
Indexes: 3
Total Documents: 10,560
Database Size: 45MB
Last Update: 2 minutes ago

INDEX          DOCUMENTS   SIZE
users          1,247       5MB
products       5,892       28MB
posts          3,421       12MB
```

---

## Serverless Functions

### Deploy Functions

```bash
$ nself service functions deploy
Deploying functions...
  ✓ sendWelcomeEmail
  ✓ processPayment
  ✓ generateReport
  ✓ resizeImage

4 functions deployed successfully
```

### Invoke Function

```bash
$ nself service functions invoke sendWelcomeEmail --data '{"userId": 123}'
Function: sendWelcomeEmail
Status: Success
Duration: 245ms
Response:
{
  "sent": true,
  "messageId": "msg_abc123"
}
```

### View Logs

```bash
$ nself service functions logs sendWelcomeEmail
[2026-01-23 10:30:15] INFO: Function invoked
[2026-01-23 10:30:15] INFO: Fetching user 123
[2026-01-23 10:30:16] INFO: Sending email to user@example.com
[2026-01-23 10:30:16] INFO: Email sent: msg_abc123
```

---

## ML Tracking (MLflow)

### Open UI

```bash
$ nself service mlflow ui
Opening MLflow UI in browser...
URL: http://mlflow.local.nself.org
```

### List Experiments

```bash
$ nself service mlflow experiments
MLflow Experiments
==================
ID   NAME                    RUNS   LAST RUN
1    Default                 12     2026-01-22
2    image-classification    45     2026-01-23
3    price-prediction        28     2026-01-21
```

### List Runs

```bash
$ nself service mlflow runs --experiment image-classification
Recent Runs
===========
RUN ID          STATUS      ACCURACY    LOSS      DURATION
abc123          FINISHED    0.945       0.124     5m 32s
def456          FINISHED    0.932       0.156     4m 18s
ghi789          FAILED      -           -         2m 05s
```

---

## Object Storage (MinIO)

### List Buckets

```bash
$ nself service storage buckets
MinIO Buckets
=============
BUCKET          OBJECTS     SIZE        CREATED
uploads         1,247       2.3GB       2026-01-15
backups         45          15.6GB      2026-01-10
public          892         456MB       2026-01-18
```

### Upload File

```bash
$ nself service storage upload ./image.jpg --bucket uploads
Uploading image.jpg to uploads...
  Size: 2.4MB
  Progress: 100%
Uploaded: uploads/image.jpg
URL: http://minio.local.nself.org/uploads/image.jpg
```

### Generate Presigned URL

```bash
$ nself service storage presign uploads/image.jpg --expires 1h
Presigned URL (valid for 1 hour):
https://minio.local.nself.org/uploads/image.jpg?X-Amz-...
```

---

## Cache (Redis)

### Statistics

```bash
$ nself service cache stats
Redis Statistics
================
Version: 7.2.4
Uptime: 2d 4h 32m
Memory Used: 45MB / 256MB
Connected Clients: 3
Total Keys: 1,247
Ops/sec: 156
Hit Rate: 94.2%
```

### Flush Cache

```bash
$ nself service cache flush
WARNING: This will delete all cached data.
Continue? [y/N]: y
Flushing Redis cache...
Cache flushed successfully
```

### List Keys

```bash
$ nself service cache keys --pattern "user:*"
Keys matching 'user:*'
======================
user:123:session
user:123:preferences
user:456:session
user:789:cart
...
(showing 10 of 1,247 keys)
```

---

## Environment Variables

Each service can be configured via environment variables:

```bash
# Email
EMAIL_PROVIDER=sendgrid
SENDGRID_API_KEY=SG.xxx

# Search
SEARCH_PROVIDER=meilisearch
MEILISEARCH_API_KEY=xxx

# Functions
FUNCTIONS_RUNTIME=nodejs20

# MLflow
MLFLOW_TRACKING_URI=http://mlflow:5000

# Storage
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin

# Cache
REDIS_URL=redis://redis:6379
```

---

## Examples

### Development Setup

```bash
# Enable development services
nself service enable mailpit
nself service enable meilisearch
nself service enable redis

# Check status
nself service list

# Test email
nself service email test
```

### Production Setup

```bash
# Configure production email
nself service email config --provider sendgrid

# Enable production services
nself service enable redis
nself service enable meilisearch
nself service enable functions

# Deploy functions
nself service functions deploy
```

---

## Performance (consolidated from `nself perf`)

Performance benchmarking and profiling are now available as `nself service` subcommands.

### Benchmark

```bash
nself service bench api                       # Benchmark GraphQL API
nself service bench api --requests 5000       # 5000 requests
nself service bench api --duration 60         # 60-second test
```

### Scale

```bash
nself service scale hasura 3                  # Scale Hasura to 3 replicas
nself service scale postgres 2                # Scale Postgres to 2 replicas
```

### Profile

```bash
nself service profile hasura                  # Profile Hasura resource usage
nself service profile postgres --duration 30  # 30-second profile
```

### Optimize

```bash
nself service optimize                        # Get optimization suggestions
nself service optimize --auto-fix             # Apply safe optimizations automatically
```

> **Migration:** If you were using `nself perf`, update your commands:
> `nself perf bench` → `nself service bench`
> `nself perf scale` → `nself service scale`
> `nself perf profile` → `nself service profile`
> `nself perf optimize` → `nself service optimize`
> `nself perf migrate` → `nself db migrate`

---

*See also: [CLOUD](CLOUD.md) | [DEPLOY](DEPLOY.md) | [SERVICES_OPTIONAL](../services/SERVICES_OPTIONAL.md)*
