# nself status

**Category**: Utilities

Check the status of all running nself services.

## Overview

Displays the current state of all Docker containers and services in your nself project.

**Features**:
- ✅ Real-time container status
- ✅ Health check results
- ✅ Resource usage (CPU, memory)
- ✅ Uptime information
- ✅ Color-coded output
- ✅ Monorepo support (backend + frontends)

## Usage

```bash
nself status [OPTIONS] [SERVICE...]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-a, --all` | Show all containers (including stopped) | false |
| `-q, --quiet` | Show only running/stopped status | false |
| `--format FORMAT` | Output format (table/json/yaml) | table |
| `--health` | Show health check details | false |
| `--resources` | Show resource usage | false |
| `-v, --verbose` | Show detailed information | false |

## Arguments

| Argument | Description |
|----------|-------------|
| `SERVICE` | Check specific service(s) only (optional) |

## Examples

### Basic Status

```bash
nself status
```

**Output**:
```
╔═══════════════════════════════════════════════════════════╗
║                  nself Service Status                     ║
╚═══════════════════════════════════════════════════════════╝

Service          Status      Health      Uptime        Port
────────────────────────────────────────────────────────────
postgres         running     healthy     2h 15m        5432
hasura           running     healthy     2h 15m        8080
auth             running     healthy     2h 15m        4000
nginx            running     healthy     2h 15m        80/443
redis            running     healthy     2h 14m        6379
minio            running     healthy     2h 14m        9000
admin            running     healthy     2h 14m        3001
────────────────────────────────────────────────────────────

Running: 7/7    Healthy: 7/7    Total: 7 services
```

### Status with Health Details

```bash
nself status --health
```

**Output**:
```
Service: postgres
  Status: running
  Health: healthy
  Checks: 3/3 passing
    ✓ Connection responsive (0.2s)
    ✓ Database accessible (0.1s)
    ✓ Replication lag < 1s (0.0s)
  Uptime: 2h 15m
  Restarts: 0

Service: hasura
  Status: running
  Health: healthy
  Checks: 2/2 passing
    ✓ GraphQL endpoint responsive (0.1s)
    ✓ Metadata sync complete (0.0s)
  Uptime: 2h 15m
  Restarts: 0
```

### Status with Resource Usage

```bash
nself status --resources
```

**Output**:
```
Service          Status      CPU      Memory       Network
──────────────────────────────────────────────────────────
postgres         running     12%      234 MB       1.2 MB/s
hasura           running     5%       156 MB       0.8 MB/s
auth             running     2%       89 MB        0.2 MB/s
nginx            running     1%       12 MB        0.5 MB/s
redis            running     1%       45 MB        0.1 MB/s
──────────────────────────────────────────────────────────

Total CPU: 21%    Total Memory: 536 MB / 8 GB (6.7%)
```

### Status of Specific Service

```bash
nself status postgres
```

**Output**:
```
Service: postgres
  Container: myapp_postgres
  Status: running
  Health: healthy
  Uptime: 2h 15m
  Restarts: 0

  Connection:
    Host: localhost
    Port: 5432
    Database: myapp_db

  Resources:
    CPU: 12%
    Memory: 234 MB / 2 GB (11.7%)
    Network: 1.2 MB/s
```

### All Containers (Including Stopped)

```bash
nself status --all
```

**Output**:
```
Service          Status      Health      Last Seen
──────────────────────────────────────────────────────
postgres         running     healthy     -
hasura           running     healthy     -
auth             running     healthy     -
nginx            running     healthy     -
redis            stopped     -           12m ago
minio            stopped     -           1h ago
──────────────────────────────────────────────────────

Running: 4/6    Stopped: 2/6
```

### Quiet Output

```bash
nself status --quiet
```

**Output**:
```
all services running
```

Or if issues:
```
3 services down: redis, minio, mlflow
```

### JSON Output

```bash
nself status --format json
```

**Output**:
```json
{
  "services": [
    {
      "name": "postgres",
      "container": "myapp_postgres",
      "status": "running",
      "health": "healthy",
      "uptime_seconds": 8100,
      "restarts": 0,
      "cpu_percent": 12.3,
      "memory_bytes": 245366784,
      "network_rx_bytes": 1258291,
      "network_tx_bytes": 892034
    },
    ...
  ],
  "summary": {
    "total": 7,
    "running": 7,
    "healthy": 7,
    "stopped": 0
  }
}
```

## Status Indicators

### Container Status

