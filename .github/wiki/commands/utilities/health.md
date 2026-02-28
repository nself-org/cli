# nself health

**Category**: Utilities

Perform comprehensive health checks on all nself services.

## Overview

Run detailed health diagnostics to verify all services are functioning correctly, including connectivity, performance, and integration tests.

**Features**:
- ✅ Service connectivity checks
- ✅ Database health verification
- ✅ GraphQL endpoint testing
- ✅ Authentication system validation
- ✅ Performance benchmarks
- ✅ Dependency checks

## Usage

```bash
nself health [OPTIONS] [SERVICE...]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-v, --verbose` | Show detailed check results | false |
| `--quick` | Skip slow checks | false |
| `--critical-only` | Check only critical services | false |
| `--format FORMAT` | Output format (table/json) | table |
| `--timeout N` | Check timeout (seconds) | 10 |

## Arguments

| Argument | Description |
|----------|-------------|
| `SERVICE` | Check specific service(s) only (optional) |

## Examples

### Basic Health Check

```bash
nself health
```

**Output**:
```
╔═══════════════════════════════════════════════════════════╗
║              nself Health Check Results                   ║
╚═══════════════════════════════════════════════════════════╝

Service          Status    Response    Health Checks
──────────────────────────────────────────────────────────
postgres         ✓ OK      12ms        3/3 passing
hasura           ✓ OK      45ms        2/2 passing
auth             ✓ OK      23ms        2/2 passing
nginx            ✓ OK      5ms         1/1 passing
redis            ✓ OK      2ms         1/1 passing
minio            ✓ OK      15ms        2/2 passing
──────────────────────────────────────────────────────────

Overall Status: ✓ HEALTHY
Services: 6/6 healthy
Average Response: 17ms
```

### Verbose Health Check

```bash
nself health --verbose
```

**Output**:
```
PostgreSQL Health Check
─────────────────────────────────────────
✓ Connection established (12ms)
  Host: localhost:5432
  Database: myapp_db

✓ Database accessible (8ms)
  Tables: 24
  Total rows: 15,423

✓ Replication lag < 1s (0ms)
  Master: online
  Lag: 0.0s

Hasura Health Check
─────────────────────────────────────────
✓ GraphQL endpoint responsive (45ms)
  Endpoint: https://api.localhost/v1/graphql
  Query test: __typename

✓ Metadata synchronized (5ms)
  Tables tracked: 24
  Relationships: 15

✓ Database connection healthy (3ms)
  Pool size: 10
  Active connections: 2

Auth Service Health Check
─────────────────────────────────────────
✓ API endpoint responsive (23ms)
  Endpoint: https://auth.localhost/healthz

✓ JWT signing functional (2ms)
  Algorithm: HS256
  Test token generated: valid

✓ Database connectivity (5ms)
  Users table accessible
  Sessions table accessible
```

### Check Specific Service

```bash
nself health postgres
```

**Output**:
```
PostgreSQL Health Check

Connection:     ✓ OK (12ms)
Database:       ✓ OK (8ms)
Replication:    ✓ OK (0ms)
Disk Space:     ✓ OK (85% used)
Connections:    ✓ OK (5/100)

Overall: ✓ HEALTHY
```

### Quick Check (Critical Only)

```bash
nself health --quick
```

**Skips**:
- Performance benchmarks
- Detailed connection pool checks
- Slow integration tests

**Output**:
```
Quick Health Check (critical services only)

postgres    ✓ OK
hasura      ✓ OK
auth        ✓ OK
nginx       ✓ OK

Status: ✓ ALL CRITICAL SERVICES HEALTHY
```

### JSON Output

```bash
nself health --format json
```

**Output**:
```json
{
  "overall_status": "healthy",
  "timestamp": "2026-02-13T14:30:15Z",
  "services": {
    "postgres": {
      "status": "healthy",
      "response_time_ms": 12,
      "checks": {
        "connection": { "status": "pass", "time_ms": 12 },
        "database": { "status": "pass", "time_ms": 8 },
        "replication": { "status": "pass", "time_ms": 0 }
      }
    },
    "hasura": {
      "status": "healthy",
      "response_time_ms": 45,
      "checks": {
        "graphql": { "status": "pass", "time_ms": 45 },
        "metadata": { "status": "pass", "time_ms": 5 }
      }
    }
  },
  "summary": {
    "total_services": 6,
    "healthy": 6,
    "unhealthy": 0,
    "average_response_ms": 17
  }
}
```

## Health Check Categories

### Connectivity Checks

Tests basic network connectivity and service availability.

**PostgreSQL**:
- TCP connection to port 5432
- Database login with credentials
- Query execution

**Hasura**:
- HTTP connection to GraphQL endpoint
- Admin secret authentication
- Simple query execution

**Auth**:
- HTTP connection to auth endpoint
- Health endpoint response
- JWT generation test

### Performance Checks

Measures response times and performance metrics.

**Database Query Performance**:
```sql
SELECT pg_stat_statements_reset();
-- Run test queries
SELECT avg(total_time) FROM pg_stat_statements;
```

