# nself restart

**Category**: Core Commands

Restart nself services with optimized stop-then-start sequence.

## Overview

The `restart` command intelligently restarts your nself services, combining stop and start into a single optimized operation.

**Features**:
- ✅ Smart restart (only affected services)
- ✅ Full restart (all services)
- ✅ Hot reload (configuration changes without rebuild)
- ✅ Health checking (verifies services after restart)
- ✅ Monorepo support (backend + frontends)

## Usage

```bash
nself restart [OPTIONS] [SERVICE...]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-f, --force` | Force recreate containers | false |
| `--rebuild` | Rebuild images before restart | false |
| `--quick` | Skip health checks | false |
| `-t, --timeout N` | Restart timeout (seconds) | 30 |
| `-v, --verbose` | Show detailed output | false |
| `-h, --help` | Show help message | - |

## Arguments

| Argument | Description |
|----------|-------------|
| `SERVICE` | Specific service(s) to restart (optional) |

## Restart Modes

### Smart Restart (Default)

```bash
nself restart
```

**Behavior**:
- Restarts only modified services
- Keeps healthy services running
- Applies configuration changes
- Fastest restart method

**Output**:
```
→ Detecting changed services...
  Modified: redis, nginx
  Unchanged: postgres, hasura, auth

→ Restarting modified services...
  ⠋ Restarting redis...    ✓
  ⠋ Restarting nginx...    ✓

✓ Services restarted (4.2s)
```

### Full Restart

```bash
nself restart --force
```

**Behavior**:
- Stops all services
- Recreates all containers
- Applies all configuration
- Slower but ensures clean state

**Output**:
```
→ Full restart requested

→ Stopping all services...
  ✓ All services stopped

→ Recreating containers...
  ✓ Containers recreated

→ Starting services...
  ✓ All services started (12.5s)
```

### Restart with Rebuild

```bash
nself restart --rebuild
```

**Use when**:
- Custom service code changed
- Dockerfile modified
- Dependencies updated

**Behavior**:
1. Stops services
2. Rebuilds Docker images
3. Recreates containers
4. Starts services

**Output**:
```
→ Rebuild requested

→ Stopping services...
  ✓ Stopped

→ Building images...
  ⠋ Building api...        ✓ (15.3s)
  ⠋ Building worker...     ✓ (8.7s)

→ Starting services...
  ✓ All services started
```

## Examples

### Basic Restart

```bash
nself restart
```

**Common use case**: After changing `.env` configuration.

### Restart Specific Service

```bash
nself restart redis
```

**Output**:
```
→ Restarting redis...
✓ Service restarted
```

### Restart Multiple Services

```bash
nself restart redis minio prometheus
```

**Output**:
```
→ Restarting specified services...
  ✓ redis restarted
  ✓ minio restarted
  ✓ prometheus restarted
```

### Quick Restart (Skip Health Checks)

```bash
nself restart --quick
```

**Faster but**:
- Doesn't verify services are healthy
- Use only in development
- Not recommended for production

### Restart After Configuration Change

```bash
# Change configuration
echo "REDIS_PORT=6380" >> .env

# Rebuild configs
nself build

# Restart affected services
nself restart redis
```

## When to Use Restart vs Stop/Start

### Use `nself restart`

✅ After configuration changes in `.env`
✅ When services need quick reload
✅ During active development
✅ When you know what changed

