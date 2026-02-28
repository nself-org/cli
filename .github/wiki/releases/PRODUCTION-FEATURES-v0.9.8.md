# Production Features - nself v0.9.8

## Overview

Version 0.9.8 introduces comprehensive production-ready features to make nself deployments robust, reliable, and enterprise-grade. This release focuses on operational excellence with automated monitoring, health checks, resource management, and deployment safety.

## New Features

### 1. Comprehensive Health Endpoints

**Location**: `/src/lib/services/health-endpoints.sh`

All services now support three-tier health checking:

#### `/health` - Liveness Probe
- Quick check if service is alive
- Returns 200 OK if service can accept requests
- Used by orchestrators to detect crashed services

#### `/ready` - Readiness Probe
- Checks all dependencies (database, Redis, etc.)
- Returns 200 OK only when fully ready to serve traffic
- Used by load balancers to route traffic

#### `/status` - Detailed Diagnostics
- Full service metrics and diagnostics
- Resource usage (CPU, memory)
- Dependency status with response times
- Uptime and version information

**Supported Languages**:
- Node.js/Express
- Python/Flask
- Go

**Usage**:
```javascript
// Node.js example
GET /health   // {"status": "healthy", "service": "api", "version": "1.0.0"}
GET /ready    // Checks database, redis, etc.
GET /status   // Full diagnostics
```

### 2. Graceful Shutdown Handlers

**Location**: `/src/lib/services/graceful-shutdown.sh`

Prevents data loss and request failures during deployments:

**Features**:
- Stops accepting new connections immediately
- Waits for in-flight requests to complete (30s timeout)
- Closes database/cache connections cleanly
- Flushes buffered logs
- Handles SIGTERM, SIGINT signals

**Supported Languages**:
- Node.js/Express
- Python/Flask
- Go

**Behavior**:
1. Signal received (SIGTERM/SIGINT)
2. Stop accepting new requests (return 503)
3. Wait for active requests to finish
4. Close database connections
5. Close Redis connections
6. Flush logs
7. Exit cleanly

### 3. Auto-Calculate Resource Limits

**Location**: `/src/lib/docker/resources.sh`

Automatically calculates optimal Docker resource limits based on system resources and service count.

**Features**:
- Detects available system memory and CPU
- Calculates per-service limits based on priority
- Accounts for service type (database gets more resources)
- Ensures minimum requirements are met
- Allows manual override via environment variables

**Service Priorities** (1-10 scale):
- PostgreSQL: 10 (highest)
- Hasura: 8
- MinIO/MeiliSearch: 7
- Redis: 6
- Custom services: 5
- Nginx: 4

**Usage**:
```bash
# Check minimum requirements
nself resources check

# Calculate and show allocation
nself resources calculate

# Apply to docker-compose.yml
DOCKER_RESOURCE_LIMITS=true nself build
```

**Example Output**:
```
System Resources:
  Total Memory: 16384 MB
  Total CPU: 8 cores
  Services: 12

Resource Allocation:
  Service              Mem Limit    Mem Reserve  CPU Limit    CPU Reserve
  -------              ---------    -----------  ---------    -----------
  postgres             2048MB       1024MB       2000m        1000m
  hasura               1024MB       512MB        1000m        500m
  redis                512MB        256MB        500m         250m
```

### 4. Log Rotation Configuration

**Location**: `/src/lib/logging/logrotate.sh`

Automated log management to prevent disk space issues.

**Features**:
- Daily rotation
- Configurable retention (default: 7 days)
- Compression of old logs
- Per-service configuration
- Docker-compatible (sends SIGHUP to containers)

**Setup**:
```bash
# Generate logrotate configs
bash src/lib/logging/logrotate.sh generate

# Install system-wide (requires sudo)
sudo bash src/lib/logging/logrotate.sh install

# Setup log directories
bash src/lib/logging/logrotate.sh setup

# Cleanup old logs manually
bash src/lib/logging/logrotate.sh cleanup ./logs 7

# Check disk usage
bash src/lib/logging/logrotate.sh usage
```

**Configuration**:
```bash
# Environment variables
LOG_RETENTION_DAYS=7
LOG_COMPRESS=true
LOG_MAX_SIZE=100M
```

