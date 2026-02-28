# nself logs - Service Logs

**Version 0.9.9** | View and stream container logs

---

## Overview

The `nself logs` command provides access to container logs with clean formatting, color coding, and filtering options. It supports real-time streaming, multi-service aggregation, and various output formats.

---

## Basic Usage

```bash
# View all service logs
nself logs

# Follow logs in real-time
nself logs -f
nself logs --follow

# View specific service logs
nself logs postgres
nself logs hasura

# Last N lines
nself logs --tail 100
nself logs -n 50
```

---

## Service Selection

```bash
# Single service
nself logs postgres

# Multiple services
nself logs postgres hasura auth

# By category
nself logs --required    # Core services only
nself logs --optional    # Optional services
nself logs --monitoring  # Monitoring stack
nself logs --custom      # Custom services (CS_N)
```

---

## Filtering Options

### Time-Based

```bash
# Since specific time
nself logs --since 1h
nself logs --since 30m
nself logs --since "2024-01-20 10:00:00"

# Until specific time
nself logs --until 5m
```

### Content-Based

```bash
# Grep for pattern
nself logs | grep "error"
nself logs --grep "connection refused"

# Exclude patterns (quiet mode)
nself logs --quiet  # Hides noisy health checks
```

---

## Output Formats

### Default (Colorized)

```
postgres     2024-01-20 10:15:32  LOG:  database system is ready
hasura       2024-01-20 10:15:33  {"level":"info","message":"Server started"}
auth         2024-01-20 10:15:34  Auth service initialized
```

### Raw Output

```bash
nself logs --raw  # No formatting or colors
```

### JSON Output

```bash
nself logs --json  # Structured JSON output
```

---

## Options Reference

| Option | Short | Description |
|--------|-------|-------------|
| `--follow` | `-f` | Stream logs in real-time |
| `--tail` | `-n` | Number of lines to show |
| `--since` | | Show logs since timestamp |
| `--until` | | Show logs until timestamp |
| `--timestamps` | `-t` | Show timestamps |
| `--quiet` | `-q` | Suppress noisy output |
| `--raw` | | No formatting |
| `--json` | | JSON output |
| `--no-color` | | Disable colors |

---

## Examples

### Debug a Failing Service

```bash
# Check recent errors
nself logs postgres --tail 50 | grep -i error

# Watch in real-time
nself logs postgres -f
```

### Monitor All Services

```bash
# Follow all logs with timestamps
nself logs -f -t

# Follow specific services
nself logs -f postgres hasura auth
```

### Export Logs

```bash
# Save to file
nself logs --since 1h > logs.txt

# JSON for processing
nself logs --json > logs.json
```

### Quiet Mode

```bash
# Hide health check noise
nself logs --quiet -f
```

---

## Log Levels

Logs are color-coded by level:

| Level | Color | Example |
|-------|-------|---------|
| ERROR | Red | Connection refused |
| WARN | Yellow | Deprecated feature |
| INFO | Blue | Server started |
| DEBUG | Gray | Query executed |

---

## Troubleshooting

### No Logs Appearing

```bash
# Check if containers are running
nself status

# Check Docker logs directly
docker compose logs
```

### Logs Too Verbose

```bash
# Use quiet mode
nself logs --quiet

# Filter specific service
nself logs postgres --tail 20
```

### Missing Timestamps

```bash
# Force timestamps
nself logs -t
```

---

## See Also

- [status](STATUS.md) - Check service status
- [exec](EXEC.md) - Execute commands in containers
- [doctor](DOCTOR.md) - System diagnostics
