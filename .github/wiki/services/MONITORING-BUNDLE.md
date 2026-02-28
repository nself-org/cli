# nself Monitoring Bundle - Complete Reference

## Overview

The monitoring bundle is activated with a **single environment variable**:
```bash
MONITORING_ENABLED=true
```

This automatically enables **10 monitoring services** with smart dependency handling.

---

## The 10 Monitoring Services

### Core Observability Stack (4 services)

#### 1. **Prometheus** - Metrics Collection & Storage
- **Purpose**: Time-series database for metrics
- **What it does**: Scrapes and stores metrics from all services
- **Port**: 9090 (default)
- **Always needed?**: ✅ YES - Core of monitoring stack
- **Dependencies**: None
- **Scrapes metrics from**:
  - Itself (prometheus:9090)
  - Node Exporter (system metrics)
  - cAdvisor (container metrics)
  - Postgres Exporter (database metrics)
  - Redis Exporter (Redis metrics)
  - Hasura (application metrics)
  - Custom services (if configured)

#### 2. **Grafana** - Visualization & Dashboards
- **Purpose**: Create dashboards and visualize metrics/logs
- **What it does**: Provides UI for viewing Prometheus metrics and Loki logs
- **Port**: 3000 (default)
- **Always needed?**: ⚠️ RECOMMENDED - Without it, you can't see the data visually
- **Dependencies**: Prometheus (for metrics), Loki (for logs)
- **Access**: https://grafana.local.nself.org
- **Default credentials**: admin/admin (change via GRAFANA_ADMIN_PASSWORD)

#### 3. **Loki** - Log Aggregation
- **Purpose**: Collect and query logs from all containers
- **What it does**: Stores logs in a queryable format
- **Port**: 3100 (default)
- **Always needed?**: ⚠️ RECOMMENDED - Logs are critical for debugging
- **Dependencies**: None (but needs Promtail to ship logs to it)
- **Similar to**: Elasticsearch, but more lightweight

#### 4. **Promtail** - Log Shipping Agent
- **Purpose**: Ships logs from Docker containers to Loki
- **What it does**: Reads Docker container logs and forwards to Loki
- **Port**: 9080 (internal only)
- **Always needed?**: ✅ YES if using Loki - Without it, Loki receives no logs
- **Dependencies**: Loki (must be running)
- **Critical**: This is the connector between containers and Loki

---

### Distributed Tracing (1 service)

#### 5. **Tempo** - Distributed Tracing Backend
- **Purpose**: Track requests across multiple services
- **What it does**: Stores and queries distributed traces (like Jaeger)
- **Ports**: 3200 (HTTP), 14268 (Jaeger ingest)
- **Always needed?**: ❌ NO - Only if you're doing distributed tracing
- **Dependencies**: None
- **Use case**: Track a request from frontend → API → database → external service
- **Optional**: Can disable with `TEMPO_ENABLED=false`

---

### Alerting (1 service)

#### 6. **Alertmanager** - Alert Routing & Management
- **Purpose**: Route and manage alerts from Prometheus
- **What it does**:
  - Receives alerts from Prometheus
  - Groups, deduplicates, silences alerts
  - Routes to notification channels (email, Slack, PagerDuty)
- **Port**: 9093 (default)
- **Always needed?**: ❌ NO - Only if you want alerting
- **Dependencies**: Prometheus (sends alerts to it)
- **Use case**: Get notified when CPU > 90%, disk full, service down
- **Optional**: Can disable with `ALERTMANAGER_ENABLED=false`

---

### Metrics Exporters (4 services)

#### 7. **cAdvisor** - Container Metrics Exporter
- **Purpose**: Expose Docker container metrics to Prometheus
- **What it does**:
  - Monitors CPU, memory, disk, network usage per container
  - Provides /metrics endpoint for Prometheus
- **Port**: 8082 (default)
- **Always needed?**: ✅ YES for container monitoring
- **Dependencies**: Docker daemon
- **Metrics exposed**: Container CPU, memory, disk I/O, network
- **OS-aware**: Adjusts volume mounts for macOS vs Linux