### 5. Automated Backup Scheduling

**Location**: `/src/cli/backup.sh` (enhanced)

Cron-based automated backups with flexible scheduling.

**Features**:
- Hourly, daily, weekly, monthly schedules
- Multiple backup types (full, database, config)
- Automatic pruning of old backups
- Email notifications on failure
- Backup verification

**Usage**:
```bash
# Create backup schedule
nself backup schedule create daily              # Daily at 2 AM
nself backup schedule create hourly             # Every hour
nself backup schedule create weekly             # Sundays at 2 AM

# Manage schedules
nself backup schedule list                      # Show all schedules
nself backup schedule enable daily              # Enable schedule
nself backup schedule disable daily             # Disable schedule
nself backup schedule remove daily              # Delete schedule
nself backup schedule status                    # Show recent activity

# Manual backup
nself backup create full                        # Create backup now
```

**Cron Format**:
```cron
# Daily backup at 2 AM
0 2 * * * cd /path/to/project && nself backup create full

# Hourly backup
0 * * * * cd /path/to/project && nself backup create database
```

### 6. Default Alert Rules

**Location**: `/src/lib/monitoring/alert-rules.sh`

Pre-configured Prometheus alerts for production monitoring.

**Alert Categories**:

#### Infrastructure Alerts (Critical)
- `ServiceDown` - Service not responding (1m)
- `HighCPUUsage` - CPU > 80% (5m)
- `HighMemoryUsage` - Memory > 90% (5m)
- `LowDiskSpace` - Disk < 10% free (5m)

#### Database Alerts
- `PostgreSQLDown` - Database not responding
- `PostgreSQLTooManyConnections` - > 80% connections used
- `PostgreSQLSlowQueries` - Average query time > 1s
- `PostgreSQLDeadlocks` - Deadlocks detected

#### Cache Alerts
- `RedisDown` - Redis not responding
- `RedisHighMemoryUsage` - > 90% memory used
- `RedisTooManyConnections` - > 1000 clients

#### Container Alerts
- `ContainerHighCPU` - Container CPU > 80%
- `ContainerHighMemory` - Container memory > 90%
- `ContainerRestarting` - Container restart loop

#### Security Alerts
- `SSLCertificateExpiringSoon` - Expires in < 30 days
- `SSLCertificateExpired` - Certificate expired

#### Application Alerts
- `HighErrorRate` - 5xx errors > 5%
- `SlowResponseTime` - P95 latency > 1s

#### Backup Alerts
- `BackupFailed` - No successful backup in 24h
- `NoRecentBackup` - No backup in 48h (critical)

**Setup**:
```bash
# Generate alert rules
bash src/lib/monitoring/alert-rules.sh rules

# Generate Alertmanager config
bash src/lib/monitoring/alert-rules.sh alertmanager

# Generate both
bash src/lib/monitoring/alert-rules.sh all
```

**Configuration**:
```bash
# .env configuration for notifications
SLACK_WEBHOOK_URL=https://hooks.slack.com/services/YOUR/WEBHOOK
ALERT_EMAIL_TO=ops@example.com
ALERT_EMAIL_FROM=nself@example.com
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-email@gmail.com
SMTP_PASSWORD=your-app-password
```

### 7. Automated Rollback

**Location**: `/src/lib/deploy/rollback.sh`

Health-check based automatic rollback on failed deployments.

**Features**:
- Creates snapshot before every deployment
- Runs health checks after deployment
- Automatically rolls back on failure
- Keeps last 3 deployment snapshots
- Zero-downtime rollback

**Workflow**:
1. Create pre-deployment snapshot
2. Execute deployment
3. Wait for services to stabilize
4. Run health checks (timeout: 120s, required: 80%)
5. If health checks fail → automatic rollback
6. If health checks pass → cleanup old snapshots

