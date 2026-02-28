# Monitor Command

Access monitoring dashboards and view real-time service status.

## Quick Start

```bash
# Open Grafana (default)
nself monitor

# View service status in terminal
nself monitor services

# View resource usage
nself monitor resources
```

## Commands

| Command | Description |
|---------|-------------|
| `nself monitor` | Open Grafana dashboard (default) |
| `nself monitor dashboard` | Open Grafana dashboard |
| `nself monitor grafana` | Open Grafana dashboard |
| `nself monitor prometheus` | Open Prometheus UI |
| `nself monitor loki` | Open Loki in Grafana Explore |
| `nself monitor alerts` | Open Alertmanager UI |
| `nself monitor services` | Show service status (CLI) |
| `nself monitor resources` | Show resource usage (CLI) |
| `nself monitor logs [service]` | Tail service logs (CLI) |

## Prerequisites

Monitoring must be enabled first:

```bash
nself metrics enable
```

## Dashboard Access

### Grafana

Primary visualization dashboard:

```bash
nself monitor dashboard
# or
nself monitor grafana
```

**URL:** `https://grafana.<your-domain>`

Features:
- Pre-built dashboards
- Custom dashboard creation
- Alert visualization
- Log exploration (if Loki enabled)
- Trace exploration (if Tempo enabled)

### Prometheus

Metrics database and query interface:

```bash
nself monitor prometheus
```

**URL:** `https://prometheus.<your-domain>`

Features:
- PromQL query interface
- Target health status
- Alert rules
- Service discovery

### Loki (Logs)

Log aggregation (requires standard or full profile):

```bash
nself monitor loki
```

Opens Grafana Explore with Loki data source selected.

### Alertmanager

Alert management (requires full profile):

```bash
nself monitor alerts
```

**URL:** `https://alerts.<your-domain>`

Features:
- Active alerts
- Alert silencing
- Alert grouping
- Notification history

## CLI Views

### Service Status

View service health in terminal:

```bash
nself monitor services
```

Output:
```
╔══════════════════════════════════════════════════════════════╗
║                     SERVICE STATUS                           ║
╚══════════════════════════════════════════════════════════════╝

Core Services:
  • Nginx:               ● Running
  • PostgreSQL:          ● Running
  • Hasura:              ● Running
  • Auth:                ● Running

Monitoring Stack:
  • Prometheus:          ● Running
  • Grafana:             ● Running
  • cAdvisor:            ● Running
```

### Resource Usage

View resource consumption:

```bash
nself monitor resources
```

Output:
```
╔══════════════════════════════════════════════════════════════╗
║                    RESOURCE USAGE                            ║
╚══════════════════════════════════════════════════════════════╝

CONTAINER                      CPU %        MEMORY      MEMORY %
---------                      -----        ------      --------
nginx                          0.5%         15MiB       0.2%
postgres                       2.1%         256MiB      3.2%
hasura                         1.2%         128MiB      1.6%
grafana                        0.8%         85MiB       1.1%
prometheus                     3.5%         512MiB      6.4%

Total containers: 15
```

### Log Tailing

View service logs in real-time:

```bash
# List available services
nself monitor logs

# Tail specific service
nself monitor logs nginx
nself monitor logs hasura
nself monitor logs prometheus
```

Press `Ctrl+C` to stop tailing.

## Available Dashboards

When monitoring is enabled, these dashboards are pre-configured:

| Dashboard | Description |
|-----------|-------------|
| Container Overview | CPU, memory, network by container |
| Service Health | Uptime, latency, error rates |
| PostgreSQL | Database metrics, queries |
| Redis | Cache metrics (if enabled) |
| Nginx | Request rates, response times |
| System | Host-level metrics |

## Health Indicators

### Service States

| Indicator | Meaning |
|-----------|---------|
| ● Running | Service is healthy |
| ● Starting | Service is initializing |
| ● Unhealthy | Service failing health checks |
| ○ Stopped | Service not running |

### Resource Colors

| Color | CPU/Memory Usage |
|-------|------------------|
| Green | < 50% |
| Yellow | 50-80% |
| Red | > 80% |

## Accessing Without CLI

If you prefer direct URLs:

| Service | URL |
|---------|-----|
| Grafana | `https://grafana.<domain>` |
| Prometheus | `https://prometheus.<domain>` |
| Alertmanager | `https://alerts.<domain>` |
| Loki | Via Grafana Explore |

## Credentials

Default Grafana credentials:

```bash
# View credentials
nself metrics status

# Or check .env
grep GRAFANA .env
```

## Best Practices

1. **Bookmark Grafana** - Most common dashboard
2. **Create custom dashboards** - For your specific needs
3. **Set up alerts** - Don't just watch, be notified
4. **Use CLI for quick checks** - Faster than opening browser
5. **Check resources regularly** - Prevent issues before they occur

## Troubleshooting

### "Monitoring is not enabled"

```bash
nself metrics enable
nself build && nself restart
```

### Dashboard Not Loading

```bash
# Check if Grafana is running
nself status grafana

# View Grafana logs
nself logs grafana
```

### Missing Data

```bash
# Check Prometheus targets
open https://prometheus.<domain>/targets

# Verify data collection
nself monitor resources
```

### Connection Refused

```bash
# Ensure services are running
nself start

# Check specific service
nself status prometheus
nself status grafana
```