#### 8. **Node Exporter** - Host/System Metrics Exporter
- **Purpose**: Expose host system metrics to Prometheus
- **What it does**:
  - Monitors CPU, memory, disk, network at the host level
  - Shows overall system health
- **Port**: 9100 (default)
- **Always needed?**: ✅ YES for system monitoring
- **Dependencies**: None
- **Metrics exposed**: System CPU, memory, disk usage, file descriptors, network

#### 9. **Postgres Exporter** - PostgreSQL Metrics Exporter
- **Purpose**: Expose PostgreSQL database metrics to Prometheus
- **What it does**:
  - Monitors database connections, queries, locks, table sizes
  - Tracks slow queries, replication lag
- **Port**: 9187 (default)
- **Always needed?**: ✅ YES - PostgreSQL is always required in nself
- **Dependencies**: PostgreSQL service (always present as required service)
- **Note**: Since PostgreSQL is a required service, this exporter is always included in the monitoring bundle

#### 10. **Redis Exporter** - Redis Metrics Exporter
- **Purpose**: Expose Redis metrics to Prometheus
- **What it does**:
  - Monitors Redis memory, connections, hit rate, key count
  - Tracks command stats, replication
- **Port**: 9121 (default)
- **Always needed?**: ⚠️ CONDITIONAL - Only if Redis is enabled
- **Dependencies**: Redis service (REDIS_ENABLED=true)
- **Smart logic**: **Automatically disabled if Redis not enabled**

---

## Smart Dependency Handling

### ✅ What Works Automatically

When you set `MONITORING_ENABLED=true`, the system:

1. **Enables all 10 services** with smart defaults
2. **Checks dependencies** before starting each exporter
3. **Auto-disables Redis Exporter** if Redis isn't enabled

### 🔍 Dependency Logic

```bash
# In monitoring-exporters.sh

# Postgres Exporter - Always included (PostgreSQL is required)
[[ "${POSTGRES_EXPORTER_ENABLED:-false}" != "true" ]] && return 0
# Note: No POSTGRES_ENABLED check - PostgreSQL is always present

# Redis Exporter - Only if Redis is enabled
[[ "${REDIS_EXPORTER_ENABLED:-false}" != "true" ]] && return 0
[[ "${REDIS_ENABLED:-false}" != "true" ]] && return 0     # ← Smart check!
```

**This means:**
- ✅ Postgres Exporter is **always included** (PostgreSQL is a required service)
- ⚠️ Redis Exporter is **only included** if Redis is enabled

### Example Scenarios

#### Scenario 1: Minimal + Monitoring
```bash
MONITORING_ENABLED=true
# No Redis, no MinIO, no other optional services
```

**Result**:
- ✅ Prometheus, Grafana, Loki, Promtail (4)
- ✅ Tempo, Alertmanager (2)
- ✅ cAdvisor, Node Exporter, Postgres Exporter (3)
- ❌ Redis Exporter (0) - Redis not enabled
- **Total: 9 containers** (full monitoring except Redis Exporter)

#### Scenario 2: Standard + Monitoring
```bash
REDIS_ENABLED=true
MINIO_ENABLED=true
# ... other optional services
MONITORING_ENABLED=true
```

**Result**:
- ✅ All 10 monitoring services
- ✅ Postgres Exporter (PostgreSQL present)
- ✅ Redis Exporter (Redis present)
- **Total: 10 containers** (full monitoring stack)

---

## Configuration Options

### Simple Approach (Recommended)
```bash
# .env
MONITORING_ENABLED=true
```

This enables **everything** with sensible defaults.

### Advanced Approach (Granular Control)
```bash
# .env
MONITORING_ENABLED=true

# Optionally disable specific services
TEMPO_ENABLED=false           # Don't need tracing
ALERTMANAGER_ENABLED=false    # Don't need alerting yet

# Customize ports
GRAFANA_PORT=3001
PROMETHEUS_PORT=9091

# Customize Grafana
GRAFANA_ADMIN_PASSWORD=mysecretpassword
```

