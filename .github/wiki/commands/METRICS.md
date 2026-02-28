# Metrics Command

Configure and manage the monitoring stack for your nself project.

## Quick Start

```bash
# Enable monitoring with smart defaults
nself metrics enable

# Or choose a specific profile
nself metrics enable minimal    # Metrics only (~1GB RAM)
nself metrics enable standard   # Metrics + Logs (~2GB RAM)
nself metrics enable full       # Complete observability (~3-4GB RAM)

# Check status
nself metrics status

# Open Grafana
nself metrics dashboard
```

## Commands

| Command | Description |
|---------|-------------|
| `nself metrics` | Show help |
| `nself metrics enable [profile]` | Enable monitoring |
| `nself metrics disable` | Disable monitoring |
| `nself metrics status` | Show monitoring status |
| `nself metrics profile [name]` | View/change profile |
| `nself metrics config [key] [value]` | Configure settings |
| `nself metrics dashboard` | Open Grafana |

## Monitoring Profiles

### Minimal (~1GB RAM)

Metrics only - lightweight monitoring:

```bash
nself metrics enable minimal
```

**Components:**
- Prometheus (metrics database)
- Grafana (visualization)
- cAdvisor (container metrics)

**Best for:** Development, resource-constrained environments

### Standard (~2GB RAM)

Metrics + Logs:

```bash
nself metrics enable standard
```

**Components:**
- Everything in minimal, plus:
- Loki (log aggregation)
- Promtail (log collection)

**Best for:** Staging environments, debugging

### Full (~3-4GB RAM)

Complete observability:

```bash
nself metrics enable full
```

**Components:**
- Everything in standard, plus:
- Tempo (distributed tracing)
- Alertmanager (alerting)
- Node Exporter (host metrics)
- PostgreSQL Exporter
- Redis Exporter (if Redis enabled)

**Best for:** Production environments

### Auto (Smart Defaults)

Automatically selects based on environment:

```bash
nself metrics enable auto
```

| ENV | Profile |
|-----|---------|
| dev | minimal |
| staging | standard |
| prod | full |

## Grafana Dashboard

Access at `https://grafana.<your-domain>` or via:

```bash
nself metrics dashboard
```

### Default Credentials

```
Username: admin
Password: (check .env or run `nself metrics status`)
```

### Pre-built Dashboards

- **Container Overview** - CPU, memory, network per container
- **Service Health** - Uptime, response times
- **Resource Usage** - System-wide metrics
- **Logs Explorer** - Search and filter logs (if Loki enabled)

## Prometheus Metrics

Access at `https://prometheus.<your-domain>`

### Example Queries

```promql
# Container CPU usage
rate(container_cpu_usage_seconds_total[5m]) * 100

# Container memory
container_memory_usage_bytes

# Request rate
rate(http_requests_total[5m])

# Error rate
rate(http_requests_total{status=~"5.."}[5m])
```

## Log Aggregation (Loki)

When standard or full profile is enabled:

```bash
# View logs in Grafana
nself metrics dashboard

# Go to Explore > Loki
```

### LogQL Examples

```logql
# All logs from nginx
{container_name="nginx"}

# Error logs
{container_name=~".+"} |= "error"

# JSON log parsing
{container_name="api"} | json | level="error"
```

## Distributed Tracing (Tempo)

When full profile is enabled, configure your apps to send traces:

```javascript
// OpenTelemetry example
const { NodeTracerProvider } = require('@opentelemetry/sdk-trace-node');
const { OTLPTraceExporter } = require('@opentelemetry/exporter-trace-otlp-grpc');

const exporter = new OTLPTraceExporter({
  url: 'http://tempo:4317'
});
```

## Alerting

When full profile is enabled:

### Built-in Alerts

- **HighCPUUsage** - Container CPU > 80%
- **HighMemoryUsage** - Container memory > 80%
- **ServiceDown** - Service not responding
- **DiskSpaceLow** - Less than 10% disk space
- **HighErrorRate** - Error rate > 5%

### Configure Notifications

```bash
# In .env
ALERTMANAGER_WEBHOOK_URL=https://hooks.slack.com/services/...
ALERTMANAGER_EMAIL_TO=ops@example.com
ALERTMANAGER_PAGERDUTY_KEY=your-key
```

## Configuration

### View Current Config

```bash
nself metrics config
```

### Modify Settings

```bash
# Set Grafana password
nself metrics config GRAFANA_ADMIN_PASSWORD mypassword

# Set retention period
nself metrics config PROMETHEUS_RETENTION 30d

# Set memory limits
nself metrics config PROMETHEUS_MEMORY_LIMIT 2Gi
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MONITORING_ENABLED` | Enable monitoring | `false` |
| `MONITORING_PROFILE` | Profile to use | `auto` |
| `GRAFANA_ADMIN_USER` | Grafana username | `admin` |
| `GRAFANA_ADMIN_PASSWORD` | Grafana password | auto-generated |
| `PROMETHEUS_RETENTION` | Metrics retention | `15d` |
| `PROMETHEUS_MEMORY_LIMIT` | Memory limit | `1Gi` |
| `LOKI_RETENTION` | Log retention | `7d` |
| `TEMPO_RETENTION` | Trace retention | `72h` |

## Resource Usage

| Profile | Containers | RAM | Disk/day |
|---------|------------|-----|----------|
| minimal | 3 | ~1GB | ~100MB |
| standard | 5 | ~2GB | ~500MB |
| full | 10 | ~4GB | ~1GB |

## Disabling Individual Components

After enabling a profile, you can disable specific components:

```bash
# In .env
MONITORING_ENABLED=true
MONITORING_PROFILE=full
TEMPO_ENABLED=false  # Disable tracing
```

## Best Practices

1. **Start minimal** - Add components as needed
2. **Set retention** - Don't keep data forever
3. **Use alerts** - Don't just collect metrics
4. **Monitor the monitors** - Watch resource usage
5. **Archive important data** - Export before retention expires

## Troubleshooting

### High Memory Usage

```bash
# Check resource usage
nself metrics status

# Switch to lighter profile
nself metrics profile minimal
```

### Missing Metrics

```bash
# Check Prometheus targets
open https://prometheus.<domain>/targets

# Check if containers are labeled correctly
docker inspect <container> | grep Labels
```

### Logs Not Appearing

```bash
# Check Promtail
nself logs promtail

# Verify Loki is receiving
curl http://localhost:3100/ready
```
