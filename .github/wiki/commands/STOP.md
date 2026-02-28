# nself stop - Stop Services

**Version 0.9.9** | Stop and manage running nself services

---

## Overview

The `nself stop` command stops running Docker containers. It provides options for graceful shutdown, removing containers, and cleaning up resources.

---

## Table of Contents

- [Basic Usage](#basic-usage)
- [Stop Options](#stop-options)
- [Options Reference](#options-reference)
- [Shutdown Behavior](#shutdown-behavior)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Basic Usage

```bash
# Stop all services (graceful)
nself stop

# Stop specific service
nself stop postgres

# Stop and remove containers
nself stop --remove

# Force stop (immediate)
nself stop --force
```

---

## Stop Options

### Graceful Stop (Default)

Sends SIGTERM and waits for graceful shutdown:

```bash
nself stop
```

**Behavior:**
- Sends SIGTERM to all containers
- Waits for graceful shutdown (default 30 seconds)
- Preserves data volumes
- Containers can be restarted quickly

### Remove Containers

Stop and remove containers:

```bash
nself stop --remove
```

**Behavior:**
- Stops all containers
- Removes containers (not volumes)
- Next start will recreate containers
- Data is preserved in volumes

### Force Stop

Immediate stop without waiting:

```bash
nself stop --force
```

**Behavior:**
- Sends SIGKILL immediately
- No graceful shutdown
- Use when services are hung
- May cause data loss in rare cases

### Full Cleanup

Remove everything including volumes:

```bash
nself stop --clean
```

**Behavior:**
- Stops all containers
- Removes all containers
- Removes all volumes (DATA LOSS!)
- Removes networks
- Complete reset

---

## Options Reference

| Option | Short | Description |
|--------|-------|-------------|
| `--remove` | `-r` | Remove containers after stop |
| `--force` | `-f` | Force stop (SIGKILL) |
| `--clean` | | Full cleanup including volumes |
| `--timeout <seconds>` | `-t` | Shutdown timeout (default: 30) |
| `--service <name>` | `-s` | Stop specific service only |
| `--no-confirm` | `-y` | Skip confirmation for destructive operations |
| `--quiet` | `-q` | Minimal output |

### Service Selection

```bash
# Stop single service
nself stop --service postgres

# Stop multiple services
nself stop --service postgres --service redis

# Or use docker directly
docker compose stop postgres redis
```

---

## Shutdown Behavior

### Graceful Shutdown Order

Services stop in reverse dependency order:

1. **nginx** - Stop accepting requests
2. **Custom services** - Finish processing
3. **Monitoring services** - Collect final metrics
4. **Optional services** - Redis, MinIO, etc.
5. **auth** - End sessions
6. **hasura** - Close connections
7. **postgres** - Checkpoint and stop

### Timeout Configuration

```bash
# Longer timeout for services with long-running tasks
nself stop --timeout 60

# Quick stop for development
nself stop --timeout 10
```

### Signal Flow

```
SIGTERM → Wait → SIGKILL (if timeout)
   │
   ├── Service receives SIGTERM
   ├── Service begins graceful shutdown
   ├── Service finishes current requests
   ├── Service closes connections
   └── Service exits
```

---

## Examples

### Stop for Development Break

```bash
# Graceful stop, preserve state
nself stop

# Output:
# Stopping nself services...
# ✓ nginx stopped
# ✓ auth stopped
# ✓ hasura stopped
# ✓ postgres stopped
#
# All services stopped
# Restart with: nself start
```

### Stop Specific Service

```bash
# Stop just postgres for maintenance
nself stop --service postgres

# Or using docker
docker compose stop postgres
```

### Clean Restart

```bash
# Stop and remove containers (keeps data)
nself stop --remove

# Rebuild and start fresh
nself build
nself start
```

### Factory Reset

```bash
# WARNING: Deletes all data!
nself stop --clean

# Output:
# ⚠️  This will delete all data including databases!
# Type 'yes' to confirm: yes
#
# Stopping services...
# Removing containers...
# Removing volumes...
# Removing networks...
#
# ✓ Full cleanup complete
# Reinitialize with: nself init
```

### Force Stop Hung Services

```bash
# When services won't stop gracefully
nself stop --force

# Or for single service
docker kill myapp_hasura
```

### Quick Stop for CI/CD

```bash
# Skip confirmations
nself stop --remove --no-confirm

# Or with cleanup
nself stop --clean --no-confirm
```

---

## Troubleshooting

### Service Won't Stop

```bash
# Check what's keeping it running
docker logs myapp_hasura --tail 100

# Force stop
nself stop --force

# Or kill directly
docker kill myapp_hasura
```

### "Device or resource busy"

```bash
# Check what's using the volume
lsof +D /var/lib/docker/volumes/myapp_postgres_data

# Stop the process or force remove
docker volume rm -f myapp_postgres_data
```

### Orphan Containers

```bash
# List orphan containers
docker compose ps --orphans

# Remove orphans
docker compose down --remove-orphans
```

### Volumes Not Removed

```bash
# List project volumes
docker volume ls | grep myapp

# Remove specific volume
docker volume rm myapp_postgres_data

# Remove all project volumes
docker volume rm $(docker volume ls -q | grep myapp)
```

### Network Issues After Stop

```bash
# List networks
docker network ls

# Remove project network
docker network rm myapp_default

# Or prune unused networks
docker network prune
```

---

## Data Safety

### What Is Preserved

| Stop Type | Containers | Volumes | Networks |
|-----------|------------|---------|----------|
| `nself stop` | Preserved | Preserved | Preserved |
| `nself stop --remove` | Removed | Preserved | Preserved |
| `nself stop --force` | Preserved | Preserved | Preserved |
| `nself stop --clean` | Removed | **REMOVED** | Removed |

### Backup Before Cleanup

Always backup before using `--clean`:

```bash
# Backup database
nself db backup

# Then cleanup
nself stop --clean
```

---

## Post-Stop Commands

### Check Status

```bash
nself status
# Shows all services as stopped
```

### View Remaining Resources

```bash
# Containers (stopped)
docker compose ps -a

# Volumes
docker volume ls | grep myapp

# Networks
docker network ls | grep myapp
```

### Restart Services

```bash
# Quick restart (if containers preserved)
nself start

# Full restart (if containers removed)
nself build && nself start
```

---

## Related Commands

- [start](START.md) - Start services
- [status](STATUS.md) - Check service status
- [logs](LOGS.md) - View service logs
- [db backup](DB.md#backup) - Backup database

---

*Last Updated: January 2026 | Version 0.9.9*
