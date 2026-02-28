# nself doctor

**Category**: Utilities

Diagnose common issues and provide automated fixes for nself projects.

## Overview

The doctor command runs comprehensive diagnostics to detect and fix common configuration, environment, and service issues.

**Features**:
- ✅ Automated issue detection
- ✅ One-click fixes
- ✅ Environment validation
- ✅ Dependency checks
- ✅ Performance analysis
- ✅ Security audits

## Usage

```bash
nself doctor [OPTIONS]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `--fix` | Automatically apply fixes | false |
| `--category CAT` | Check specific category only | all |
| `--skip-fix` | Skip fix suggestions | false |
| `-v, --verbose` | Show detailed diagnostics | false |
| `--export FILE` | Export report to file | - |

## Examples

### Basic Diagnostic

```bash
nself doctor
```

**Output**:
```
╔═══════════════════════════════════════════════════════════╗
║              nself Project Diagnostics                    ║
╚═══════════════════════════════════════════════════════════╝

Running diagnostics...

Environment
──────────────────────────────────────────────────────────
✓ Docker installed (v24.0.5)
✓ Docker Compose installed (v2.20.2)
✓ Node.js installed (v20.10.0)
✓ Git installed (v2.40.1)
✗ Port 5432 in use (conflict with existing PostgreSQL)

Configuration
──────────────────────────────────────────────────────────
✓ .env file exists
✓ Required variables present
✗ HASURA_GRAPHQL_ADMIN_SECRET too weak (< 32 characters)
⚠ POSTGRES_PASSWORD uses default value

Services
──────────────────────────────────────────────────────────
✓ PostgreSQL running
✓ Hasura running
✗ Redis not running (REDIS_ENABLED=true but service down)
✓ Nginx running

Performance
──────────────────────────────────────────────────────────
✓ Disk space sufficient (120 GB free)
⚠ Memory usage high (7.2 GB / 8 GB = 90%)
✓ CPU usage normal (12%)

Security
──────────────────────────────────────────────────────────
✗ .env file has insecure permissions (644)
⚠ Self-signed SSL certificate (expected in dev)
✓ No exposed secrets in git history

Summary
──────────────────────────────────────────────────────────
Issues found: 3 critical, 2 warnings

Critical Issues:
1. Port conflict: PostgreSQL (port 5432)
2. Redis service not running
3. .env file permissions insecure

Warnings:
1. Weak admin secret
2. High memory usage

Run 'nself doctor --fix' to automatically apply fixes
```

### Auto-Fix Issues

```bash
nself doctor --fix
```

**Output**:
```
Running diagnostics and applying fixes...

Fixing: Port conflict (PostgreSQL)
  → Changing POSTGRES_PORT to 5433 in .env
  ✓ Fixed

Fixing: Redis service not running
  → Starting Redis container
  ✓ Fixed

Fixing: .env file permissions
  → chmod 600 .env
  ✓ Fixed

Fixing: Weak admin secret
  → Generating new 64-character secret
  → Updating .env
  ✓ Fixed

3/3 critical issues fixed
2/2 warnings addressed

Run 'nself restart' to apply configuration changes
```

### Check Specific Category

```bash
nself doctor --category security
```

**Output**:
```
Security Diagnostics

✗ .env file permissions: 644 (should be 600)
✗ Secrets in git history: found HASURA_ADMIN_SECRET in commit abc123
⚠ Weak passwords detected:
  - POSTGRES_PASSWORD: "postgres" (default)
  - REDIS_PASSWORD: not set
✓ SSL certificates: valid
✓ Rate limiting: enabled
✓ CORS configuration: secure

Recommendations:
1. Run: chmod 600 .env
2. Rotate exposed secrets
3. Use strong passwords (32+ characters)
4. Enable Redis password
```

### Verbose Diagnostics

```bash
nself doctor --verbose
```

**Shows detailed information for every check.**

### Export Report

```bash
nself doctor --export diagnostics-report.txt
```

**Saves full diagnostic report to file.**

## Diagnostic Categories

### Environment

Checks system dependencies and tools.

**Validates**:
- Docker and Docker Compose installation
- Node.js/npm (if using custom services)
- Git (for version control)
- Port availability
- Disk space
- System resources

**Common Issues**:
```
✗ Docker not running
  Fix: Start Docker Desktop

✗ Port 8080 in use
  Fix: Change HASURA_PORT in .env or stop conflicting service

✗ Insufficient disk space (< 5 GB)
  Fix: Free up disk space or clean old Docker images
