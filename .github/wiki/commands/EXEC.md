# nself exec - Execute Commands

**Version 0.9.9** | Run commands inside service containers

---

## Overview

The `nself exec` command executes commands inside running service containers. It provides an easy way to access shells, run database queries, debug services, and perform maintenance tasks.

---

## Basic Usage

```bash
# Open shell in a service
nself exec postgres
nself exec hasura

# Run specific command
nself exec postgres psql -U postgres
nself exec redis redis-cli

# Run with options
nself exec -it postgres bash
```

---

## Common Operations

### Database Access

```bash
# PostgreSQL shell
nself exec postgres psql -U postgres -d myapp

# Run SQL query
nself exec postgres psql -U postgres -c "SELECT * FROM users LIMIT 5"
```

### Redis Access

```bash
# Redis CLI
nself exec redis redis-cli

# Run Redis command
nself exec redis redis-cli KEYS "*"
```

### Debug Services

```bash
# Check Hasura logs
nself exec hasura cat /var/log/hasura.log

# Test network connectivity
nself exec hasura curl -s http://postgres:5432
```

---

## Options Reference

| Option | Short | Description |
|--------|-------|-------------|
| `--interactive` | `-i` | Keep STDIN open |
| `--tty` | `-t` | Allocate a TTY |
| `--user` | `-u` | Run as specific user |
| `--workdir` | `-w` | Working directory |
| `--env` | `-e` | Set environment variable |

---

## Examples

### Interactive Shell

```bash
# Bash shell in postgres container
nself exec -it postgres bash

# Sh shell (for Alpine containers)
nself exec -it hasura sh
```

### Run as Root

```bash
nself exec -u root postgres apt-get update
```

### With Environment Variables

```bash
nself exec -e DEBUG=true myservice ./debug.sh
```

---

## Service Shortcuts

| Shortcut | Container | Shell |
|----------|-----------|-------|
| `postgres` | PostgreSQL | psql |
| `redis` | Redis | redis-cli |
| `hasura` | Hasura | sh |
| `auth` | Auth Service | sh |

---

## See Also

- [logs](LOGS.md) - View service logs
- [status](STATUS.md) - Check service status
- [db](DB.md) - Database operations
