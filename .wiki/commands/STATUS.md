# nself status - Service Status

**Version 0.9.9** | Monitor and inspect nself service health

---

## Overview

The `nself status` command provides comprehensive status information about your nself services. It shows container states, health checks, resource usage, and connectivity status for all running services.

---

## Table of Contents

- [Basic Usage](#basic-usage)
- [Output Formats](#output-formats)
- [Status Indicators](#status-indicators)
- [Options Reference](#options-reference)
- [Service Details](#service-details)
- [Health Check Information](#health-check-information)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Basic Usage

```bash
# Quick status overview
nself status

# Detailed status with all info
nself status --verbose

# JSON output for scripting
nself status --json

# Watch mode (auto-refresh)
nself status --watch

# Check specific service
nself status postgres
```

---

## Output Formats

### Default Output

The default output provides a clean, color-coded overview:

```
nself Status - myapp (development)
═══════════════════════════════════════════════════════════════

Required Services
─────────────────────────────────────────────────────────────────
  ✓ postgres    running   healthy   5432:5432   PostgreSQL 16
  ✓ hasura      running   healthy   8080:8080   Hasura GraphQL Engine
  ✓ auth        running   healthy   4000:4000   nHost Auth
  ✓ nginx       running   healthy   80,443      Nginx Reverse Proxy

Optional Services
─────────────────────────────────────────────────────────────────
  ✓ redis       running   healthy   6379:6379   Redis 7
  ✓ minio       running   healthy   9000,9001   MinIO Object Storage
  ✓ mailpit     running   healthy   1025,8025   MailPit Dev Server
  ✓ meilisearch running   healthy   7700:7700   MeiliSearch

Monitoring Bundle
─────────────────────────────────────────────────────────────────
  ✓ prometheus  running   healthy   9090:9090   Prometheus
  ✓ grafana     running   healthy   3000:3000   Grafana
  ✓ loki        running   healthy   3100:3100   Loki
  ○ promtail    running   -         -           Promtail (no healthcheck)

Custom Services
─────────────────────────────────────────────────────────────────
  ✓ express-api running   healthy   8001:8001   Express.js API (CS_1)
  ✓ worker      running   healthy   -           BullMQ Worker (CS_2)

Summary: 14/14 services running, 12/14 healthy
```

### Verbose Output

Add `--verbose` for detailed information:

```bash
nself status --verbose
```

Shows additional details:
- Container IDs
- Image versions
- Uptime
- Memory/CPU usage
- Network information
- Volume mounts
- Environment highlights

### JSON Output

For scripting and automation:

```bash
nself status --json
```

```json
{
  "project": "myapp",
  "environment": "development",
  "timestamp": "2026-01-24T12:30:00Z",
  "summary": {
    "total": 14,
    "running": 14,
    "healthy": 12,
    "unhealthy": 0,
    "stopped": 0
  },
  "services": [
    {
      "name": "postgres",
      "category": "required",
      "status": "running",
      "health": "healthy",
      "container_id": "abc123def456",
      "image": "postgres:16-alpine",
      "ports": ["5432:5432"],
      "uptime": "2h 15m",
      "memory": "256MB",
      "cpu": "0.5%"
    }
  ]
}
```

### Compact Output

For minimal output (CI/CD):

```bash
nself status --quiet
```

```
running:14 healthy:12 unhealthy:0
```

---

## Status Indicators

### Container States

| Symbol | State | Description |
|--------|-------|-------------|
| `✓` | Running | Container is running |
| `✗` | Stopped | Container is stopped |
| `⟳` | Restarting | Container is restarting |
| `○` | Created | Container created but not started |
| `!` | Error | Container has errors |
| `-` | Missing | Container doesn't exist |

### Health States

| Symbol | State | Description |
|--------|-------|-------------|
| `healthy` | Healthy | Health check passing |
| `unhealthy` | Unhealthy | Health check failing |
| `starting` | Starting | Health check not yet run |
| `-` | N/A | No health check defined |

### Color Coding

- **Green** - Healthy/Running
- **Yellow** - Starting/Warning
- **Red** - Unhealthy/Error/Stopped
- **Gray** - N/A or no healthcheck

---

## Options Reference

| Option | Short | Description |
|--------|-------|-------------|
| `--verbose` | `-v` | Show detailed information |
| `--json` | `-j` | Output as JSON |
| `--quiet` | `-q` | Minimal output |
| `--watch` | `-w` | Auto-refresh every 2 seconds |
| `--interval <seconds>` | `-i` | Watch refresh interval |
| `--category <name>` | `-c` | Filter by category |
| `--health-only` | | Show only health status |
| `--ports` | `-p` | Show port mappings |
| `--resources` | `-r` | Show resource usage |
| `--no-color` | | Disable colored output |

### Category Filters

```bash
# Show only required services
nself status --category required

# Show only monitoring services
nself status --category monitoring

# Show only custom services
nself status --category custom

# Show only optional services
nself status --category optional
```

---

## Service Details

### Required Services

Always checked regardless of configuration:

```bash
# Check required services only
nself status --category required
```

| Service | Default Port | Health Check |
|---------|-------------|--------------|
| PostgreSQL | 5432 | `pg_isready` |
| Hasura | 8080 | `/healthz` endpoint |
| Auth | 4000 | `/healthz` endpoint |
| Nginx | 80, 443 | Process check |

### Optional Services

Checked when enabled:

| Service | Variable | Default Port | Health Check |
|---------|----------|-------------|--------------|
| Redis | `REDIS_ENABLED` | 6379 | `redis-cli ping` |
| MinIO | `MINIO_ENABLED` | 9000, 9001 | `/minio/health/live` |
| MailPit | `MAILPIT_ENABLED` | 1025, 8025 | HTTP check |
| MeiliSearch | `MEILISEARCH_ENABLED` | 7700 | `/health` endpoint |
| Functions | `FUNCTIONS_ENABLED` | 3000 | HTTP check |
| MLflow | `MLFLOW_ENABLED` | 5000 | HTTP check |
| nself Admin | `NSELF_ADMIN_ENABLED` | 3001 | HTTP check |

### Monitoring Services

Checked when `MONITORING_ENABLED=true`:

| Service | Default Port | Health Check |
|---------|-------------|--------------|
| Prometheus | 9090 | `/-/healthy` |
| Grafana | 3000 | `/api/health` |
| Loki | 3100 | `/ready` |
| Promtail | - | None |
| Tempo | 3200 | `/ready` |
| Alertmanager | 9093 | `/-/healthy` |
| cAdvisor | 8080 | HTTP check |
| Node Exporter | 9100 | `/metrics` |
| Postgres Exporter | 9187 | `/metrics` |
| Redis Exporter | 9121 | `/metrics` |

### Custom Services

Custom services defined via CS_N variables:

```bash
# Show custom service details
nself status --category custom --verbose
```

### Plugin Services

Services added by plugins:

```bash
# Show plugin service status
nself status --category plugins
```

---

## Health Check Information

### Understanding Health Checks

Each service can have a health check that determines if it's functioning correctly:

```bash
# Show detailed health info
nself status --health-only --verbose
```

```
Health Check Details
═══════════════════════════════════════════════════════════════

postgres
  Check: pg_isready -U postgres
  Interval: 10s
  Timeout: 5s
  Retries: 3
  Status: healthy (last check: 2s ago)

hasura
  Check: curl -f http://localhost:8080/healthz
  Interval: 30s
  Timeout: 10s
  Retries: 3
  Status: healthy (last check: 15s ago)
```

### Health Check States

**Healthy**: Service is responding correctly
```
✓ postgres   healthy   Last check: 2s ago
```

**Unhealthy**: Health check is failing
```
✗ hasura     unhealthy   Failed 3 consecutive checks
```

**Starting**: Health checks haven't passed yet
```
⟳ auth      starting   Waiting for health check (attempt 2/3)
```

### Custom Health Checks

For custom services, health checks are defined in docker-compose:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8001/health"]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 40s
```

---

## Examples

### Quick Health Check

```bash
# Fast check - are services running?
nself status --quiet

# Output: running:14 healthy:12 unhealthy:0
```

### Monitoring Dashboard

```bash
# Watch mode for live updates
nself status --watch --interval 5
```

### CI/CD Integration

```bash
#!/bin/bash
# Check if all services are healthy

status=$(nself status --json)
healthy=$(echo "$status" | jq '.summary.healthy')
total=$(echo "$status" | jq '.summary.total')

if [[ "$healthy" -eq "$total" ]]; then
  echo "All services healthy"
  exit 0
else
  echo "Some services unhealthy: $healthy/$total"
  exit 1
fi
```

### Resource Monitoring

```bash
# Show resource usage
nself status --resources

# Output:
# Service         CPU     Memory    Network I/O
# postgres        0.5%    256MB     1.2MB/500KB
# hasura          2.1%    512MB     5.6MB/2.1MB
# redis           0.1%    64MB      100KB/50KB
```

### Specific Service Check

```bash
# Check single service
nself status postgres

# Output:
# postgres
#   Status: running
#   Health: healthy
#   Container: myapp_postgres
#   Image: postgres:16-alpine
#   Ports: 5432:5432
#   Uptime: 2 hours, 15 minutes
#   Memory: 256MB / 1GB (25%)
#   CPU: 0.5%
```

### Port Information

```bash
# Show all port mappings
nself status --ports

# Output:
# Service         Internal    External    URL
# postgres        5432        5432        -
# hasura          8080        8080        api.local.nself.org
# nginx           80          80          *.local.nself.org
# nginx           443         443         *.local.nself.org (SSL)
# redis           6379        6379        -
# grafana         3000        3000        grafana.local.nself.org
```

---

## Troubleshooting

### Service Not Running

```bash
# Check why service isn't running
nself status postgres --verbose

# Check logs
nself logs postgres --tail 50

# Try to start it
docker compose up -d postgres
```

### Service Unhealthy

```bash
# Check health check details
docker inspect myapp_postgres | jq '.[0].State.Health'

# View health check logs
docker inspect myapp_postgres | jq '.[0].State.Health.Log'

# Common causes:
# - Service still starting up
# - Missing dependencies
# - Configuration errors
# - Port conflicts
```

### Missing Services

```bash
# Service not showing in status
nself status --verbose

# Check if enabled in .env
grep "REDIS_ENABLED" .env

# Rebuild if needed
nself build && nself start
```

### Status Command Slow

```bash
# Use quick mode for faster checks
nself status --quiet

# Skip resource collection
nself status --no-resources
```

### Inconsistent Status

```bash
# Force refresh Docker state
docker compose ps

# Check for orphan containers
docker compose ps --orphans

# Full cleanup and restart
nself stop --remove
nself start
```

---

## Integration with Other Commands

### After Start

```bash
# Start and check status
nself start && nself status
```

### Before Deploy

```bash
# Verify all healthy before proceeding
nself status --quiet | grep -q "unhealthy:0" && deploy.sh
```

### With Doctor

```bash
# Full health check
nself doctor && nself status --verbose
```

### With Logs

```bash
# Check status then tail logs
nself status && nself logs --follow
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All services running (or requested services running) |
| 1 | One or more services not running |
| 2 | One or more services unhealthy |
| 3 | No services found |

```bash
# Use in scripts
nself status
case $? in
  0) echo "All good" ;;
  1) echo "Some services down" ;;
  2) echo "Some services unhealthy" ;;
  3) echo "No services found" ;;
esac
```

---

## Related Commands

- [start](START.md) - Start services
- [stop](STOP.md) - Stop services
- [logs](LOGS.md) - View service logs
- [urls](URLS.md) - Show service URLs
- [doctor](DOCTOR.md) - System health check

---

*Last Updated: January 2026 | Version 0.9.9*