**Usage**:
```bash
# Deploy with automatic rollback
bash src/lib/deploy/rollback.sh deploy "docker compose up -d"

# Create snapshot manually
bash src/lib/deploy/rollback.sh create-snapshot

# List snapshots
bash src/lib/deploy/rollback.sh list

# Rollback to previous deployment
bash src/lib/deploy/rollback.sh rollback

# Rollback to specific snapshot
bash src/lib/deploy/rollback.sh rollback deployment_20240131_120000

# Run health checks manually
bash src/lib/deploy/rollback.sh health-check

# Cleanup old snapshots
bash src/lib/deploy/rollback.sh cleanup
```

**Configuration**:
```bash
ROLLBACK_DIR=.nself/rollback
ROLLBACK_RETENTION=3
HEALTH_CHECK_TIMEOUT=120
HEALTH_CHECK_INTERVAL=5
HEALTH_CHECK_REQUIRED_PERCENT=80
```

### 8. PgBouncer Auto-Configuration

**Location**: `/src/lib/database/pgbouncer.sh`

Intelligent connection pooling configuration based on workload.

**Features**:
- Auto-calculates pool sizes based on service count
- Determines optimal pool mode based on workload type
- Generates configuration files
- Creates MD5 password hashes
- Monitors pool statistics

**Pool Mode Selection**:
- `transaction` - Read-heavy workloads (default)
- `session` - Write-heavy/analytical workloads
- `statement` - Serverless/short queries

**Usage**:
```bash
# Complete setup
bash src/lib/database/pgbouncer.sh setup

# Calculate optimal pool sizes
bash src/lib/database/pgbouncer.sh calculate

# Enable in .env
bash src/lib/database/pgbouncer.sh enable

# Generate configs only
bash src/lib/database/pgbouncer.sh generate-ini
bash src/lib/database/pgbouncer.sh generate-userlist

# Show statistics
bash src/lib/database/pgbouncer.sh stats
```

**Configuration**:
```bash
# .env settings
PGBOUNCER_ENABLED=true
PGBOUNCER_PORT=6432
PGBOUNCER_POOL_MODE=transaction
EXPECTED_CONCURRENT_USERS=100
WORKLOAD_TYPE=mixed  # read-heavy, write-heavy, analytical, mixed
```

**Connection String**:
```
# Before (direct to PostgreSQL)
postgres://user:pass@postgres:5432/database

# After (through PgBouncer)
postgres://user:pass@pgbouncer:6432/database
```

### 9. Production Deployment Checklist

**Location**: `/src/cli/checklist.sh`

Automated verification of production readiness.

**Checks Performed**:
1. ✓ SSL certificates valid and not expiring
2. ✓ Backups configured and recent
3. ✓ Monitoring active with alerts
4. ✓ Resource limits defined
5. ✓ Secrets properly configured
6. ✓ Firewall enabled
7. ✓ Log rotation configured
8. ✓ Health endpoints responding
9. ✓ Database properly tuned
10. ✓ Security headers configured

**Usage**:
```bash
# Run checklist
nself checklist

# Auto-fix issues
nself checklist --fix

# Verbose output
nself checklist --verbose

# JSON format
nself checklist --json
```

**Example Output**:
```
Production Readiness Checklist

[PASS]  ssl certificates
[PASS]  backups
[WARN]  monitoring
         Monitoring not enabled (set MONITORING_ENABLED=true in .env)
[PASS]  resource limits
[FAIL]  secrets
         Security issues found:
           - PostgreSQL using default password
           - Hasura using default admin secret
[WARN]  firewall
         No firewall detected (recommended for production)
[PASS]  log rotation
[PASS]  health endpoints
[WARN]  database tuning
         PgBouncer not enabled (recommended for production)
[PASS]  security headers

Summary

  ✓ Passed:   7 / 10
  ⚠ Warnings: 2
  ✗ Failed:   1

✗ 1 critical issue(s) found. Fix these before deploying!
```

## Integration with Existing Commands

### Enhanced Commands

#### `nself build`
Now supports resource limit generation:
```bash
DOCKER_RESOURCE_LIMITS=true nself build
```

#### `nself backup`
New `schedule` subcommand:
```bash
nself backup schedule create daily
nself backup schedule list
```

#### `nself start`
Now uses health checks from health endpoints.

### New Commands

#### `nself checklist`
Production readiness verification:
```bash
nself checklist
nself checklist --fix
```

