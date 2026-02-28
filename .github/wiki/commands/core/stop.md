# nself stop

**Category**: Core Commands

Stop all running nself services gracefully.

## Overview

The `stop` command gracefully shuts down all Docker containers for your nself project, preserving data and state for the next start.

**Features**:
- ✅ Graceful shutdown (respects shutdown signals)
- ✅ Data preservation (volumes maintained)
- ✅ Monorepo support (stops backend + frontends)
- ✅ Selective stopping (specific services)
- ✅ Force stop option (kill immediately)

## Usage

```bash
nself stop [OPTIONS] [SERVICE...]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-f, --force` | Force stop (SIGKILL) | false |
| `-t, --timeout N` | Shutdown timeout (seconds) | 10 |
| `-v, --volumes` | Remove volumes (⚠️ data loss) | false |
| `--clean` | Remove containers after stop | false |
| `-h, --help` | Show help message | - |

## Arguments

| Argument | Description |
|----------|-------------|
| `SERVICE` | Specific service(s) to stop (optional) |

## Examples

### Stop All Services

```bash
nself stop
```

**Output**:
```
→ Stopping nself services...

  ⠋ Stopping postgres...     ✓
  ⠋ Stopping hasura...       ✓
  ⠋ Stopping auth...         ✓
  ⠋ Stopping nginx...        ✓
  ⠋ Stopping redis...        ✓
  ⠋ Stopping minio...        ✓

✓ All services stopped (3.2s)
```

### Stop Specific Service

```bash
nself stop redis
```

**Output**:
```
→ Stopping redis...
✓ Service stopped
```

### Stop Multiple Services

```bash
nself stop redis minio mlflow
```

**Output**:
```
→ Stopping specified services...
  ✓ redis stopped
  ✓ minio stopped
  ✓ mlflow stopped
```

### Force Stop

```bash
nself stop --force
```

**Use when**:
- Services not responding to graceful shutdown
- Emergency shutdown needed
- Troubleshooting hanging processes

**Warning**: May result in data corruption for services with in-flight transactions.

### Stop and Remove Containers

```bash
nself stop --clean
```

**What happens**:
- Stops all services gracefully
- Removes containers (not volumes)
- Next `nself start` will recreate containers

**Useful for**:
- Fresh container state
- Configuration changes that need container rebuild
- Freeing disk space

### Stop and Remove Volumes (⚠️ DANGER)

```bash
nself stop --volumes
```

**WARNING**: Permanently deletes all data!

**Confirms before executing**:
```
⚠️  WARNING: This will DELETE ALL DATA including:
  - PostgreSQL databases
  - MinIO storage files
  - Redis cache
  - Monitoring data

Are you sure? [y/N]:
```

**Only use when**:
- Completely removing project
- Resetting to clean slate (dev only)
- Data is backed up elsewhere

## Monorepo Support

When running in a monorepo structure (with `/backend` directory), `nself stop` automatically handles both backend services and frontend applications.

### From Project Root

```bash
# Monorepo structure detected
nself stop
```

**Output**:
```
→ Monorepo Mode Detected

→ Stopping frontend applications...
  ✓ app1 stopped (PID: 12346)
  ✓ app2 stopped (PID: 12347)

→ Stopping backend services...
  ✓ Backend services stopped

✓ All services stopped
```

### From Backend Directory

```bash
cd backend
nself stop
# Only stops backend Docker services
```

## Shutdown Process

### Graceful Shutdown (Default)

```
1. Send SIGTERM to containers
   ↓
2. Wait for graceful shutdown (10s default)
   ↓
3. If still running, send SIGKILL
   ↓
4. Remove containers (if --clean)
   ↓
5. Report status
```

### Force Shutdown (--force)

```
1. Send SIGKILL immediately
   ↓
2. Wait for container removal
   ↓
3. Report status
```

## What Happens to Data?

### Data Preserved (Default)

