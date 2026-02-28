# nself logs

**Category**: Utilities

View and tail logs from nself services.

## Overview

Stream or retrieve logs from Docker containers, with filtering, formatting, and real-time tailing support.

**Features**:
- ✅ Real-time log streaming
- ✅ Historical log retrieval
- ✅ Multi-service parallel logs
- ✅ Grep filtering
- ✅ Timestamped output
- ✅ Follow mode (tail -f)
- ✅ JSON formatting

## Usage

```bash
nself logs [OPTIONS] [SERVICE...]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-f, --follow` | Follow log output (tail -f) | false |
| `-t, --tail N` | Show last N lines | 100 |
| `--since TIME` | Show logs since time | - |
| `--until TIME` | Show logs until time | - |
| `--timestamps` | Show timestamps | false |
| `--grep PATTERN` | Filter logs by pattern | - |
| `--level LEVEL` | Filter by log level | - |
| `--format FORMAT` | Output format (raw/json) | raw |
| `-a, --all` | Show logs from all services | false |

## Arguments

| Argument | Description |
|----------|-------------|
| `SERVICE` | Service(s) to show logs from (optional) |

## Examples

### View Logs from Specific Service

```bash
nself logs postgres
```

**Output**:
```
2026-02-13 14:30:15 LOG:  database system is ready to accept connections
2026-02-13 14:30:20 LOG:  autovacuum launcher started
2026-02-13 14:31:00 LOG:  checkpoint starting: time
2026-02-13 14:31:05 LOG:  checkpoint complete
```

### Follow Logs (Real-time)

```bash
nself logs -f hasura
```

**Output** (continuously updating):
```
2026-02-13 14:30:15 INFO: HTTP request received
2026-02-13 14:30:15 INFO: GraphQL query executed in 45ms
2026-02-13 14:30:16 INFO: Cache hit for query
2026-02-13 14:30:17 INFO: HTTP request received
^C
```

### Last N Lines

```bash
nself logs --tail 50 nginx
```

**Shows last 50 log lines.**

### Multiple Services

```bash
nself logs postgres hasura auth
```

**Output** (color-coded by service):
```
postgres | 2026-02-13 14:30:15 LOG:  connection received
hasura   | 2026-02-13 14:30:15 INFO: Query executed
auth     | 2026-02-13 14:30:16 INFO: User authenticated
postgres | 2026-02-13 14:30:16 LOG:  query completed
```

### All Services

```bash
nself logs --all
```

**Shows logs from all running services simultaneously.**

### Logs with Timestamps

```bash
nself logs --timestamps postgres
```

**Output**:
```
2026-02-13T14:30:15.123456Z LOG:  database system ready
2026-02-13T14:30:20.789012Z LOG:  autovacuum launcher started
```

### Filter by Pattern (Grep)

```bash
nself logs --grep "ERROR" hasura
```

**Output**:
```
2026-02-13 14:30:15 ERROR: GraphQL validation failed
2026-02-13 14:32:42 ERROR: Database connection lost
```

### Filter by Log Level

```bash
nself logs --level error postgres
```

**Shows only error-level logs.**

### Time-Based Filtering

```bash
# Logs from last hour
nself logs --since 1h postgres

# Logs from specific time
nself logs --since "2026-02-13T14:00:00" postgres

# Logs between times
nself logs --since "14:00:00" --until "14:30:00" postgres
```

### JSON Format

```bash
nself logs --format json hasura
```

**Output**:
```json
{
  "timestamp": "2026-02-13T14:30:15Z",
  "service": "hasura",
  "level": "INFO",
  "message": "Query executed",
  "duration_ms": 45,
  "query_hash": "abc123"
}
```

## Log Levels

### Standard Levels

| Level | Description | Use Case |
|-------|-------------|----------|
| `DEBUG` | Verbose debugging | Development troubleshooting |
| `INFO` | Informational messages | Normal operation |
| `WARN` | Warning messages | Potential issues |
| `ERROR` | Error messages | Failures that need attention |
| `FATAL` | Critical failures | Service crashes |

### Filter by Level

```bash
# Show errors and above
nself logs --level error

# Show warnings and above
nself logs --level warn

# Show everything (debug and above)
nself logs --level debug
```

## Common Log Patterns

### PostgreSQL

```bash
# Slow queries
nself logs postgres | grep "duration:"

# Connection issues
nself logs postgres | grep "connection"

# Errors only
nself logs --level error postgres
```

### Hasura

```bash
# GraphQL queries
nself logs hasura | grep "query"

# Metadata changes
nself logs hasura | grep "metadata"

# Authentication events
nself logs hasura | grep "auth"
```

### Nginx

