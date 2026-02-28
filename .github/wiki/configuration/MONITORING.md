# Monitoring Configuration

Monitor nself services with Grafana, Prometheus, and more.

## Enable Monitoring

```bash
# In .env
MONITORING_ENABLED=true

nself build
nself start
```

## Access Monitoring

```bash
# Open Grafana dashboards
nself monitor

# View Prometheus metrics
http://localhost:8080/prometheus

# View Loki logs
http://localhost:8080/loki
```

See [Monitoring Complete Guide](../guides/MONITORING-COMPLETE.md) for details.

