# nself start - Start Services

**Version 0.9.9** | Start all nself services with smart health checking

---

## Overview

The `nself start` command starts all Docker containers defined in your project. It includes intelligent health checking, automatic recovery, and configurable startup behavior.

---

## Table of Contents

- [Basic Usage](#basic-usage)
- [Start Modes](#start-modes)
- [Options Reference](#options-reference)
- [Health Checking](#health-checking)
- [Configuration](#configuration)
- [Service Order](#service-order)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Basic Usage

```bash
# Start all services
nself start

# Start with verbose output
nself start --verbose

# Start specific service
nself start postgres

# Start in background (detached)
nself start --detach
```

---

## Start Modes

### Smart Mode (Default)

Intelligently handles existing containers:

```bash
nself start
# or
NSELF_START_MODE=smart nself start
```

**Behavior:**
- Resumes stopped containers
- Keeps running healthy containers
- Only recreates problematic containers
- Fastest startup for ongoing development
- Automatically removes exited init containers (e.g., `minio_init`) after startup

### Fresh Mode

Force recreates all containers:

```bash
nself start --fresh
# or
NSELF_START_MODE=fresh nself start
```

**Behavior:**
- Stops all existing containers
- Removes and recreates all containers
- Use after configuration changes
- Slower but guarantees clean state

### Force Mode

Most aggressive cleanup:

```bash
nself start --clean-start
# or
NSELF_START_MODE=force nself start
```

**Behavior:**
- Removes all containers first
- Cleans up orphan networks
- Fresh start from scratch
- Use when troubleshooting issues

---

## Options Reference

| Option | Short | Description |
|--------|-------|-------------|
| `--verbose` | `-v` | Show detailed output |
| `--debug` | | Maximum verbosity |
| `--detach` | `-d` | Run in background |
| `--fresh` | | Force recreate all containers |
| `--clean-start` | | Remove everything first |
| `--quick` | `-q` | Minimal checks, fast start |
| `--skip-health-checks` | | Skip health validation |
| `--timeout <seconds>` | `-t` | Health check timeout |
| `--health-required <percent>` | | Minimum healthy percentage |
| `--service <name>` | `-s` | Start specific service only |

### Health Check Options

```bash
# Longer timeout for slow systems
nself start --timeout 180

# Lower health requirement for development
nself start --health-required 60

# Skip health checks entirely
nself start --skip-health-checks
```

---

## Health Checking

### Progressive Health Checking

The start command uses progressive health checking:

```
Starting services...
  postgres    [████████████████████] ✓ healthy
  hasura      [█████████████░░░░░░░]   starting
  auth        [████████░░░░░░░░░░░░]   starting
  nginx       [██░░░░░░░░░░░░░░░░░░]   pending
```

### Health Check Configuration

Configure via environment variables:

```bash
# Timeout for health checks (seconds)
NSELF_HEALTH_CHECK_TIMEOUT=120    # Default: 120

# Check interval (seconds)
NSELF_HEALTH_CHECK_INTERVAL=2     # Default: 2

# Minimum healthy percentage
NSELF_HEALTH_CHECK_REQUIRED=80    # Default: 80

# Skip health checks
NSELF_SKIP_HEALTH_CHECKS=false    # Default: false
```

### Health Check Behavior

- **Accepts partial success** - Default 80% healthy is OK
- **Doesn't fail on timeout** - Continues if services are running
- **Real-time progress** - Shows status during startup

---

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NSELF_START_MODE` | Start mode (smart/fresh/force) | `smart` |
| `NSELF_HEALTH_CHECK_TIMEOUT` | Health check timeout (seconds) | `120` |
| `NSELF_HEALTH_CHECK_INTERVAL` | Check interval (seconds) | `2` |
| `NSELF_HEALTH_CHECK_REQUIRED` | Minimum healthy percentage | `80` |
| `NSELF_SKIP_HEALTH_CHECKS` | Skip health validation | `false` |
| `NSELF_DOCKER_BUILD_TIMEOUT` | Docker build timeout | `300` |
| `NSELF_CLEANUP_ON_START` | Cleanup strategy (auto/always/never) | `auto` |
| `NSELF_PARALLEL_LIMIT` | Parallel container starts | `5` |
| `NSELF_LOG_LEVEL` | Verbosity (debug/info/warn/error) | `info` |

### Configuration Presets

**Development (Quick Iteration):**
```bash
NSELF_HEALTH_CHECK_REQUIRED=60
NSELF_HEALTH_CHECK_TIMEOUT=60
```

**Production (Full Validation):**
```bash
NSELF_HEALTH_CHECK_REQUIRED=100
NSELF_HEALTH_CHECK_TIMEOUT=180
```

**CI/CD (Clean State):**
```bash
NSELF_START_MODE=fresh
NSELF_CLEANUP_ON_START=always
```

**Debugging:**
```bash
NSELF_LOG_LEVEL=debug
NSELF_SKIP_HEALTH_CHECKS=true
```

---

## Service Order

Services start in dependency order:

### Phase 1: Infrastructure
1. **postgres** - Database (no dependencies)

### Phase 2: Core Services
2. **hasura** - GraphQL (depends on postgres)
3. **auth** - Authentication (depends on postgres, hasura)
4. **redis** - Cache (if enabled)

### Phase 3: Optional Services
5. **minio** - Storage
6. **meilisearch** - Search
7. **mailpit** - Email
8. **functions** - Serverless
9. **mlflow** - ML tracking

### Phase 4: Custom Services
10. **CS_1 through CS_10** - Your services

### Phase 5: Monitoring
11. **prometheus** - Metrics
12. **grafana** - Dashboards
13. **loki** - Logs
14. **promtail** - Log shipping
15. All other monitoring services

### Phase 6: Routing
16. **nginx** - Reverse proxy (starts last)

---

## Examples

### Standard Development Start

```bash
nself start

# Output:
# Starting nself services...
#
# ✓ postgres      healthy (2.3s)
# ✓ hasura        healthy (5.1s)
# ✓ auth          healthy (3.2s)
# ✓ nginx         healthy (1.1s)
#
# All 4 services healthy
# Service URLs: nself urls
```

### Start with Full Monitoring

```bash
# Enable monitoring in .env first
echo "MONITORING_ENABLED=true" >> .env
nself build

nself start

# Output:
# Starting nself services (14 containers)...
#
# ✓ postgres           healthy
# ✓ hasura             healthy
# ✓ auth               healthy
# ✓ prometheus         healthy
# ✓ grafana            healthy
# ✓ loki               healthy
# ✓ promtail           healthy
# ✓ alertmanager       healthy
# ✓ cadvisor           healthy
# ✓ node-exporter      healthy
# ✓ postgres-exporter  healthy
# ✓ redis-exporter     healthy
# ✓ nginx              healthy
#
# All 14 services healthy
```

### Quick Development Restart

```bash
# Fast restart after code changes
nself start --quick

# Skips most health checks, fastest startup
```

### Debug Startup Issues

```bash
# Maximum verbosity
nself start --debug

# Or with environment variable
NSELF_LOG_LEVEL=debug nself start
```

### Start Single Service

```bash
# Restart just nginx after config change
nself start --service nginx

# Or with docker directly
docker compose restart nginx
```

### Fresh Start After Config Change

```bash
# Edit .env or nginx configs
vim .env

# Rebuild and fresh start
nself build
nself start --fresh
```

---

## Troubleshooting

### Services Timing Out But Running

```bash
# Lower health requirement
nself start --health-required 70

# Or increase timeout
nself start --timeout 180

# Or skip health checks
nself start --skip-health-checks
```

### Port Already In Use

`nself start` now checks for port conflicts before launching containers and reports
clearly which process holds each port and which `.env` variable to change.

Example output when Tailscale holds port 443:

```
ERROR: Port 443 is already in use by 'Tailscale' (needed by nginx HTTPS)
       To change it: set NGINX_SSL_PORT=<port> in .env
Cannot start: one or more required ports are in use.
Update the port variables shown above in your .env file, then run 'nself build && nself start'
```

To resolve:

```bash
# Option 1: Change the conflicting port in your .env
NGINX_SSL_PORT=8443

# Then rebuild (regenerates docker-compose.yml with new port)
nself build && nself start

# Option 2: Stop the conflicting process
# macOS example - stop Tailscale:
sudo launchctl stop com.tailscale.ipn.macsys.network-extension

# Option 3: Manually identify the process
lsof -i :443 -sTCP:LISTEN
```

Ports checked at startup: `NGINX_PORT` (80), `NGINX_SSL_PORT` (443),
`POSTGRES_PORT` (5432), `REDIS_PORT` (6379), `HASURA_PORT` (8080),
`MINIO_PORT` (9000), `MINIO_CONSOLE_PORT` (9001), `MAILPIT_SMTP_PORT` (1025),
`MAILPIT_UI_PORT` (8025).

### Container Won't Start

```bash
# Check container logs
docker logs myapp_hasura

# Check docker compose config
docker compose config

# Try fresh start
nself start --fresh
```

### Out of Memory

```bash
# Check Docker memory allocation
docker system info | grep Memory

# Reduce parallel starts
NSELF_PARALLEL_LIMIT=2 nself start
```

### Slow Startup

```bash
# Skip optional services for faster start
REDIS_ENABLED=false
MINIO_ENABLED=false
MONITORING_ENABLED=false

# Rebuild and start
nself build && nself start
```

### Database Won't Connect

```bash
# Check postgres logs
docker logs myapp_postgres

# Verify postgres is healthy
docker exec myapp_postgres pg_isready

# Check connection string
docker exec myapp_hasura env | grep DATABASE_URL
```

---

## Service Status Indicators

| Symbol | Status | Description |
|--------|--------|-------------|
| ✓ | healthy | Service is running and healthy |
| ⟳ | starting | Service is starting up |
| ○ | stopped | Service is not running |
| ✗ | unhealthy | Service is running but unhealthy |
| ! | error | Service failed to start |

---

## Post-Start Commands

After successful start:

```bash
# Check status
nself status

# View URLs
nself urls

# View logs
nself logs              # All services
nself logs hasura       # Specific service
nself logs -f           # Follow logs

# Open services
open https://api.local.nself.org/console   # Hasura console
open https://admin.local.nself.org         # Admin dashboard
open https://grafana.local.nself.org       # Grafana
```

---

## Related Commands

- [init](INIT.md) - Initialize project
- [build](BUILD.md) - Generate configuration
- [stop](STOP.md) - Stop services
- [status](STATUS.md) - Check service status
- [urls](URLS.md) - View service URLs
- [logs](LOGS.md) - View service logs

---

*Last Updated: January 2026 | Version 0.9.9*