When you run `nself stop`:
- ✅ PostgreSQL data (in Docker volume)
- ✅ MinIO storage (in Docker volume)
- ✅ Redis persistence (if enabled)
- ✅ Hasura metadata
- ✅ Monitoring data

**Next `nself start` resumes from this state.**

### Data Removed (--volumes)

When you run `nself stop --volumes`:
- ❌ PostgreSQL data deleted
- ❌ MinIO storage deleted
- ❌ Redis data deleted
- ❌ All monitoring data deleted

**Next `nself start` starts with empty databases.**

## Service-Specific Behavior

### PostgreSQL

```bash
nself stop postgres
```

**Graceful shutdown**:
- Waits for active connections to close
- Flushes write-ahead log
- Saves checkpoint

**Force shutdown**:
- Immediate termination
- May need recovery on next start

### Hasura

```bash
nself stop hasura
```

**Graceful shutdown**:
- Completes in-flight GraphQL queries
- Closes connections

### MinIO

```bash
nself stop minio
```

**Graceful shutdown**:
- Completes in-flight uploads
- Flushes object metadata

### Monitoring Services

```bash
nself stop prometheus grafana loki
```

**Graceful shutdown**:
- Prometheus: Saves TSDB
- Grafana: Saves session state
- Loki: Flushes log buffer

## Timeout Configuration

### Default Timeout (10 seconds)

```bash
nself stop
# Waits 10s for graceful shutdown, then kills
```

### Custom Timeout

```bash
# Wait 30 seconds before force kill
nself stop --timeout 30
```

**Increase timeout when**:
- Database has long transactions
- MinIO has large uploads in progress
- Services need extra cleanup time

### No Timeout (Wait Forever)

```bash
# Wait indefinitely for graceful shutdown
nself stop --timeout 0
```

**Use with caution**: May hang if service not responding.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All services stopped successfully |
| 1 | Some services failed to stop |
| 2 | Configuration error |
| 3 | No services running |
| 130 | Interrupted (Ctrl+C) |

## Troubleshooting

### Service won't stop

**Error**:
```
✗ Service 'postgres' failed to stop after 10s
```

**Solutions**:
```bash
# Increase timeout
nself stop --timeout 30

# Force stop
nself stop --force

# Check for hung processes
docker ps
docker logs <container-id>
```

### "No such container" error

**Error**:
```
✗ Error: No such container: myapp_postgres
```

**Cause**: Services not running or already stopped.

**Solution**:
```bash
# Check status first
nself status

# If services running but wrong name
nself build && nself start
```

### Volume removal fails

**Error**:
```
✗ Error removing volume: volume is in use
```

**Solution**:
```bash
# Stop all services first
nself stop

# Remove volumes
docker volume rm $(docker volume ls -q | grep myapp)

# Or use Docker prune
docker volume prune
```

### Monorepo frontend processes orphaned

**Symptom**: Frontend apps still running after `nself stop`

**Solution**:
```bash
# Find process IDs
ps aux | grep -E "(pnpm|npm|yarn) dev"

# Kill manually
kill <PID>

# Or use nself-specific cleanup
pkill -f ".nself/frontend"
```

## Best Practices

### 1. Always Stop Gracefully

```bash
# Good
nself stop

# Avoid (unless necessary)
nself stop --force
```

### 2. Before Configuration Changes

```bash
# Stop services before major config changes
nself stop
vi .env
nself build
nself start
```

### 3. Development Iteration

```bash
# Quick restart during development
nself restart
# (equivalent to stop + start, but faster)
```

### 4. Before System Shutdown

```bash
# Stop services before shutting down computer
nself stop
# Ensures clean shutdown, no orphaned processes
```

## Related Commands

- `nself start` - Start services
- `nself restart` - Restart services (stop + start)
- `nself status` - Check if services are running
- `nself logs` - View service logs before stopping

## See Also

- [nself start](start.md)
- [nself restart](restart.md)
- [nself status](../utilities/status.md)
- [Service Management Guide](../../guides/SERVICE-MANAGEMENT.md)
