# nself perf - Performance Profiling

> **Deprecated:** `nself perf` is a compatibility wrapper. Subcommands have moved:
>
> - `nself perf bench` → `nself service bench`
> - `nself perf scale` → `nself service scale`
> - `nself perf profile` → `nself service profile`
> - `nself perf optimize` → `nself service optimize`
> - `nself perf migrate` → `nself db migrate`
>
> This stub will be removed in v1.0.0.

**Version 0.4.6** | Performance analysis and optimization

---

## Overview

The `nself perf` command provides comprehensive performance profiling and analysis for your nself infrastructure. It includes system profiling, slow query detection, and optimization recommendations.

---

## Usage

```bash
nself perf <subcommand> [options]
```

---

## Subcommands

### `profile [service]`

Run full system profile or service-specific profiling.

```bash
nself perf profile              # Full system profile
nself perf profile postgres     # Profile PostgreSQL
nself perf profile hasura       # Profile Hasura GraphQL
```

### `analyze`

Analyze system performance with specific focus.

```bash
nself perf analyze                  # General analysis
nself perf analyze --slow-queries   # Focus on slow queries
nself perf analyze --memory         # Memory analysis
nself perf analyze --cpu            # CPU analysis
```

### `slow-queries`

Detailed analysis of slow database queries using pg_stat_statements.

```bash
nself perf slow-queries           # Show top slow queries
nself perf slow-queries --limit 20 # Top 20 queries
```

**Requires:** pg_stat_statements extension enabled in PostgreSQL.

### `report`

Generate a performance report.

```bash
nself perf report              # Table format
nself perf report --json       # JSON output
```

### `dashboard`

Real-time terminal dashboard showing performance metrics.

```bash
nself perf dashboard           # Launch dashboard
```

**Exit:** Press Ctrl+C to stop.

### `suggest`

Get optimization recommendations based on current performance.

```bash
nself perf suggest             # Show suggestions
nself perf suggest --json      # JSON output
```

---

## Options

| Option | Description |
|--------|-------------|
| `--json` | Output in JSON format |
| `--output FILE` | Save results to file |
| `-h, --help` | Show help message |

---

## Examples

```bash
# Quick system health check
nself perf profile

# Find slow queries
nself perf slow-queries

# Get optimization tips
nself perf suggest

# Export performance report
nself perf report --json > perf-report.json

# Monitor in real-time
nself perf dashboard
```

---

## Output Formats

### Table (default)

```
  ➞ Performance Profile

  SERVICE         CPU%    MEMORY       RESPONSE
  postgres        2.3%    512MB/1GB    5ms
  hasura          4.1%    256MB/512MB  12ms
  auth            1.0%    128MB/256MB  8ms
```

### JSON

```json
{
  "timestamp": "2026-01-23T10:30:00Z",
  "services": [
    {"name": "postgres", "cpu": "2.3%", "memory": "512MB", "response_ms": 5}
  ]
}
```

---

## Prerequisites

- PostgreSQL with `pg_stat_statements` extension for slow query analysis
- Running nself services (`nself start`)

---

## Related Commands

- [bench](BENCH.md) - Benchmark testing
- [scale](SCALE.md) - Service scaling
- [status](STATUS.md) - Service status

---

*Last Updated: January 24, 2026 | Version: 0.4.8*
