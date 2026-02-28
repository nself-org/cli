# nself perf scale - Resource Scaling

> **⚠️ DEPRECATED in v0.9.6**: This command has been consolidated.
> Please use `nself perf scale` instead.
> See [Command Consolidation Map](../architecture/COMMAND-CONSOLIDATION-MAP.md) and [v0.9.6 Release Notes](../releases/v0.9.6.md) for details.

**Version 0.4.7** | Service resource scaling and auto-scaling

---

## Overview

The `nself perf scale` command manages resource allocation and scaling for services. Set CPU limits, memory limits, replicas, and configure auto-scaling based on load.

---

## Usage

```bash
nself perf scale [options] <service> [replicas]
nself perf scale --list
```

---

## Options

| Option | Description |
|--------|-------------|
| `--cpu <limit>` | Set CPU limit (e.g., 2, 1.5, 0.5) |
| `--memory <limit>` | Set memory limit (e.g., 2G, 512M) |
| `--replicas <n>` | Set number of replicas |
| `--auto` | Enable auto-scaling |
| `--min <n>` | Minimum replicas for auto-scaling |
| `--max <n>` | Maximum replicas for auto-scaling |
| `--cpu-target <n>` | CPU target percentage for auto-scaling |
| `--list` | List current resource allocations |
| `-h, --help` | Show help message |

---

## Services

The following services can be scaled:

| Service | Description |
|---------|-------------|
| `postgres` | PostgreSQL database |
| `hasura` | Hasura GraphQL engine |
| `hasura-auth` | Authentication service |
| `hasura-storage` | Storage service |
| `nginx` | Web server |
| `redis` | Redis cache |
| `functions` | Serverless functions |

---

## Examples

### List Current Allocations

```bash
nself perf scale --list
```

Shows a table of all services with their CPU limits, memory limits, and current usage.

### Set Resource Limits

```bash
# Set PostgreSQL to 4GB memory and 2 CPUs
nself perf scale postgres --memory 4G --cpu 2

# Increase Hasura memory
nself perf scale hasura --memory 2G

# Set Redis CPU limit
nself perf scale redis --cpu 0.5
```

### Horizontal Scaling

```bash
# Scale Hasura to 3 replicas
nself perf scale hasura --replicas 3

# Scale nginx to 2 replicas
nself perf scale nginx --replicas 2
```

### Auto-Scaling

```bash
# Enable auto-scaling for nginx (defaults: min=1, max=10, cpu-target=70%)
nself perf scale nginx --auto

# Configure auto-scaling with limits
nself perf scale hasura --auto --min 2 --max 10 --cpu-target 80

# Scale between 1 and 5 replicas based on 60% CPU
nself perf scale functions --auto --min 1 --max 5 --cpu-target 60
```

---

## Auto-Scaling Configuration

When auto-scaling is enabled, the configuration is stored in `.nself/autoscale-<service>.conf`:

```ini
MIN_REPLICAS=2
MAX_REPLICAS=10
CPU_TARGET=80
ENABLED=true
```

Auto-scaling is managed by the monitoring system when available.

---

## How It Works

1. **Resource Limits**: Applied via `docker-compose.override.yml`
2. **Replicas**: Docker Compose deploy replicas
3. **Changes Applied**: Service automatically restarted after configuration

The override file preserves your custom resource configurations across `nself build` runs.

---

## Best Practices

### Memory Sizing

| Service | Minimum | Recommended | Heavy Load |
|---------|---------|-------------|------------|
| postgres | 256M | 1G | 4G+ |
| hasura | 256M | 512M | 1G+ |
| redis | 64M | 256M | 1G |
| nginx | 64M | 128M | 256M |
| functions | 128M | 256M | 512M |

### CPU Allocation

- **postgres**: 0.5-2 CPUs (I/O bound, moderate CPU)
- **hasura**: 0.5-1 CPUs (depends on query complexity)
- **redis**: 0.25-0.5 CPUs (very efficient)
- **nginx**: 0.25-1 CPUs (low unless high traffic)
- **functions**: 0.5-2 CPUs (depends on workload)

### Replica Guidelines

- **postgres**: 1 (use read replicas for scaling reads)
- **hasura**: 2-5 (stateless, scales horizontally)
- **nginx**: 1-3 (reverse proxy, low overhead)
- **redis**: 1 (use clustering for horizontal scaling)
- **functions**: 1-10 (scales based on request volume)

---

## Troubleshooting

### Service Not Starting After Scale

```bash
# Check service logs
docker logs <project>_<service>

# Verify resources are available
docker system df
free -h
```

### Out of Memory

```bash
# Increase memory limit
nself perf scale <service> --memory 2G

# Check host memory
free -h
docker stats
```

### CPU Throttling

```bash
# Increase CPU limit
nself perf scale <service> --cpu 2

# Check CPU stats
docker stats --no-stream
```

---

## Related Commands

- [perf](PERF.md) - Performance monitoring
- [bench](BENCH.md) - Benchmarking
- [health](HEALTH.md) - Health checks
- [doctor](DOCTOR.md) - Diagnostics