**Advantages**:
- Faster than stop + start
- Smarter (only restarts what's needed)
- Single command

### Use `nself stop` then `nself start`

✅ Major infrastructure changes
✅ Troubleshooting complex issues
✅ Before system maintenance
✅ When you want explicit control

**Advantages**:
- More explicit
- Easier to debug
- Can inspect stopped state

## Restart Flow

### Smart Restart Flow

```
1. Detect configuration changes
   ↓
2. Identify affected services
   ↓
3. Stop affected services only
   ↓
4. Apply new configuration
   ↓
5. Start affected services
   ↓
6. Verify health (if not --quick)
   ↓
7. Report status
```

### Full Restart Flow (--force)

```
1. Stop all services
   ↓
2. Remove containers (keep volumes)
   ↓
3. Rebuild configurations
   ↓
4. Recreate all containers
   ↓
5. Start all services
   ↓
6. Verify health
   ↓
7. Report status
```

## Configuration Change Detection

The restart command intelligently detects what changed:

**Detects changes in**:
- `.env` file
- `docker-compose.yml`
- `nginx/` configuration
- Service environment variables

**Examples**:

```bash
# Changed Redis port
REDIS_PORT=6380
→ Restarts: redis, redis-exporter, services using Redis

# Changed Nginx config
nginx/sites/custom.conf modified
→ Restarts: nginx only

# Changed database password
POSTGRES_PASSWORD=newpass
→ Restarts: postgres, hasura, auth (all DB clients)
```

## Monorepo Support

### From Project Root

```bash
nself restart
```

**With monorepo**:
```
→ Monorepo Mode Detected

→ Restarting backend services...
  ✓ Backend restarted

→ Restarting frontend applications...
  ✓ app1 restarted (PID: 45678)
  ✓ app2 restarted (PID: 45679)

✓ All services restarted
```

### From Backend Directory

```bash
cd backend
nself restart
# Only restarts backend services
```

## Health Checking

After restart, nself verifies services are healthy:

```
→ Checking service health...
  ✓ postgres    [healthy]
  ✓ hasura      [healthy]
  ✓ auth        [healthy]
  ✓ nginx       [healthy]
  ✓ redis       [healthy]

✓ All services healthy
```

**Health check criteria**:
- Container is running
- Service responds to health endpoint
- No restart loops
- Passes basic connectivity test

**Skip health checks**:
```bash
nself restart --quick
# Faster, but doesn't verify
```

## Timeout Configuration

### Default Timeout (30 seconds)

```bash
nself restart
# Fails if not healthy after 30s
```

### Custom Timeout

```bash
# Wait up to 60 seconds
nself restart --timeout 60
```

**Increase timeout when**:
- Services have slow startup
- Database has large recovery
- Network is slow

## Restart Hooks

### Pre-Restart Hook

```bash
# .nself-hooks/pre-restart.sh
#!/bin/bash
echo "Backing up before restart..."
nself db backup pre-restart.sql
```

### Post-Restart Hook

```bash
# .nself-hooks/post-restart.sh
#!/bin/bash
echo "Verifying after restart..."
nself health
```

**Enable hooks**:
```bash
ENABLE_HOOKS=true nself restart
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Restart successful |
| 1 | Restart failed |
| 2 | Configuration error |
| 3 | Health check failed |
| 130 | Interrupted (Ctrl+C) |

## Troubleshooting

### Services fail to restart

**Error**:
```
✗ Service 'postgres' failed to start after restart
```

**Solutions**:
```bash
# Check logs
nself logs postgres

# Try force restart
nself restart --force

# Or explicit stop/start
nself stop
nself start
```

### Health checks failing

**Error**:
```
✗ Health check failed for: hasura
  Connection refused on port 8080
```

**Solutions**:
```bash
# Increase timeout
nself restart --timeout 60

# Check service logs
nself logs hasura

# Skip health checks temporarily
nself restart --quick
```

### Configuration not applied

**Symptom**: Changes in `.env` not reflected after restart.

**Solution**:
```bash
# Rebuild configs first
nself build

# Then restart
nself restart --force
```

### Container recreation needed

**Error**:
```
✗ Configuration requires container recreation
```

**Solution**:
```bash
# Use force restart
nself restart --force

# Or explicit sequence
nself stop --clean
nself start
```

## Best Practices

### 1. Restart After Config Changes

```bash
vi .env          # Make changes
nself build      # Regenerate configs
nself restart    # Apply changes
```

### 2. Use Specific Service Restart in Development

```bash
# Only restart what you're working on
nself restart api
# Faster than restarting everything
```

### 3. Full Restart for Major Changes

```bash
# After enabling new services
echo "MONITORING_ENABLED=true" >> .env
nself build
nself restart --force
```

### 4. Rebuild When Code Changes

```bash
# Custom service code changed
nself restart --rebuild api
```

## Related Commands

- `nself start` - Start services
- `nself stop` - Stop services
- `nself status` - Check service status
- `nself health` - Verify service health
- `nself build` - Rebuild configurations

## See Also

- [nself start](start.md)
- [nself stop](stop.md)
- [nself status](../utilities/status.md)
- [nself health](../utilities/health.md)
- [Service Management Guide](../../guides/SERVICE-MANAGEMENT.md)