| Status | Icon | Meaning |
|--------|------|---------|
| `running` | ✓ (green) | Container is running |
| `stopped` | ✗ (red) | Container stopped |
| `paused` | ⏸ (yellow) | Container paused |
| `restarting` | ⟳ (yellow) | Container restarting |
| `dead` | ☠ (red) | Container dead/failed |

### Health Status

| Health | Icon | Meaning |
|--------|------|---------|
| `healthy` | ✓ (green) | All health checks passing |
| `unhealthy` | ✗ (red) | Health checks failing |
| `starting` | ⠋ (yellow) | Starting up, health pending |
| `none` | - (gray) | No health check defined |

## Monorepo Support

### From Project Root

```bash
nself status
```

**With monorepo**:
```
╔═══════════════════════════════════════════════════════════╗
║              Monorepo Service Status                      ║
╚═══════════════════════════════════════════════════════════╝

Backend Services (Docker)
──────────────────────────────────────────────────────────
postgres         running     healthy     2h 15m
hasura           running     healthy     2h 15m
auth             running     healthy     2h 15m
nginx            running     healthy     2h 15m

Frontend Applications
──────────────────────────────────────────────────────────
app1             running     -           2h 10m   (pnpm)
app2             running     -           2h 10m   (npm)

Summary: 6/6 running    4/4 healthy
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All services running and healthy |
| 1 | Some services stopped or unhealthy |
| 2 | Critical services down |
| 3 | No services found |

**Use in scripts**:
```bash
if ! nself status --quiet > /dev/null 2>&1; then
  echo "Services not healthy, restarting..."
  nself restart
fi
```

## Health Check Details

### PostgreSQL Health

- Connection responsive
- Database accessible
- Replication lag (if applicable)
- Disk space available

### Hasura Health

- GraphQL endpoint responding
- Metadata synchronized
- Database connectivity

### Auth Service Health

- API responding
- JWT signing functional
- Database connectivity

### Nginx Health

- Proxy responding
- SSL certificates valid
- Upstream services reachable

## Resource Monitoring

### CPU Usage

```bash
nself status --resources | grep postgres
postgres    running    12%    234 MB    1.2 MB/s
```

**High CPU?**
- Check for slow queries: `nself logs postgres | grep "duration:"`
- Review indexes: `nself db shell -c "\di"`

### Memory Usage

```bash
nself status --resources | sort -k4 -h
```

**High memory?**
- Check for memory leaks in custom services
- Review PostgreSQL shared_buffers setting
- Monitor connections: `nself db shell -c "SELECT count(*) FROM pg_stat_activity;"`

### Network Usage

**High network?**
- Check for data-intensive queries
- Review GraphQL query complexity
- Monitor logs for API abuse

## Troubleshooting

### Service shows as unhealthy

```bash
# Check health check logs
nself status --health postgres

# View container logs
nself logs postgres --tail 50

# Restart service
nself restart postgres
```

### Container constantly restarting

```bash
# Check restart count
nself status postgres

# View logs
nself logs postgres

# Common causes:
# - Port conflict
# - Missing environment variables
# - Failed health checks
# - Configuration errors
```

### Status shows no services

```bash
# Check if services are running
docker ps

# Check if in correct directory
ls docker-compose.yml

# If services should be running
nself start
```

### Different status in monorepo vs backend

```bash
# From monorepo root
cd ~/project
nself status
# Shows backend + frontends

# From backend directory
cd ~/project/backend
nself status
# Shows only backend services
```

## Automation

### Monitoring Script

```bash
#!/bin/bash
# monitor.sh - Check status every minute

while true; do
  if ! nself status --quiet; then
    echo "[$(date)] Services unhealthy!" | tee -a monitor.log
    nself status >> monitor.log

    # Alert (email, Slack, etc.)
    # curl -X POST https://hooks.slack.com/... -d "Services down"
  fi
  sleep 60
done
```

### Pre-Deployment Check

```bash
#!/bin/bash
# deploy.sh

# Verify services healthy before deploy
if ! nself status --quiet; then
  echo "ERROR: Services not healthy, aborting deployment"
  nself status
  exit 1
fi

# Deploy
git pull
nself build
nself restart
```

### Health Check Endpoint

```bash
# Create health check endpoint for load balancers
nself status --format json > /var/www/html/health.json
```

## Related Commands

- `nself start` - Start services
- `nself stop` - Stop services
- `nself restart` - Restart services
- `nself health` - Detailed health diagnostics
- `nself logs` - View service logs
- `nself monitor` - Real-time monitoring dashboard

## See Also

- [nself health](health.md)
- [nself monitor](monitor.md)
- [nself logs](logs.md)
- [Service Management Guide](../../guides/SERVICE-MANAGEMENT.md)