### Individual Service Enable (Without Bundle)
```bash
# .env
# Don't set MONITORING_ENABLED=true

# Enable only what you need
PROMETHEUS_ENABLED=true
GRAFANA_ENABLED=true
NODE_EXPORTER_ENABLED=true
CADVISOR_ENABLED=true
```

---

## Service Categories Summary

### Always Needed for Monitoring
1. ✅ **Prometheus** - Core metrics database
2. ✅ **Grafana** - Visualization (technically optional, but you want this)
3. ✅ **cAdvisor** - Container metrics
4. ✅ **Node Exporter** - System metrics

### Includes Database Monitoring
5. ✅ **Postgres Exporter** - Always included (PostgreSQL is required)
6. ⚠️ **Redis Exporter** - Only if Redis enabled

### Logging Stack (Recommended)
7. ✅ **Loki** - Log storage
8. ✅ **Promtail** - Log shipping (required for Loki to work)

### Optional/Advanced
9. ❌ **Tempo** - Only for distributed tracing
10. ❌ **Alertmanager** - Only for alerting

---

## Resource Usage

### Minimal Monitoring (4 services)
- Prometheus, Grafana, cAdvisor, Node Exporter
- Memory: ~500MB
- Good for: Development

### Standard Monitoring (8 services)
- Above + Loki, Promtail, Postgres Exporter, Redis Exporter
- Memory: ~1GB
- Good for: Staging

### Full Monitoring (10 services)
- All services including Tempo and Alertmanager
- Memory: ~1.2GB
- Good for: Production

---

## Prometheus Scrape Configuration

When `MONITORING_ENABLED=true`, Prometheus automatically scrapes:

```yaml
# monitoring/prometheus/prometheus.yml (auto-generated)
scrape_configs:
  - job_name: 'prometheus'        # Itself
  - job_name: 'node'              # Node Exporter
  - job_name: 'cadvisor'          # cAdvisor
  - job_name: 'postgres'          # Postgres Exporter
  - job_name: 'redis'             # Redis Exporter
  - job_name: 'hasura'            # Hasura GraphQL
  - job_name: 'custom_*'          # Custom services (auto-added)
```

**Important**: The config is smart - it includes targets even if they're not running. Prometheus will show them as "DOWN" which is useful for debugging.

---

## When to Use What

### Development Environment
```bash
MONITORING_ENABLED=false
```
Or enable minimal:
```bash
PROMETHEUS_ENABLED=true
GRAFANA_ENABLED=true
```

### Staging Environment
```bash
MONITORING_ENABLED=true
TEMPO_ENABLED=false          # Skip tracing
ALERTMANAGER_ENABLED=false   # Skip alerting
```

### Production Environment
```bash
MONITORING_ENABLED=true
# Enable everything!
GRAFANA_ADMIN_PASSWORD=strong-password
```

---

## Access URLs

When monitoring is enabled:

| Service | URL | Purpose |
|---------|-----|---------|
| Grafana | https://grafana.local.nself.org | Dashboards & visualization |
| Prometheus | https://prometheus.local.nself.org | Metrics query interface |
| Alertmanager | https://alertmanager.local.nself.org | Alert management |

Exporters don't have public URLs - they're internal services scraped by Prometheus.

---

## Common Questions

### Q: Do I need all 10 services?
**A**: No. You need at minimum:
- Prometheus (metrics)
- Grafana (visualization)
- cAdvisor (container metrics)
- Node Exporter (system metrics)

### Q: What if I don't enable Redis but monitoring is on?
**A**: Redis Exporter won't start. The system detects Redis isn't enabled and skips it. No errors, no wasted resources.

### Q: Can I add monitoring to an existing project?
**A**: Yes! Just add `MONITORING_ENABLED=true` to your .env and run `nself build --force && nself start`

### Q: How much overhead does monitoring add?
**A**: Minimal. Exporters use <50MB each. Prometheus/Grafana are the heaviest (~200-300MB each).

### Q: Can I disable individual services?
**A**: Yes. Even with `MONITORING_ENABLED=true`, you can set:
```bash
TEMPO_ENABLED=false
ALERTMANAGER_ENABLED=false
```

