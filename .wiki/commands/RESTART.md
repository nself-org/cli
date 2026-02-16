# nself restart - Restart Services

**Version 0.9.9** | Stop and start services

---

## Overview

The `nself restart` command stops and then starts nself services. It's useful for applying configuration changes, recovering from errors, or refreshing service state.

---

## Basic Usage

```bash
# Restart all services
nself restart

# Restart specific service
nself restart postgres
nself restart hasura

# Restart multiple services
nself restart postgres hasura auth
```

---

## Options Reference

| Option | Description |
|--------|-------------|
| `--timeout` | Seconds to wait for graceful stop |
| `--force` | Force restart (kill if needed) |
| `--no-deps` | Don't restart dependencies |

---

## Restart Behavior

1. Gracefully stops containers (SIGTERM)
2. Waits for containers to stop (default 10s)
3. Starts containers in dependency order
4. Waits for health checks to pass

---

## Common Use Cases

### Apply Configuration Changes

```bash
# After editing .env
nself restart
```

### Recover from Errors

```bash
# Force restart hung service
nself restart --force postgres
```

### Restart Service Chain

```bash
# Restart Hasura (also restarts dependents)
nself restart hasura
```

---

## See Also

- [start](START.md) - Start services
- [stop](STOP.md) - Stop services
- [status](STATUS.md) - Check service status