**GraphQL Performance**:
```graphql
query HealthCheckQuery {
  __typename
}
```

**Response Time Thresholds**:
- ✓ Good: < 50ms
- ⚠ Warning: 50-200ms
- ✗ Critical: > 200ms

### Integration Checks

Verifies services work together correctly.

**Database → Hasura**:
- Hasura can query database
- Schema sync is current
- Permissions applied correctly

**Auth → Database**:
- User authentication works
- Sessions persisted
- JWT tokens valid

**Nginx → Upstream Services**:
- Proxy routing functional
- SSL termination working
- Load balancing operational

## Exit Codes

| Code | Status | Meaning |
|------|--------|---------|
| 0 | Healthy | All checks passed |
| 1 | Unhealthy | One or more checks failed |
| 2 | Warning | Some checks slow/degraded |
| 3 | Critical | Core services down |

**Use in scripts**:
```bash
if nself health > /dev/null 2>&1; then
  echo "Services healthy, proceeding..."
else
  echo "Services unhealthy, aborting!"
  exit 1
fi
```

## Troubleshooting

### Service Fails Health Check

```bash
# Check which specific check failed
nself health --verbose postgres

# View logs for errors
nself logs postgres --tail 50

# Restart service
nself restart postgres

# Re-check health
nself health postgres
```

### Slow Response Times

```bash
# Check system resources
nself status --resources

# Check for slow queries
nself db shell -c "
SELECT query, mean_time
FROM pg_stat_statements
ORDER BY mean_time DESC
LIMIT 10;
"

# Review connection pool
nself health --verbose hasura | grep "Active connections"
```

### Intermittent Failures

```bash
# Run health check multiple times
for i in {1..10}; do
  nself health --quick
  sleep 5
done

# Monitor for pattern in failures
```

## Automated Health Monitoring

### Continuous Monitoring Script

```bash
#!/bin/bash
# health-monitor.sh

while true; do
  if ! nself health --quick > /dev/null 2>&1; then
    echo "[$(date)] Health check failed!" | tee -a health.log
    nself health >> health.log

    # Send alert
    curl -X POST https://hooks.slack.com/... \
      -d '{"text":"nself services unhealthy!"}'
  fi
  sleep 60
done
```

### Cron Job

```bash
# Check health every 5 minutes
*/5 * * * * cd /path/to/project && nself health --quick || echo "Health check failed" | mail -s "nself Alert" admin@example.com
```

### Kubernetes Liveness Probe

```yaml
livenessProbe:
  exec:
    command:
    - nself
    - health
    - --quick
  initialDelaySeconds: 30
  periodSeconds: 60
```

## Health Check Configuration

### Custom Timeout

```bash
# In .env
HEALTH_CHECK_TIMEOUT=30

# Or via flag
nself health --timeout 30
```

### Disable Specific Checks

```bash
# In .env
SKIP_PERFORMANCE_CHECKS=true
SKIP_INTEGRATION_TESTS=true
```

### Custom Health Endpoints

```bash
# In .env
CUSTOM_HEALTH_ENDPOINT_1=https://api.example.com/health
CUSTOM_HEALTH_ENDPOINT_2=https://worker.example.com/health
```

## Health Metrics

### Track Health Over Time

```bash
# Save health check results
nself health --format json >> health-history.jsonl

# Analyze historical data
cat health-history.jsonl | jq '.summary.average_response_ms'
```

### Generate Health Report

```bash
# Daily health report
nself health --verbose > reports/health-$(date +%Y%m%d).txt
```

### Integration with Monitoring

```bash
# Export to Prometheus
nself health --format json | jq -r '
  .services[] |
  "nself_health_response_time{service=\"\(.name)\"} \(.response_time_ms)"
' | curl --data-binary @- http://prometheus:9091/metrics/job/nself
```

## Best Practices

### 1. Check Before Deployment

```bash
# Pre-deployment health check
if ! nself health; then
  echo "Services unhealthy, aborting deployment"
  exit 1
fi

# Deploy
./deploy.sh

# Post-deployment health check
sleep 10
nself health
```

### 2. Regular Scheduled Checks

```bash
# Hourly health checks
0 * * * * nself health --quick >> /var/log/nself-health.log
```

### 3. Alert on Failures

```bash
# Health check with alerting
if ! nself health --critical-only; then
  # Send PagerDuty alert
  # Send email alert
  # Send Slack notification
fi
```

### 4. Document Expected Response Times

```bash
# Create baseline
nself health --format json > baseline-health.json

# Compare against baseline
nself health --format json > current-health.json
diff baseline-health.json current-health.json
```

## Related Commands

- `nself status` - Service running status
- `nself logs` - View service logs
- `nself monitor` - Real-time monitoring
- `nself doctor` - Diagnostic tool

## See Also

- [nself status](status.md)
- [nself doctor](doctor.md)
- [nself monitor](monitor.md)
- [Health Check Guide](../../guides/HEALTH-MONITORING.md)