## Environment Variables

### New Configuration Options

```bash
# Resource Limits
DOCKER_RESOURCE_LIMITS=true
POSTGRES_MEMORY_MB=2048
POSTGRES_CPU_MILLICORES=2000

# Logging
LOG_RETENTION_DAYS=7
LOG_COMPRESS=true
LOG_MAX_SIZE=100M

# Backups
BACKUP_RETENTION_DAYS=30
BACKUP_RETENTION_COUNT=10
BACKUP_RETENTION_MIN=3

# Monitoring & Alerts
SLACK_WEBHOOK_URL=https://hooks.slack.com/...
ALERT_EMAIL_TO=ops@example.com
SMTP_HOST=smtp.gmail.com

# Rollback
ROLLBACK_RETENTION=3
HEALTH_CHECK_TIMEOUT=120
HEALTH_CHECK_REQUIRED_PERCENT=80

# PgBouncer
PGBOUNCER_ENABLED=true
PGBOUNCER_PORT=6432
PGBOUNCER_POOL_MODE=transaction
EXPECTED_CONCURRENT_USERS=100
WORKLOAD_TYPE=mixed
```

## Production Deployment Workflow

### Recommended Flow

```bash
# 1. Run production checklist
nself checklist --fix

# 2. Setup automated backups
nself backup schedule create daily

# 3. Configure monitoring alerts
bash src/lib/monitoring/alert-rules.sh all

# 4. Setup log rotation
bash src/lib/logging/logrotate.sh setup
bash src/lib/logging/logrotate.sh generate

# 5. Enable PgBouncer
bash src/lib/database/pgbouncer.sh setup
bash src/lib/database/pgbouncer.sh enable

# 6. Build with resource limits
DOCKER_RESOURCE_LIMITS=true nself build

# 7. Deploy with automatic rollback
bash src/lib/deploy/rollback.sh deploy "docker compose up -d"

# 8. Verify deployment
nself checklist
nself health check
```

## Minimum System Requirements

After implementing resource auto-calculation:

**Development**:
- 2GB RAM
- 2 CPU cores
- 10GB disk space

**Production (basic)**:
- 4GB RAM
- 4 CPU cores
- 50GB disk space

**Production (recommended)**:
- 8GB+ RAM
- 8+ CPU cores
- 100GB+ disk space

## Backward Compatibility

All features are **opt-in** and backward compatible:
- Existing deployments continue to work
- No breaking changes to existing commands
- New features enabled via environment variables

## Testing

All production features have been tested on:
- Ubuntu 22.04 LTS
- macOS (latest)
- Docker 24.x
- Docker Compose v2.x

## Documentation

Full documentation available at:
- `/docs/production/HEALTH-ENDPOINTS.md`
- `/docs/production/GRACEFUL-SHUTDOWN.md`
- `/docs/production/RESOURCE-LIMITS.md`
- `/docs/production/LOG-ROTATION.md`
- `/docs/production/BACKUP-SCHEDULING.md`
- `/docs/production/ALERT-RULES.md`
- `/docs/production/AUTOMATED-ROLLBACK.md`
- `/docs/production/PGBOUNCER.md`
- `/docs/production/DEPLOYMENT-CHECKLIST.md`

## Migration Guide

### From v0.9.7 to v0.9.8

1. **Update nself**:
   ```bash
   nself update
   ```

2. **Run checklist**:
   ```bash
   nself checklist
   ```

3. **Enable new features** (optional):
   ```bash
   # Add to .env
   DOCKER_RESOURCE_LIMITS=true
   MONITORING_ENABLED=true
   PGBOUNCER_ENABLED=true
   LOG_RETENTION_DAYS=7
   ```

4. **Rebuild**:
   ```bash
   nself build
   nself restart
   ```

5. **Setup backups**:
   ```bash
   nself backup schedule create daily
   ```

## Contributors

- Production features development
- Health endpoint templates
- Resource calculation algorithms
- Alert rule definitions
- Rollback automation

## License

MIT License - See LICENSE file for details

---

**Release Date**: February 2024
**Version**: 0.9.8
**Codename**: Production Ready
