# nself v0.3.9 Command Examples with Outputs

This document provides comprehensive examples of every nself v0.3.9 command with actual output formatting.

## Table of Contents

- [Core Commands](#core-commands)
  - [init](#init---initialize-project)
  - [build](#build---build-infrastructure)
  - [up](#up---start-services)
  - [down](#down---stop-services)
  - [restart](#restart---restart-services)
  - [status](#status---service-status)
  - [logs](#logs---view-logs)
- [Management Commands](#management-commands)
  - [doctor](#doctor---system-diagnostics)
  - [db](#db---database-operations)
  - [email](#email---configure-email)
  - [urls](#urls---service-urls)
  - [prod](#prod---production-setup)
  - [trust](#trust---ssl-certificates)
- [Development Commands](#development-commands)
  - [scaffold](#scaffold---create-services)
  - [diff](#diff---configuration-diff)
  - [reset](#reset---reset-project)
  - [validate-env](#validate-env---validate-configuration)
  - [hot-reload](#hot-reload---development-mode)
- [System Commands](#system-commands)
  - [update](#update---update-nself)
  - [version](#version---version-info)
  - [help](#help---help-text)

---

## Core Commands

### init - Initialize Project

#### Basic Usage
```bash
$ nself init
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║   Welcome to nself - Modern Full-Stack Platform                ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Initializing project: myproject
[INFO] Domain: localhost
[INFO] Creating environment configuration...
[SUCCESS] Created .env.local
[INFO] Creating project structure...
[SUCCESS] Created directory: hasura/migrations
[SUCCESS] Created directory: hasura/metadata
[SUCCESS] Created directory: hasura/seeds
[SUCCESS] Created directory: nginx/conf.d
[SUCCESS] Created directory: nginx/ssl
[SUCCESS] Created directory: postgres/init
[INFO] Creating default configurations...
[SUCCESS] Created nginx/nginx.conf
[SUCCESS] Created postgres/init/00-init.sql
[SUCCESS] Created schema.dbml

╔════════════════════════════════════════════════════════════════╗
║                    INITIALIZATION COMPLETE                     ║
╚════════════════════════════════════════════════════════════════╝

Next steps:
1. Review configuration: cat .env.local
2. Build infrastructure: nself build
3. Start services: nself up

For production deployment: nself prod
```

#### Custom Project Name and Domain
```bash
$ nself init myapp api.example.com
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║   Welcome to nself - Modern Full-Stack Platform                ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Initializing project: myapp
[INFO] Domain: api.example.com
[WARNING] Custom domain detected. Ensure DNS is configured for:
  - api.api.example.com (GraphQL API)
  - auth.api.example.com (Authentication)
  - storage.api.example.com (File storage)
[INFO] Creating environment configuration...
[SUCCESS] Created .env.local with custom settings
...
```

#### Reinitialize Existing Project
```bash
$ nself init
```

**Output (when .env.local exists):**
```
╔════════════════════════════════════════════════════════════════╗
║   Welcome to nself - Modern Full-Stack Platform                ║
╚════════════════════════════════════════════════════════════════╝

[WARNING] Project already initialized (.env.local exists)
Reinitialize project? (y/N) [10s timeout]: y
[INFO] Backed up existing configuration to .env.local.backup
[INFO] Initializing project: myproject
...
```

---

### build - Build Infrastructure

#### Standard Build
```bash
$ nself build
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    BUILDING INFRASTRUCTURE                     ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Loading configuration from .env.local...
[SUCCESS] Configuration loaded

⠋ Generating SSL certificates...
[SUCCESS] Generated SSL certificate for *.localhost
[SUCCESS] Certificate trusted in system keychain

⠋ Generating docker-compose.yml...
[INFO] Configuring services:
  ✓ PostgreSQL (port 5432)
  ✓ Hasura GraphQL (port 8080)
  ✓ Authentication (port 4000)
  ✓ MinIO Storage (port 9000)
  ✓ Nginx Proxy (ports 80, 443)
  ✓ Redis Cache (port 6379) [ENABLED]
  ○ Functions (disabled)
  ○ Dashboard (disabled)
[SUCCESS] Generated docker-compose.yml

⠋ Building Docker images...
[INFO] Building postgres:15-alpine...
[SUCCESS] postgres:15-alpine ready
[INFO] Building hasura/graphql-engine:v2.35.0...
[SUCCESS] hasura/graphql-engine:v2.35.0 ready
[INFO] Building nhost/hasura-auth:latest...
[SUCCESS] nhost/hasura-auth:latest ready
[INFO] Building minio/minio:latest...
[SUCCESS] minio/minio:latest ready
[INFO] Building nginx:alpine...
[SUCCESS] nginx:alpine ready

╔════════════════════════════════════════════════════════════════╗
║                      BUILD COMPLETE                            ║
╚════════════════════════════════════════════════════════════════╝

Ready to start services: nself up
```

#### Build with Cache Clear
```bash
$ nself build --no-cache
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    BUILDING INFRASTRUCTURE                     ║
╚════════════════════════════════════════════════════════════════╝

[WARNING] Building without cache (this will take longer)
[INFO] Cleaning Docker build cache...
[SUCCESS] Cache cleared

⠋ Building Docker images (no cache)...
[INFO] postgres:15-alpine [■□□□□□□□□□] 10%
[INFO] postgres:15-alpine [■■■□□□□□□□] 30%
[INFO] postgres:15-alpine [■■■■■□□□□□] 50%
[INFO] postgres:15-alpine [■■■■■■■□□□] 70%
[INFO] postgres:15-alpine [■■■■■■■■■□] 90%
[SUCCESS] postgres:15-alpine built from scratch
...
```

---

### up - Start Services

#### Standard Start (Attached)
```bash
$ nself up
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                     STARTING SERVICES                          ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Checking prerequisites...
[SUCCESS] Docker daemon is running
[SUCCESS] Required ports are available
[SUCCESS] Configuration is valid

⠋ Starting PostgreSQL...
[SUCCESS] PostgreSQL started (port 5432)

⠋ Waiting for database...
[SUCCESS] Database is ready

⠋ Starting Hasura GraphQL Engine...
[SUCCESS] Hasura started (port 8080)

⠋ Starting Authentication Service...
[SUCCESS] Auth service started (port 4000)

⠋ Starting MinIO Storage...
[SUCCESS] MinIO started (port 9000)

⠋ Starting Nginx Proxy...
[SUCCESS] Nginx started (ports 80, 443)

╔════════════════════════════════════════════════════════════════╗
║                    ALL SERVICES RUNNING                        ║
╚════════════════════════════════════════════════════════════════╝

Service URLs:
  GraphQL API:     https://api.localhost
  Authentication:  https://auth.localhost
  Storage:         https://storage.localhost
  Admin Console:   https://api.localhost/console

Press Ctrl+C to stop services...

postgres_1   | 2025-08-10 12:34:56.789 UTC [1] LOG: database system is ready
hasura_1     | {"type":"startup","timestamp":"2025-08-10T12:34:57.123Z"}
auth_1       | {"level":"info","message":"Server started on port 4000"}
minio_1      | API: https://storage.localhost
nginx_1      | /docker-entrypoint.sh: Configuration complete; ready
```

#### Detached Start
```bash
$ nself up -d
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                     STARTING SERVICES                          ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Starting services in background...
⠋ Creating network nself_default...
[SUCCESS] Network created

⠋ Creating volume nself_postgres_data...
[SUCCESS] Volume created

⠋ Starting nself_postgres_1...
[SUCCESS] PostgreSQL started

⠋ Starting nself_hasura_1...
[SUCCESS] Hasura started

⠋ Starting nself_auth_1...
[SUCCESS] Auth service started

⠋ Starting nself_minio_1...
[SUCCESS] MinIO started

⠋ Starting nself_nginx_1...
[SUCCESS] Nginx started

╔════════════════════════════════════════════════════════════════╗
║                 SERVICES STARTED IN BACKGROUND                 ║
╚════════════════════════════════════════════════════════════════╝

View logs: nself logs -f
Check status: nself status
Stop services: nself down
```

#### Start with Auto-Fix
```bash
$ nself up
```

**Output (when port conflict detected):**
```
╔════════════════════════════════════════════════════════════════╗
║                     STARTING SERVICES                          ║
╚════════════════════════════════════════════════════════════════╝

[WARNING] Port 5432 is in use by process 'docker-proxy' (PID: 12345)
[INFO] Attempting auto-fix...
[INFO] Stopping conflicting container 'old_postgres_1'...
[SUCCESS] Port 5432 is now available

⠋ Starting PostgreSQL...
[SUCCESS] PostgreSQL started (port 5432)
...
```

---

### down - Stop Services

#### Standard Stop
```bash
$ nself down
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                     STOPPING SERVICES                          ║
╚════════════════════════════════════════════════════════════════╝

⠋ Stopping nginx...
[SUCCESS] Nginx stopped

⠋ Stopping authentication service...
[SUCCESS] Auth service stopped

⠋ Stopping MinIO storage...
[SUCCESS] MinIO stopped

⠋ Stopping Hasura GraphQL...
[SUCCESS] Hasura stopped

⠋ Stopping PostgreSQL...
[SUCCESS] PostgreSQL stopped

⠋ Removing containers...
[SUCCESS] Containers removed

╔════════════════════════════════════════════════════════════════╗
║                    ALL SERVICES STOPPED                        ║
╚════════════════════════════════════════════════════════════════╝

Services have been stopped. Data is preserved.
To remove data: nself down --volumes
```

#### Stop with Volume Removal
```bash
$ nself down --volumes
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                     STOPPING SERVICES                          ║
╚════════════════════════════════════════════════════════════════╝

[WARNING] This will delete all data! Continue? (y/N): y

⠋ Stopping all services...
[SUCCESS] Services stopped

⠋ Removing containers...
[SUCCESS] Containers removed

⠋ Removing volumes...
[INFO] Removing nself_postgres_data...
[SUCCESS] PostgreSQL data removed
[INFO] Removing nself_minio_data...
[SUCCESS] MinIO data removed
[SUCCESS] All volumes removed

⠋ Removing network...
[SUCCESS] Network removed

╔════════════════════════════════════════════════════════════════╗
║                  SERVICES AND DATA REMOVED                     ║
╚════════════════════════════════════════════════════════════════╝

All data has been deleted. Run 'nself init' to start fresh.
```

---

### restart - Restart Services

#### Restart All Services
```bash
$ nself restart
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    RESTARTING SERVICES                         ║
╚════════════════════════════════════════════════════════════════╝

⠋ Stopping services...
[SUCCESS] Services stopped

⠋ Starting services...
[SUCCESS] Services started

╔════════════════════════════════════════════════════════════════╗
║                   SERVICES RESTARTED                           ║
╚════════════════════════════════════════════════════════════════╝

All services have been restarted successfully.
```

#### Restart Specific Service
```bash
$ nself restart hasura
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    RESTARTING SERVICE                          ║
╚════════════════════════════════════════════════════════════════╝

⠋ Restarting hasura...
[INFO] Stopping nself_hasura_1...
[SUCCESS] Hasura stopped
[INFO] Starting nself_hasura_1...
[SUCCESS] Hasura started

╔════════════════════════════════════════════════════════════════╗
║                    SERVICE RESTARTED                           ║
╚════════════════════════════════════════════════════════════════╝

Hasura GraphQL Engine has been restarted.
Check status: nself status
```

---

### status - Service Status

#### Standard Status
```bash
$ nself status
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                      SERVICE STATUS                            ║
╚════════════════════════════════════════════════════════════════╝

Service          Status    Health    Uptime      Port(s)
──────────────────────────────────────────────────────────────────
postgres         Running   Healthy   2h 15m      5432
hasura           Running   Healthy   2h 14m      8080
auth             Running   Healthy   2h 14m      4000
minio            Running   Healthy   2h 14m      9000, 9001
nginx            Running   Healthy   2h 14m      80, 443
redis            Running   Healthy   1h 45m      6379

╔════════════════════════════════════════════════════════════════╗
║                     SYSTEM RESOURCES                           ║
╚════════════════════════════════════════════════════════════════╝

Container        CPU %     Memory         Disk
──────────────────────────────────────────────────────────────────
postgres         2.1%      128 MB / 1 GB  2.3 GB
hasura           5.3%      256 MB / 2 GB  145 MB
auth             1.2%      89 MB / 512 MB 78 MB
minio            0.8%      156 MB / 1 GB  1.2 GB
nginx            0.1%      12 MB / 128 MB 23 MB
redis            0.5%      45 MB / 256 MB 125 MB

Total:           9.0%      686 MB         3.9 GB
```

#### JSON Format Status
```bash
$ nself status --format json
```

**Output:**
```json
{
  "services": [
    {
      "name": "postgres",
      "status": "running",
      "health": "healthy",
      "uptime": "2h 15m",
      "ports": [5432],
      "cpu_percent": 2.1,
      "memory_usage": "128 MB",
      "memory_limit": "1 GB",
      "disk_usage": "2.3 GB"
    },
    {
      "name": "hasura",
      "status": "running",
      "health": "healthy",
      "uptime": "2h 14m",
      "ports": [8080],
      "cpu_percent": 5.3,
      "memory_usage": "256 MB",
      "memory_limit": "2 GB",
      "disk_usage": "145 MB"
    }
  ],
  "summary": {
    "total_services": 6,
    "running": 6,
    "stopped": 0,
    "unhealthy": 0,
    "total_cpu": 9.0,
    "total_memory": "686 MB",
    "total_disk": "3.9 GB"
  }
}
```

---

### logs - View Logs

#### View All Logs
```bash
$ nself logs
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                      SERVICE LOGS                              ║
╚════════════════════════════════════════════════════════════════╝

postgres_1  | 2025-08-10 12:34:56 UTC [1] LOG: starting PostgreSQL 15.3
postgres_1  | 2025-08-10 12:34:56 UTC [1] LOG: listening on IPv4 "0.0.0.0", port 5432
postgres_1  | 2025-08-10 12:34:56 UTC [1] LOG: database system is ready to accept connections

hasura_1    | {"type":"startup","timestamp":"2025-08-10T12:34:57.123Z","level":"info"}
hasura_1    | {"type":"http-log","timestamp":"2025-08-10T12:35:01.456Z","level":"info","detail":{"operation":"health_check","request_id":"abc123","response_size":15,"status":200}}

auth_1      | {"level":"info","message":"Server started on port 4000","timestamp":"2025-08-10T12:34:58.789Z"}
auth_1      | {"level":"info","message":"Connected to database","timestamp":"2025-08-10T12:34:59.012Z"}

minio_1     | MinIO Object Storage Server
minio_1     | API: https://storage.localhost
minio_1     | Console: http://localhost:9001

nginx_1     | 2025/08/10 12:35:00 [notice] 1#1: nginx/1.25.0
nginx_1     | 2025/08/10 12:35:00 [notice] 1#1: start worker processes
```

#### Follow Specific Service Logs
```bash
$ nself logs hasura -f
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    HASURA SERVICE LOGS                         ║
╚════════════════════════════════════════════════════════════════╝

Following logs for hasura (Ctrl+C to stop)...

hasura_1 | {"type":"startup","timestamp":"2025-08-10T12:34:57.123Z","level":"info","detail":{"version":"v2.35.0"}}
hasura_1 | {"type":"schema-sync","timestamp":"2025-08-10T12:34:58.456Z","level":"info","detail":{"message":"Schema sync completed"}}
hasura_1 | {"type":"http-log","timestamp":"2025-08-10T12:35:01.789Z","level":"info","detail":{"operation":"query","request_id":"xyz789","query":{"name":"GetUsers"},"response_size":2456,"status":200,"execution_time":45}}
hasura_1 | {"type":"websocket-log","timestamp":"2025-08-10T12:35:15.234Z","level":"info","detail":{"event":"connection_init","connection_id":"conn_123"}}
hasura_1 | {"type":"subscription-log","timestamp":"2025-08-10T12:35:15.567Z","level":"info","detail":{"operation":"subscription","name":"UserUpdates"}}
```

#### Tail Last N Lines
```bash
$ nself logs postgres --tail 10
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                   POSTGRES LOGS (Last 10)                      ║
╚════════════════════════════════════════════════════════════════╝

postgres_1 | 2025-08-10 14:45:23 UTC [52] LOG: checkpoint starting: time
postgres_1 | 2025-08-10 14:45:25 UTC [52] LOG: checkpoint complete
postgres_1 | 2025-08-10 14:47:15 UTC [89] LOG: connection received: host=172.18.0.3
postgres_1 | 2025-08-10 14:47:15 UTC [89] LOG: connection authorized: user=postgres database=postgres
postgres_1 | 2025-08-10 14:47:16 UTC [89] LOG: statement: SELECT version()
postgres_1 | 2025-08-10 14:47:16 UTC [89] LOG: statement: SELECT * FROM users LIMIT 10
postgres_1 | 2025-08-10 14:47:17 UTC [89] LOG: duration: 1.234 ms
postgres_1 | 2025-08-10 14:47:17 UTC [89] LOG: disconnection: session time: 0:00:02.456
postgres_1 | 2025-08-10 14:50:23 UTC [52] LOG: checkpoint starting: time
postgres_1 | 2025-08-10 14:50:24 UTC [52] LOG: checkpoint complete
```

---

## Management Commands

### doctor - System Diagnostics

```bash
$ nself doctor
```

**Output:**
```
nself Doctor v0.3.0
System Diagnostics & Health Check
══════════════════════════════════════════════════════════════════

System Information
──────────────────────────────────────────────────
[INFO] Operating System: Darwin 24.6.0
[INFO] Architecture: arm64
[INFO] Current Directory: /Users/admin/myproject
[INFO] User: admin
[INFO] Date: Sun Aug 10 14:52:30 EDT 2025
[INFO] nself Version: 0.3.0

System Requirements
──────────────────────────────────────────────────
⠋ Checking curl                ✓ curl is available: curl 8.7.1
⠋ Checking Git                 ✓ Git is available: git version 2.46.2
⠋ Checking Docker              ✓ Docker is available: Docker version 28.0.4
⠋ Checking Docker daemon       ✓ Docker daemon is running
⠋ Checking Docker permissions  ✓ Docker can be run without sudo
⠋ Checking Docker Compose      ✓ Docker Compose (plugin) is available: v2.29.1
⠋ Checking memory              ✓ Memory: 8192MB available (minimum 2048MB)
⠋ Checking disk space          ✓ Disk space: 125GB available (minimum 5GB)

Network & Connectivity
──────────────────────────────────────────────────
⠋ Checking internet connectivity  ✓ Internet connectivity is working
⠋ Checking Docker Hub            ✓ Docker Hub is reachable
⠋ Checking port 80               ✓ Port 80 is available for HTTP (nginx)
⠋ Checking port 443              ✓ Port 443 is available for HTTPS (nginx)
⠋ Checking port 5432             ⚠ Port 5432 is already in use (needed for PostgreSQL)
⠋ Checking port 8080             ✓ Port 8080 is available for Hasura GraphQL
⠋ Checking port 4000             ✓ Port 4000 is available for Hasura Auth
⠋ Checking port 9000             ✓ Port 9000 is available for MinIO
⠋ Checking port 6379             ✓ Port 6379 is available for Redis
⠋ Checking port 1025             ✓ Port 1025 is available for SMTP (MailPit)
⠋ Checking port 8025             ✓ Port 8025 is available for MailPit UI

nself Configuration
──────────────────────────────────────────────────
⠋ Checking nself configuration   ✓ .env.local configuration file found
⠋ Loading configuration          ✓ Configuration loaded successfully
⠋ Checking essential variables   ✓ Essential configuration variables are set
[SUCCESS] Using nself.org domain for local development
⠋ Checking docker-compose.yml    ✓ docker-compose.yml found
⠋ Checking running services      ✓ 5/6 services are running
⠋ Checking service health        ⚠ 1 services are stopped
⠋ Checking SSL certificates      ✓ SSL certificates found
⠋ Checking certificate expiry    ✓ Certificate expires: Dec 31 23:59:59 2025 GMT

Service URLs
──────────────────────────────────────────────────
[SUCCESS] Core Services:
[INFO]   GraphQL API:     https://api.localhost
[INFO]   Auth:            https://auth.localhost
[INFO]   Storage:         https://storage.localhost
[INFO]   Admin Console:   https://api.localhost/console

[SUCCESS] Direct Access (localhost):
[INFO]   PostgreSQL:      localhost:5432
[INFO]   MinIO Console:   http://localhost:9001

Recommendations
──────────────────────────────────────────────────
[WARNING] Found 2 warning(s):
• Port 5432 conflict may prevent PostgreSQL from starting
• 1 service is stopped (check with 'nself status')
• Warnings won't prevent nself from running
• Consider addressing for optimal performance

[INFO] Common fixes:
  nself up            - Auto-fix will handle port conflicts
  nself status        - Check service details

──────────────────────────────────────────────────
[SUCCESS] Health check completed - No critical issues found!
```

---

### db - Database Operations

#### Database Migration
```bash
$ nself db migrate
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    DATABASE MIGRATION                          ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Connecting to database...
[SUCCESS] Connected to PostgreSQL

⠋ Checking for pending migrations...
[INFO] Found 3 pending migrations:
  - 001_create_users_table.sql
  - 002_create_posts_table.sql
  - 003_add_user_roles.sql

⠋ Applying migration: 001_create_users_table.sql
[SUCCESS] Created table: users

⠋ Applying migration: 002_create_posts_table.sql
[SUCCESS] Created table: posts
[SUCCESS] Added foreign key: posts.user_id -> users.id

⠋ Applying migration: 003_add_user_roles.sql
[SUCCESS] Added column: users.role
[SUCCESS] Created index: idx_users_role

╔════════════════════════════════════════════════════════════════╗
║                  MIGRATIONS COMPLETE                           ║
╚════════════════════════════════════════════════════════════════╝

Applied 3 migrations successfully.
Database schema is up to date.
```

#### Database Seed
```bash
$ nself db seed
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                     DATABASE SEEDING                           ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Loading seed file: hasura/seeds/default.sql

⠋ Seeding table: users
[SUCCESS] Inserted 100 users

⠋ Seeding table: posts
[SUCCESS] Inserted 500 posts

⠋ Seeding table: comments
[SUCCESS] Inserted 2000 comments

╔════════════════════════════════════════════════════════════════╗
║                    SEEDING COMPLETE                            ║
╚════════════════════════════════════════════════════════════════╝

Database seeded with test data:
  - 100 users
  - 500 posts
  - 2000 comments
```

#### Database Console
```bash
$ nself db console
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    DATABASE CONSOLE                            ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Connecting to PostgreSQL...
[SUCCESS] Connected to database: postgres

psql (15.3)
Type "help" for help.

postgres=# \dt
              List of relations
 Schema |      Name       | Type  |  Owner   
--------+-----------------+-------+----------
 public | users           | table | postgres
 public | posts           | table | postgres
 public | comments        | table | postgres
 public | schema_migrations | table | postgres
(4 rows)

postgres=# SELECT COUNT(*) FROM users;
 count 
-------
   100
(1 row)

postgres=# \q

[INFO] Disconnected from database
```

#### Database Backup
```bash
$ nself db backup
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                     DATABASE BACKUP                            ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Creating backup of database: postgres

⠋ Dumping schema...
[SUCCESS] Schema dumped

⠋ Dumping data...
[INFO] Backing up table: users (100 rows)
[INFO] Backing up table: posts (500 rows)
[INFO] Backing up table: comments (2000 rows)
[SUCCESS] Data dumped

⠋ Compressing backup...
[SUCCESS] Backup compressed

╔════════════════════════════════════════════════════════════════╗
║                    BACKUP COMPLETE                             ║
╚════════════════════════════════════════════════════════════════╝

Backup saved to: backups/postgres_20250810_145623.sql.gz
Size: 1.2 MB
```

#### Database Restore
```bash
$ nself db restore backups/postgres_20250810_145623.sql.gz
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    DATABASE RESTORE                            ║
╚════════════════════════════════════════════════════════════════╝

[WARNING] This will replace all existing data!
Continue? (y/N): y

⠋ Decompressing backup...
[SUCCESS] Backup decompressed

⠋ Dropping existing tables...
[INFO] Dropped 3 tables

⠋ Restoring schema...
[SUCCESS] Schema restored

⠋ Restoring data...
[INFO] Restored table: users (100 rows)
[INFO] Restored table: posts (500 rows)
[INFO] Restored table: comments (2000 rows)
[SUCCESS] Data restored

⠋ Verifying restore...
[SUCCESS] All tables verified

╔════════════════════════════════════════════════════════════════╗
║                   RESTORE COMPLETE                             ║
╚════════════════════════════════════════════════════════════════╝

Database restored from: backups/postgres_20250810_145623.sql.gz
```

---

### email - Configure Email

#### Configure SMTP
```bash
$ nself email smtp
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                   EMAIL CONFIGURATION                          ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Configuring SMTP email provider

Enter SMTP host (e.g., smtp.gmail.com): smtp.gmail.com
Enter SMTP port (587): 587
Enter SMTP username: user@example.com
Enter SMTP password: ****
Use TLS? (Y/n): y

⠋ Testing connection...
[SUCCESS] Connected to SMTP server

⠋ Sending test email...
[SUCCESS] Test email sent to user@example.com

⠋ Updating configuration...
[SUCCESS] Updated .env.local

╔════════════════════════════════════════════════════════════════╗
║                 EMAIL CONFIGURED                               ║
╚════════════════════════════════════════════════════════════════╝

SMTP email has been configured successfully.
Restart services to apply changes: nself restart auth
```

#### Configure SendGrid
```bash
$ nself email sendgrid --test
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                   EMAIL CONFIGURATION                          ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Configuring SendGrid email provider

Enter SendGrid API key: ****
Enter sender email: noreply@example.com
Enter sender name (My App): My App

⠋ Validating API key...
[SUCCESS] API key is valid

⠋ Verifying sender...
[SUCCESS] Sender email verified

⠋ Sending test email...
Enter recipient email: test@example.com
[SUCCESS] Test email sent successfully

⠋ Updating configuration...
[SUCCESS] Updated .env.local with SendGrid settings

╔════════════════════════════════════════════════════════════════╗
║                 SENDGRID CONFIGURED                            ║
╚════════════════════════════════════════════════════════════════╝

SendGrid has been configured and tested successfully.
```

---

### urls - Service URLs

#### Standard Output
```bash
$ nself urls
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                      SERVICE URLS                              ║
╚════════════════════════════════════════════════════════════════╝

Core Services
──────────────────────────────────────────────────
GraphQL API:       https://api.localhost
  Playground:      https://api.localhost/console
  Health:          https://api.localhost/healthz

Authentication:    https://auth.localhost
  Sign In:         https://auth.localhost/signin
  Sign Up:         https://auth.localhost/signup
  OAuth:           https://auth.localhost/oauth

Storage:           https://storage.localhost
  Console:         http://localhost:9001
  Access Key:      minioadmin
  Secret Key:      minioadmin

Optional Services
──────────────────────────────────────────────────
Redis Cache:       redis://localhost:6379
NestJS API:        https://nestjs.localhost     [ENABLED]
Dashboard:         https://dashboard.localhost  [DISABLED]
Functions:         https://functions.localhost  [DISABLED]

Development Tools
──────────────────────────────────────────────────
Database:          postgresql://postgres:****@localhost:5432/postgres
nself Admin:       https://admin.localhost      [DISABLED]
MailHog:           https://mailhog.localhost    [ENABLED]

Connection Examples
──────────────────────────────────────────────────
GraphQL Query:
  curl https://api.localhost/v1/graphql \
    -H 'x-hasura-admin-secret: ****' \
    -d '{"query":"{ users { id email }}"}'

PostgreSQL:
  psql postgresql://postgres:****@localhost:5432/postgres

Redis:
  redis-cli -h localhost -p 6379
```

#### JSON Output
```bash
$ nself urls --format json
```

**Output:**
```json
{
  "core": {
    "graphql": {
      "url": "https://api.localhost",
      "playground": "https://api.localhost/console",
      "health": "https://api.localhost/healthz"
    },
    "auth": {
      "url": "https://auth.localhost",
      "signin": "https://auth.localhost/signin",
      "signup": "https://auth.localhost/signup"
    },
    "storage": {
      "url": "https://storage.localhost",
      "console": "http://localhost:9001",
      "access_key": "minioadmin",
      "secret_key": "minioadmin"
    }
  },
  "optional": {
    "redis": {
      "url": "redis://localhost:6379",
      "enabled": true
    },
    "nestjs": {
      "url": "https://nestjs.localhost",
      "enabled": true
    },
    "dashboard": {
      "url": "https://dashboard.localhost",
      "enabled": false
    }
  },
  "database": {
    "connection_string": "postgresql://postgres:****@localhost:5432/postgres",
    "host": "localhost",
    "port": 5432,
    "database": "postgres",
    "username": "postgres"
  }
}
```

---

### prod - Production Setup

```bash
$ nself prod
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                   PRODUCTION CONFIGURATION                     ║
╚════════════════════════════════════════════════════════════════╝

[WARNING] This will configure your project for production deployment

Current environment: development
Target environment: production

Continue? (y/N): y

⠋ Generating secure passwords...
[SUCCESS] Generated 32-character passwords

⠋ Configuring domain...
Enter production domain (example.com): api.myapp.com
[SUCCESS] Domain configured: api.myapp.com

⠋ Configuring SSL...
Do you have SSL certificates? (y/N): n
[INFO] You'll need to obtain SSL certificates for:
  - api.api.myapp.com
  - auth.api.myapp.com
  - storage.api.myapp.com
[INFO] Recommended: Use Let's Encrypt with certbot

⠋ Configuring security...
[SUCCESS] Enabled security headers
[SUCCESS] Disabled GraphQL console
[SUCCESS] Enabled rate limiting
[SUCCESS] Configured CORS for production

⠋ Configuring resources...
[SUCCESS] Set production resource limits:
  - PostgreSQL: 4GB RAM, 10GB storage
  - Hasura: 2GB RAM
  - Auth: 1GB RAM
  - MinIO: 2GB RAM, 100GB storage
  - Nginx: 512MB RAM

⠋ Configuring backups...
Enable automated backups? (Y/n): y
[SUCCESS] Configured daily backups at 2:00 AM

⠋ Writing production configuration...
[SUCCESS] Created .env.production

╔════════════════════════════════════════════════════════════════╗
║                 PRODUCTION READY                               ║
╚════════════════════════════════════════════════════════════════╝

Production configuration complete!

Next steps:
1. Review .env.production
2. Obtain SSL certificates
3. Configure DNS records
4. Deploy with: ENV=production nself up -d

Security checklist:
✓ Strong passwords generated
✓ GraphQL console disabled
✓ Security headers enabled
✓ Rate limiting configured
✓ CORS configured
□ SSL certificates needed
□ Firewall rules needed
□ Monitoring setup needed
```

---

### trust - SSL Certificates

```bash
$ nself trust
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    SSL CERTIFICATE TRUST                       ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Managing SSL certificates for local development

⠋ Checking for existing certificates...
[SUCCESS] Found certificate: nginx/ssl/nself.org.crt
[SUCCESS] Certificate is valid

⠋ Installing certificate authority...
[INFO] Installing nself CA to system trust store
Password: ****
[SUCCESS] CA installed to system keychain

⠋ Trusting certificate...
[SUCCESS] Certificate trusted for:
  - *.localhost
  - *.local.nself.org
  - localhost

⠋ Verifying trust...
[SUCCESS] Certificate verification passed

╔════════════════════════════════════════════════════════════════╗
║                   CERTIFICATES TRUSTED                         ║
╚════════════════════════════════════════════════════════════════╝

Your browser will now trust nself certificates.
You may need to restart your browser.
```

---

## Development Commands

### scaffold - Create Services

#### Create NestJS Service
```bash
$ nself scaffold nest api-gateway
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    SERVICE SCAFFOLDING                         ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Creating NestJS service: api-gateway

⠋ Creating service directory...
[SUCCESS] Created services/api-gateway/

⠋ Copying template files...
[SUCCESS] Copied NestJS template

⠋ Updating configuration...
[INFO] Replacing placeholders:
  {{SERVICE_NAME}} -> api-gateway
  {{PORT}} -> 3001
  {{DATABASE_URL}} -> postgresql://...
[SUCCESS] Configuration updated

⠋ Generating package.json...
[SUCCESS] Generated package.json with dependencies

⠋ Creating Dockerfile...
[SUCCESS] Created multi-stage Dockerfile

⠋ Registering service...
[SUCCESS] Added to .env.local:
  NESTJS_ENABLED=true
  NESTJS_PORT=3001
  NESTJS_ROUTE=api-gateway.localhost

⠋ Updating docker-compose...
[SUCCESS] Service added to docker-compose.yml

╔════════════════════════════════════════════════════════════════╗
║                   SERVICE CREATED                              ║
╚════════════════════════════════════════════════════════════════╝

NestJS service created: services/api-gateway/

Next steps:
1. cd services/api-gateway
2. npm install
3. nself build
4. nself up

Service will be available at:
  https://api-gateway.localhost
```

#### Create BullMQ Worker with Auto-Start
```bash
$ nself scaffold bull email-worker --start
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    SERVICE SCAFFOLDING                         ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Creating BullMQ worker: email-worker

⠋ Creating service directory...
[SUCCESS] Created services/email-worker/

⠋ Copying template files...
[SUCCESS] Copied BullMQ template

⠋ Configuring worker...
[INFO] Queue name: email-queue
[INFO] Redis connection: redis://localhost:6379
[SUCCESS] Worker configured

⠋ Creating job processors...
[SUCCESS] Created processors:
  - send-email.js
  - send-bulk.js
  - retry-failed.js

⠋ Registering service...
[SUCCESS] Added to .env.local

⠋ Starting service...
[INFO] Building Docker image...
[SUCCESS] Image built: email-worker:latest

[INFO] Starting container...
[SUCCESS] Worker started

╔════════════════════════════════════════════════════════════════╗
║                 WORKER CREATED & STARTED                       ║
╚════════════════════════════════════════════════════════════════╝

BullMQ worker running: email-worker
Queue: email-queue
Redis: redis://localhost:6379

Monitor queue: nself logs email-worker -f
```

---

### diff - Configuration Diff

```bash
$ nself diff
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                  CONFIGURATION DIFFERENCES                     ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Comparing: .env.local vs .env.example

--- .env.example
+++ .env.local
@@ -1,20 +1,20 @@
 # Project Configuration
-PROJECT_NAME=EXAMPLE
-BASE_DOMAIN=EXAMPLE
-ENV=EXAMPLE
+PROJECT_NAME=myapp
+BASE_DOMAIN=api.myapp.com
+ENV=production

 # Database Configuration
-POSTGRES_PASSWORD=EXAMPLE
+POSTGRES_PASSWORD=xK9mP2nQ8vL4jF6
 POSTGRES_USER=postgres
 POSTGRES_DB=postgres

 # Hasura Configuration
-HASURA_GRAPHQL_ADMIN_SECRET=EXAMPLE
-HASURA_GRAPHQL_JWT_SECRET=EXAMPLE
+HASURA_GRAPHQL_ADMIN_SECRET=aB3cD4eF5gH6iJ7
+HASURA_GRAPHQL_JWT_SECRET={"type":"HS256","key":"..."}

 # Optional Services
-REDIS_ENABLED=EXAMPLE
+REDIS_ENABLED=true
+NESTJS_ENABLED=true
+NESTJS_PORT=3001

[INFO] Summary:
  .env.local: 24 variables
  .env.example: 20 variables

[WARNING] Missing variables in .env.local:
  - MONITORING_ENABLED
  - BACKUP_ENABLED
```

---

### reset - Reset Project

```bash
$ nself reset
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                     PROJECT RESET                              ║
╚════════════════════════════════════════════════════════════════╝

[WARNING] This will stop all services and remove containers

Data will be preserved. Use --hard to remove data.

Continue? (y/N): y

⠋ Stopping services...
[SUCCESS] Services stopped

⠋ Removing containers...
[SUCCESS] Containers removed

⠋ Removing networks...
[SUCCESS] Networks removed

⠋ Cleaning build cache...
[SUCCESS] Build cache cleared

╔════════════════════════════════════════════════════════════════╗
║                    RESET COMPLETE                              ║
╚════════════════════════════════════════════════════════════════╝

Project has been reset. Data is preserved.
Run 'nself up' to start fresh.
```

#### Hard Reset
```bash
$ nself reset --hard
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                     PROJECT RESET                              ║
╚════════════════════════════════════════════════════════════════╝

[WARNING] HARD RESET: This will DELETE ALL DATA!

This action will remove:
  - All containers
  - All volumes (DATABASE DATA)
  - All networks
  - Build cache
  - Generated files

Type 'DELETE' to confirm: DELETE

⠋ Stopping services...
[SUCCESS] Services stopped

⠋ Removing containers...
[SUCCESS] Containers removed

⠋ Removing volumes...
[WARNING] Deleting data volumes...
[SUCCESS] Deleted: nself_postgres_data (2.3 GB)
[SUCCESS] Deleted: nself_minio_data (1.2 GB)
[SUCCESS] All volumes removed

⠋ Removing networks...
[SUCCESS] Networks removed

⠋ Removing generated files...
[SUCCESS] Removed docker-compose.yml
[SUCCESS] Removed nginx configs

╔════════════════════════════════════════════════════════════════╗
║                  HARD RESET COMPLETE                           ║
╚════════════════════════════════════════════════════════════════╝

All data has been deleted.
Run 'nself init' to start completely fresh.
```

---

### validate-env - Validate Configuration

```bash
$ nself validate-env
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                 ENVIRONMENT VALIDATION                         ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Validating: .env.local

⠋ Checking required variables...
[SUCCESS] All required variables present

⠋ Validating variable formats...
[SUCCESS] PROJECT_NAME: myapp (valid)
[SUCCESS] BASE_DOMAIN: localhost (valid)
[SUCCESS] ENV: dev (valid)
[WARNING] POSTGRES_PASSWORD: weak password (8 chars)
[SUCCESS] HASURA_GRAPHQL_ADMIN_SECRET: ******** (valid)
[ERROR] HASURA_GRAPHQL_JWT_SECRET: invalid JSON

⠋ Checking variable dependencies...
[SUCCESS] REDIS_ENABLED requires REDIS_PORT: ✓
[WARNING] MONITORING_ENABLED=true but GRAFANA_PASSWORD not set

⠋ Checking for conflicts...
[SUCCESS] No port conflicts detected
[SUCCESS] No service conflicts detected

⠋ Validating secrets...
[WARNING] Using default MinIO credentials (change for production)
[WARNING] JWT secret should be at least 32 characters

╔════════════════════════════════════════════════════════════════╗
║                  VALIDATION RESULTS                            ║
╚════════════════════════════════════════════════════════════════╝

Status: FAILED
  ✓ 15 valid configurations
  ⚠ 4 warnings
  ✗ 1 error

Errors must be fixed:
  - HASURA_GRAPHQL_JWT_SECRET: Invalid JSON format

Warnings (recommended to fix):
  - Weak database password
  - Missing GRAFANA_PASSWORD
  - Default MinIO credentials
  - Short JWT secret

Fix errors and run validation again.
```

---

### hot-reload - Development Mode

```bash
$ nself hot-reload
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    HOT RELOAD MODE                             ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Enabling hot reload for development

⠋ Detecting services...
[SUCCESS] Found services:
  - NestJS (services/api-gateway)
  - React (services/dashboard)
  - Hasura metadata

⠋ Installing watchers...
[SUCCESS] Watching services/api-gateway/**/*.ts
[SUCCESS] Watching services/dashboard/**/*.{js,jsx,ts,tsx}
[SUCCESS] Watching hasura/metadata/**/*.yaml

⠋ Starting development mode...
[SUCCESS] Services restarted with hot reload

╔════════════════════════════════════════════════════════════════╗
║                   HOT RELOAD ACTIVE                            ║
╚════════════════════════════════════════════════════════════════╝

Watching for changes... (Ctrl+C to stop)

[12:34:56] File changed: services/api-gateway/src/app.service.ts
[12:34:56] Rebuilding api-gateway...
[12:34:58] Build complete, restarting...
[12:34:59] Service restarted ✓

[12:35:23] File changed: hasura/metadata/tables.yaml
[12:35:23] Reloading Hasura metadata...
[12:35:24] Metadata reloaded ✓

[12:36:45] File changed: services/dashboard/src/App.tsx
[12:36:45] Hot module replacement...
[12:36:46] Dashboard updated ✓
```

---

## System Commands

### update - Update nself

```bash
$ nself update
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    nself Update Check                          ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Current version: 0.3.0
[INFO] Checking for updates...

⠋ Fetching latest version...
[SUCCESS] Latest version: 0.4.0

[INFO] Changelog:
  v0.4.0 (2025-08-15)
  - Added Kubernetes deployment option
  - Improved auto-fix system
  - New 'nself deploy' command
  - Performance improvements
  - Bug fixes

Update to v0.4.0? (Y/n): y

⠋ Downloading update...
[SUCCESS] Downloaded v0.4.0

⠋ Backing up current installation...
[SUCCESS] Backup created: ~/.nself.backup/

⠋ Installing update...
[SUCCESS] Files updated
[SUCCESS] Permissions set
[SUCCESS] PATH verified

⠋ Running post-update tasks...
[SUCCESS] Configuration migrated
[SUCCESS] Templates updated

╔════════════════════════════════════════════════════════════════╗
║                    UPDATE COMPLETE                             ║
╚════════════════════════════════════════════════════════════════╝

nself has been updated to v0.4.0
Run 'nself version' to verify
```

#### Check for Updates Only
```bash
$ nself update --check
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    UPDATE CHECK                                ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Current version: 0.3.0
[INFO] Checking for updates...

⠋ Fetching latest version...
[SUCCESS] Latest version: 0.4.0

[INFO] Update available: 0.3.0 → 0.4.0

Run 'nself update' to install the latest version
```

---

### version - Version Info

#### Simple Version
```bash
$ nself version
```

**Output:**
```
nself version 0.3.0
```

#### Verbose Version
```bash
$ nself version --verbose
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                  nself Version Information                     ║
╚════════════════════════════════════════════════════════════════╝

Version:     0.3.0
Location:    /Users/admin/.nself/bin
Config:      .env.local

System Information:
  OS:        Darwin
  Arch:      arm64
  Shell:     5.9
  Docker:    28.0.4
  Compose:   v2.29.1

Installation:
  Installed: 2025-07-15
  Updated:   2025-08-10
  Channel:   stable

Components:
  ✓ Core commands
  ✓ Shared utilities
  ✓ Docker wrapper
  ✓ Auto-fix system
  ✓ Hooks system
  ✓ Templates
```

---

### help - Help Text

#### General Help
```bash
$ nself help
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║               nself - Modern Full-Stack Platform               ║
╚════════════════════════════════════════════════════════════════╝

Version: 0.3.0

Usage: nself <command> [options]

Core Commands
  init          Initialize a new project
  build         Build project structure and Docker images
  up            Start all services
  down          Stop all services
  restart       Restart all services
  status        Show service status
  logs          View service logs

Management Commands
  doctor        Run system diagnostics
  db            Database operations
  email         Email service configuration
  urls          Show service URLs
  prod          Configure for production deployment
  trust         Manage SSL certificates

Development Commands
  diff          Show configuration differences
  reset         Reset project to clean state

Tool Commands
  scaffold      Create new service from template
  validate-env  Validate environment configuration
  hot-reload    Enable hot reload for development

Other Commands
  update        Update nself to latest version
  version       Show version information
  help          Show this help message

For command-specific help: nself help <command>
                      or: nself <command> --help
```

#### Command-Specific Help
```bash
$ nself help db
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                    DATABASE OPERATIONS                         ║
╚════════════════════════════════════════════════════════════════╝

Usage: nself db <subcommand> [options]

Subcommands:
  migrate       Run pending database migrations
  seed          Load seed data into database
  reset         Reset database to clean state
  backup        Create database backup
  restore       Restore database from backup
  console       Open interactive PostgreSQL console

Options:
  --help        Show this help message
  --force       Skip confirmation prompts
  --verbose     Show detailed output

Examples:
  nself db migrate              # Run migrations
  nself db seed                 # Load test data
  nself db backup               # Create backup
  nself db restore backup.sql   # Restore from file
  nself db console              # Interactive SQL
  nself db reset --force        # Reset without prompt

Migration Management:
  Migrations are located in: hasura/migrations/
  Format: <version>_<description>.sql
  Applied migrations are tracked in schema_migrations table

Backup/Restore:
  Backups are saved to: backups/
  Format: postgres_YYYYMMDD_HHMMSS.sql.gz
  Backups include both schema and data

Notes:
  - Database must be running (nself up)
  - Backups are compressed with gzip
  - Console requires psql client
```

---

## Error Examples

### Port Conflict Error
```bash
$ nself up
```

**Output with Error:**
```
╔════════════════════════════════════════════════════════════════╗
║                     STARTING SERVICES                          ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Checking prerequisites...
[ERROR] Port 5432 is already in use by another process

[INFO] Attempting auto-fix...
[INFO] Process using port 5432: postgres (PID: 1234)
[WARNING] Port is used by system PostgreSQL, cannot auto-fix

╔════════════════════════════════════════════════════════════════╗
║                      START FAILED                              ║
╚════════════════════════════════════════════════════════════════╝

Failed to start services: Port conflict

Solutions:
1. Stop system PostgreSQL: sudo systemctl stop postgresql
2. Change port in .env.local: POSTGRES_PORT=5433
3. Use Docker's PostgreSQL only

Run 'nself doctor' for detailed diagnostics
```

### Configuration Error
```bash
$ nself build
```

**Output with Error:**
```
╔════════════════════════════════════════════════════════════════╗
║                    BUILDING INFRASTRUCTURE                     ║
╚════════════════════════════════════════════════════════════════╝

[ERROR] Configuration error in .env.local

Invalid configuration:
  Line 15: POSTGRES_PASSWORD=    (empty value)
  Line 23: INVALID SYNTAX HERE   (not key=value format)
  Line 45: REDIS_ENABLED=yes     (must be true/false)

╔════════════════════════════════════════════════════════════════╗
║                      BUILD FAILED                              ║
╚════════════════════════════════════════════════════════════════╝

Fix configuration errors and try again.
Run 'nself validate-env' for detailed validation
```

### Docker Not Running
```bash
$ nself up
```

**Output with Error:**
```
╔════════════════════════════════════════════════════════════════╗
║                     STARTING SERVICES                          ║
╚════════════════════════════════════════════════════════════════╝

[INFO] Checking prerequisites...
[ERROR] Docker daemon is not running

╔════════════════════════════════════════════════════════════════╗
║                   DOCKER NOT AVAILABLE                         ║
╚════════════════════════════════════════════════════════════════╝

Docker Desktop is not running.

To fix:
  macOS/Windows: Start Docker Desktop application
  Linux: sudo systemctl start docker

After starting Docker, run 'nself up' again
```

---

## Tips and Best Practices

1. **Always run doctor first** when encountering issues
2. **Use detached mode** (`-d`) for production
3. **Enable hot-reload** during development
4. **Validate environment** before deploying
5. **Create backups** before major changes
6. **Monitor logs** with `nself logs -f`
7. **Use scaffold** for consistent service creation
8. **Run prod command** before production deployment

---

This comprehensive guide shows the exact output format for every nself command, including success cases, error cases, and interactive prompts.