### Q: What happens if I enable Redis after monitoring is already running?
**A**: Run `nself build --force` to regenerate config, then `nself start`. Redis Exporter will be added automatically.

---

## Implementation Details

### Auto-Enable Logic (service-detection.sh)
```bash
if [[ "$MONITORING_ENABLED" == "true" ]]; then
  export PROMETHEUS_ENABLED="${PROMETHEUS_ENABLED:-true}"
  export GRAFANA_ENABLED="${GRAFANA_ENABLED:-true}"
  export LOKI_ENABLED="${LOKI_ENABLED:-true}"
  export PROMTAIL_ENABLED="${PROMTAIL_ENABLED:-true}"
  export TEMPO_ENABLED="${TEMPO_ENABLED:-true}"
  export ALERTMANAGER_ENABLED="${ALERTMANAGER_ENABLED:-true}"
  export CADVISOR_ENABLED="${CADVISOR_ENABLED:-true}"
  export NODE_EXPORTER_ENABLED="${NODE_EXPORTER_ENABLED:-true}"
  export POSTGRES_EXPORTER_ENABLED="${POSTGRES_EXPORTER_ENABLED:-true}"
  export REDIS_EXPORTER_ENABLED="${REDIS_EXPORTER_ENABLED:-true}"
fi
```

### Conditional Exporter Logic (monitoring-exporters.sh)
```bash
# Postgres Exporter - Always included (PostgreSQL is required)
generate_postgres_exporter_service() {
  [[ "${POSTGRES_EXPORTER_ENABLED:-false}" != "true" ]] && return 0
  # Note: No POSTGRES_ENABLED check - PostgreSQL is always present
  # ... generate service
}

# Redis Exporter - Only if Redis is enabled
generate_redis_exporter_service() {
  [[ "${REDIS_EXPORTER_ENABLED:-false}" != "true" ]] && return 0
  [[ "${REDIS_ENABLED:-false}" != "true" ]] && return 0     # ← Smart check!
  # ... generate service
}
```

---

## Summary

✅ **Single variable**: `MONITORING_ENABLED=true`
✅ **10 services**: 4 core + 2 logging + 1 tracing + 1 alerting + 2 exporters (conditional)
✅ **Smart dependencies**: Exporters only start if their target service exists
✅ **Zero configuration**: Works out of the box with sensible defaults
✅ **Fully customizable**: Override any setting as needed

**The system is smart enough to handle dependencies automatically. You don't need to worry about enabling/disabling exporters based on optional services - it's handled for you!**

---

## Quick Reference: The 10 Services

| # | Service | Purpose | Always On? | Notes |
|---|---------|---------|------------|-------|
| 1 | **Prometheus** | Metrics database | ✅ YES | Core monitoring |
| 2 | **Grafana** | Dashboards | ✅ YES | Visualization |
| 3 | **Loki** | Log storage | ✅ YES | Log aggregation |
| 4 | **Promtail** | Log shipper | ✅ YES | Required for Loki |
| 5 | **Tempo** | Tracing | ❌ Optional | Can disable |
| 6 | **Alertmanager** | Alerts | ❌ Optional | Can disable |
| 7 | **cAdvisor** | Container metrics | ✅ YES | Container monitoring |
| 8 | **Node Exporter** | System metrics | ✅ YES | Host monitoring |
| 9 | **Postgres Exporter** | DB metrics | ✅ YES | PostgreSQL always present |
| 10 | **Redis Exporter** | Redis metrics | ⚠️ Conditional | Only if `REDIS_ENABLED=true` |

**Summary**:
- **9 services** always included when `MONITORING_ENABLED=true` (without Redis)
- **10 services** included when Redis is also enabled
- PostgreSQL is a required service, so its exporter is always included
- Only Redis Exporter has conditional logic based on optional service

---

## Key Takeaway

✅ **PostgreSQL is always required** → Postgres Exporter always included
⚠️ **Redis is optional** → Redis Exporter only included if Redis enabled

The system handles this automatically - you don't need to manually configure anything!

