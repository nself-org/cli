# nself health - Health Check Management

**Version 0.4.6** | Health monitoring and validation

---

## Overview

The `nself health` command provides comprehensive health check management for your nself infrastructure. Monitor service health, check endpoints, and track health history.

---

## Usage

```bash
nself health [subcommand] [options]
```

---

## Subcommands

### `check` (default)

Run all health checks.

```bash
nself health                    # Check all services
nself health check              # Same as above
```

### `service <name>`

Check specific service health.

```bash
nself health service postgres   # Check PostgreSQL
nself health service hasura     # Check Hasura
nself health service auth       # Check auth service
```

### `endpoint <url>`

Check custom endpoint health.

```bash
nself health endpoint https://api.example.com/health
```

### `watch`

Continuous health monitoring.

```bash
nself health watch              # Monitor every 10s
nself health watch --interval 5 # Monitor every 5s
```

**Exit:** Press Ctrl+C to stop.

### `history`

Show health check history.

```bash
nself health history            # Recent history
nself health history --limit 50 # Last 50 checks
```

### `config`

Show health check configuration.

```bash
nself health config             # Current settings
```

---

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `--timeout N` | Health check timeout (seconds) | 30 |
| `--interval N` | Check interval for watch mode | 10 |
| `--retries N` | Number of retries on failure | 3 |
| `--env NAME` | Check health in specific environment | current |
| `--json` | Output in JSON format | false |
| `--quiet` | Only output on failure | false |
| `-h, --help` | Show help message | - |

---

## Health States

| State | Icon | Description |
|-------|------|-------------|
| `healthy` | ✓ | Service is healthy |
| `running` | ● | Running without health check defined |
| `starting` | ○ | Health check pending |
| `unhealthy` | ✗ | Health check failed |
| `stopped` | - | Container not running |
| `not_found` | - | Container doesn't exist |

---

## Examples

```bash
# Quick health check
nself health

# Check specific service
nself health service postgres

# Continuous monitoring
nself health watch --interval 5

# Check staging environment
nself health --env staging

# JSON output for tooling
nself health --json

# Quiet mode (only show failures)
nself health --quiet
```

---

## Output Example

### Table Format

```
  ➞ Service Health

  Service              Status       Time     Details
  -------              ------       ----     -------
  postgres             ✓ healthy    12ms     Container healthy
  hasura               ✓ healthy    8ms      Container healthy
  auth                 ✓ healthy    15ms     Container healthy
  nginx                ● running    3ms      No health check defined

  ➞ Endpoint Health

  Endpoint             Status       Time     HTTP
  --------             ------       ----     ----
  api                  healthy      45ms     HTTP 200
  auth                 healthy      32ms     HTTP 200

  ➞ Summary
  Healthy: 4/4

  ✓ All services healthy
```

### JSON Format

```json
{
  "timestamp": "2026-01-23T10:30:00Z",
  "healthy": 4,
  "unhealthy": 0,
  "total": 4,
  "services": [
    {"service": "postgres", "status": "healthy", "response_time_ms": 12}
  ]
}
```

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `HEALTH_TIMEOUT` | Default timeout | 30 |
| `HEALTH_RETRIES` | Default retries | 3 |
| `HEALTH_INTERVAL` | Default interval | 10 |

---

## Related Commands

- [status](STATUS.md) - Service status
- [doctor](DOCTOR.md) - System diagnostics
- [logs](LOGS.md) - Service logs

---

*Last Updated: January 24, 2026 | Version: 0.4.8*
