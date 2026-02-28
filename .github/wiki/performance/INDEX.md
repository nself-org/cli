# Performance & Optimization

Performance optimization guides and benchmarks for nself.

---

## Overview

This section covers performance optimization strategies, benchmarking tools, and best practices for ensuring your nself deployment runs efficiently at scale.

---

## Documentation

| Document | Description |
|----------|-------------|
| **[Performance Optimization](PERFORMANCE-OPTIMIZATION-V0.9.8.md)** | Complete performance tuning guide for v0.9.8 |
| **[Performance Summary](PERFORMANCE-SUMMARY.md)** | Quick reference for common optimizations |
| **[Benchmarks](../development/PERFORMANCE-BENCHMARKS.md)** | Performance benchmarks and metrics |

---

## Quick Tips

### Database Performance

```bash
# Enable connection pooling
POSTGRES_MAX_CONNECTIONS=200

# Optimize queries
nself db analyze
```

### Service Optimization

```bash
# Adjust parallel limits
NSELF_PARALLEL_LIMIT=10

# Enable build caching
NSELF_BUILD_CACHE=true
```

### Monitoring

```bash
# Enable monitoring bundle
MONITORING_ENABLED=true

# View metrics
nself monitor
```

---

## Related Documentation

- **[Monitoring Guide](../guides/MONITORING-COMPLETE.md)** - Observability setup
- **[Database Guide](../guides/DATABASE-WORKFLOW.md)** - Database optimization
- **[Deployment Guide](../deployment/README.md)** - Production tuning

---

**[← Back to Documentation](../README.md)**