```bash
# Access logs
nself logs nginx | grep "GET\|POST"

# Error logs
nself logs --level error nginx

# Upstream errors
nself logs nginx | grep "upstream"
```

### Custom Services

```bash
# Application errors
nself logs api | grep "ERROR\|Exception"

# HTTP requests
nself logs api | grep "HTTP"

# Database queries
nself logs api | grep "SELECT\|INSERT\|UPDATE"
```

## Real-Time Monitoring

### Follow Multiple Services

```bash
nself logs -f postgres hasura auth nginx
```

**Output** (color-coded, continuously updating):
```
postgres | Connection received
hasura   | Query executed: 45ms
auth     | JWT token issued
nginx    | GET /api/graphql 200
postgres | Query: SELECT * FROM users
hasura   | Cache hit
```

### Follow with Filtering

```bash
# Follow errors only
nself logs -f --level error --all

# Follow specific pattern
nself logs -f --grep "user_id=123" hasura
```

## Saving Logs

### Save to File

```bash
nself logs postgres > postgres-logs.txt
```

### Save with Timestamps

```bash
nself logs --timestamps --since 24h postgres > postgres-24h.log
```

### Continuous Logging

```bash
# Save to rotating log file
nself logs -f --all | tee -a logs/nself-$(date +%Y%m%d).log
```

## Troubleshooting with Logs

### Service Won't Start

```bash
# Check startup logs
nself logs --tail 100 postgres

# Look for errors
nself logs --level error postgres
```

### Slow Performance

```bash
# PostgreSQL slow queries
nself logs postgres | grep "duration:" | grep "duration: [0-9]\{4,\}"

# Hasura slow queries
nself logs hasura | grep "duration" | grep "ms"
```

### Connection Issues

```bash
# Database connection errors
nself logs hasura | grep "connection"
nself logs auth | grep "database"

# Network errors
nself logs nginx | grep "upstream"
```

### Memory Issues

```bash
# Out of memory errors
nself logs --all | grep "out of memory\|OOM"

# Memory warnings
nself logs postgres | grep "memory"
```

## Log Rotation

### Automatic Rotation

Docker automatically rotates logs based on:
```yaml
# In docker-compose.yml
logging:
  driver: "json-file"
  options:
    max-size: "10m"
    max-file: "3"
```

### Manual Rotation

```bash
# Save current logs
nself logs --all > logs/backup-$(date +%Y%m%d).log

# Truncate logs
docker-compose logs --no-log-prefix > /dev/null

# Or restart services to rotate
nself restart
```

## Advanced Filtering

### Combine Multiple Patterns

```bash
# Errors OR warnings
nself logs hasura | grep -E "ERROR|WARN"

# Multiple services with specific pattern
nself logs postgres hasura | grep "user_id"
```

### Exclude Patterns

```bash
# Everything except health checks
nself logs nginx | grep -v "/health"

# Errors but not connection timeouts
nself logs --level error hasura | grep -v "timeout"
```

### Complex Time Ranges

```bash
# Business hours only
nself logs --since "09:00:00" --until "17:00:00" --all

# Last 30 minutes
nself logs --since 30m --all

# Yesterday
nself logs --since "yesterday 00:00" --until "yesterday 23:59" postgres
```

## Log Analysis

### Count Errors by Service

```bash
for service in postgres hasura auth nginx; do
  count=$(nself logs --level error $service | wc -l)
  echo "$service: $count errors"
done
```

### Extract Query Performance

```bash
# PostgreSQL query times
nself logs postgres | grep "duration:" | \
  sed -E 's/.*duration: ([0-9.]+) ms.*/\1/' | \
  sort -n | tail -20
```

### Traffic Analysis

```bash
# HTTP status codes
nself logs nginx | grep -oE " [0-9]{3} " | sort | uniq -c
```

## Export Formats

### CSV Export

```bash
nself logs --format json hasura | \
  jq -r '[.timestamp, .level, .message] | @csv' > hasura-logs.csv
```

### ELK Stack

```bash
# Send to Elasticsearch
nself logs -f --format json --all | \
  while read line; do
    curl -X POST "localhost:9200/nself-logs/_doc" \
      -H 'Content-Type: application/json' \
      -d "$line"
  done
```

### Syslog

```bash
# Send to syslog
nself logs -f --all | logger -t nself
```

## Related Commands

- `nself status` - Check if services are running
- `nself monitor` - Real-time monitoring dashboard
- `nself health` - Health check diagnostics
- `nself exec` - Execute commands in containers

## See Also

- [nself status](status.md)
- [nself monitor](monitor.md)
- [Debugging Guide](../../guides/DEBUGGING.md)
- [Docker Logs Documentation](https://docs.docker.com/engine/reference/commandline/logs/)