```

### Configuration

Validates .env files and settings.

**Validates**:
- Required environment variables present
- Variable formats (ports, URLs, emails)
- Secret strength (admin secrets, passwords)
- Service dependencies (e.g., MLflow requires MinIO)
- Domain/URL formats

**Common Issues**:
```
✗ Missing required variable: HASURA_GRAPHQL_ADMIN_SECRET
  Fix: Add to .env

✗ Invalid port: POSTGRES_PORT=abc (not a number)
  Fix: Set to valid port number

✗ Service dependency: MLFLOW_ENABLED=true requires MINIO_ENABLED=true
  Fix: Enable MinIO or disable MLflow
```

### Services

Checks running Docker containers and health.

**Validates**:
- All enabled services running
- Container health status
- Service connectivity
- Resource usage per container

**Common Issues**:
```
✗ Service enabled but not running: redis
  Fix: nself start redis

✗ Container constantly restarting: hasura
  Fix: Check logs with 'nself logs hasura'

✗ Unhealthy service: postgres
  Fix: Check health with 'nself health postgres'
```

### Performance

Analyzes system and service performance.

**Validates**:
- System resource usage (CPU, memory, disk)
- Database performance (query times, connections)
- Response times
- Docker image sizes
- Log file sizes

**Common Issues**:
```
⚠ High memory usage: 7.5 GB / 8 GB (94%)
  Fix: Increase system memory or stop unused containers

⚠ Slow database queries detected (avg 250ms)
  Fix: Add indexes, optimize queries

⚠ Large Docker images (3.5 GB total)
  Fix: Run 'docker system prune'
```

### Security

Audits security configuration.

**Validates**:
- File permissions (.env, SSL keys)
- Secret strength and rotation
- Exposed secrets in git
- SSL/TLS configuration
- Network exposure
- Default credentials

**Common Issues**:
```
✗ .env file readable by all users (644)
  Fix: chmod 600 .env

✗ Using default password: POSTGRES_PASSWORD=postgres
  Fix: Generate strong password

✗ Secrets found in git history
  Fix: Rotate secrets and use git-filter-repo
```

### Database

Checks PostgreSQL health and configuration.

**Validates**:
- Database connectivity
- Table existence and structure
- Migration status
- Connection pool size
- Query performance
- Disk usage

**Common Issues**:
```
✗ Migrations not applied: 3 pending
  Fix: nself db migrate up

⚠ High connection count: 85/100
  Fix: Increase max_connections or close idle connections

⚠ Database size large: 45 GB
  Fix: Archive old data or clean up logs
```

## Auto-Fix Capabilities

### What Can Be Auto-Fixed

✅ **File Permissions**: Automatically set correct permissions
✅ **Port Conflicts**: Suggest alternative ports
✅ **Missing Services**: Start stopped services
✅ **Weak Secrets**: Generate strong random secrets
✅ **Configuration Errors**: Fix format errors
✅ **Dependency Issues**: Install missing dependencies

### What Requires Manual Fix

❌ **Disk Space**: User must free space
❌ **Memory Issues**: User must add RAM or stop processes
❌ **Git Secret Exposure**: User must rotate secrets
❌ **Database Corruption**: User must restore from backup
❌ **Network Issues**: User must fix network configuration

## Integration

### Pre-Start Check

```bash
#!/bin/bash
# start-wrapper.sh

# Check health before starting
if ! nself doctor --fix; then
  echo "Critical issues found, aborting"
  exit 1
fi

nself start
```

### CI/CD Pipeline

```yaml
# .github/workflows/deploy.yml
- name: Run diagnostics
  run: nself doctor --category security --category config
```

### Scheduled Audits

```bash
# Cron: Run weekly security audit
0 0 * * 0 nself doctor --category security --export /var/log/security-audit-$(date +%Y%m%d).txt
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No issues found |
| 1 | Warnings only |
| 2 | Critical issues found |
| 3 | Auto-fix failed |

## Troubleshooting

### Doctor Command Fails

```bash
# Run with verbose output
nself doctor --verbose

# Check specific category
nself doctor --category environment

# Skip automatic fixes
nself doctor --skip-fix
```

### False Positives

```bash
# Ignore specific checks
SKIP_PORT_CHECK=true nself doctor

# Or create .nself-doctor-ignore
echo "port_conflict" > .nself-doctor-ignore
```

## Related Commands

- `nself health` - Service health checks
- `nself status` - Service status
- `nself logs` - View error logs
- `nself config validate` - Configuration validation only

## See Also

- [nself health](health.md)
- [nself status](status.md)
- [Troubleshooting Guide](../../guides/TROUBLESHOOTING.md)
- [Security Best Practices](../../guides/SECURITY.md)
