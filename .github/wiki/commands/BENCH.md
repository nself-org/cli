# nself perf bench - Benchmarking

> **DEPRECATED COMMAND NAME**: This command was formerly `nself bench` in v0.x. It has been consolidated to `nself perf bench` in v1.0. The old command name may still work as an alias.

**Version 0.4.6** | Performance benchmarking and load testing

---

## Overview

The `nself perf bench` command provides performance benchmarking and load testing capabilities. Run benchmarks against your API, database, and other services to measure performance and establish baselines.

---

## Usage

```bash
nself perf bench <subcommand> [options]
```

---

## Subcommands

### `run [target]`

Run benchmark against a target.

```bash
nself perf bench run api             # Benchmark GraphQL API
nself perf bench run auth            # Benchmark auth service
nself perf bench run db              # Benchmark database
nself perf bench run functions       # Benchmark serverless functions
nself perf bench run custom <url>    # Custom endpoint
```

### `baseline`

Establish a performance baseline for future comparisons.

```bash
nself perf bench baseline           # Create baseline
```

The baseline is saved to `.nself/benchmarks/baseline_latest.json`.

### `compare [file]`

Compare current performance against a baseline.

```bash
nself perf bench compare                    # Compare to latest baseline
nself perf bench compare baseline.json      # Compare to specific file
```

### `stress [target]`

Run stress test with high load.

```bash
nself perf bench stress api              # Stress test API
nself perf bench stress api --duration 120  # 2 minute stress test
```

**Warning:** Stress tests can impact system performance.

### `report`

Generate benchmark report.

```bash
nself perf bench report              # Show recent benchmarks
nself perf bench report --json       # JSON output
```

---

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `--requests N` | Number of requests | 1000 |
| `--concurrency N` | Concurrent connections | 10 |
| `--duration N` | Test duration in seconds | 30 |
| `--rate N` | Requests per second limit | unlimited |
| `--warmup N` | Warmup period in seconds | 5 |
| `--output FILE` | Save results to file | - |
| `--force` | Skip confirmation for stress tests | false |
| `--json` | Output in JSON format | false |
| `-h, --help` | Show help message | - |

---

## Targets

| Target | Description |
|--------|-------------|
| `api` | Hasura GraphQL API |
| `auth` | Authentication service |
| `db` | PostgreSQL database |
| `functions` | Serverless functions |
| `custom <url>` | Custom HTTP endpoint |

---

## Examples

```bash
# Quick API benchmark
nself perf bench run api

# Thorough benchmark with more requests
nself perf bench run api --requests 5000 --concurrency 50

# Establish baseline
nself perf bench baseline

# Compare against baseline
nself perf bench compare

# Stress test for 60 seconds
nself perf bench stress api --duration 60 --force

# Export benchmark report
nself perf bench report --json > benchmarks.json
```

---

## Benchmark Tools

nself perf bench uses available tools in order of preference:

1. **wrk** - High-performance HTTP benchmarking tool
2. **hey** - HTTP load generator
3. **ab** (Apache Bench) - Classic HTTP server benchmarking
4. **curl** (fallback) - Basic HTTP testing

Install recommended tools:
```bash
# macOS
brew install wrk

# Ubuntu/Debian
apt-get install apache2-utils
```

---

## Output Example

```
  ➞ Running curl benchmark
  URL: https://api.local.nself.org/v1/graphql
  Requests: 1000

  ➞ Results

  Total requests:      1000
  Successful:          998
  Failed:              2
  Total time:          45 seconds
  Requests/sec:        22 req/sec
  Avg response:        45 ms
  Min response:        12 ms
  Max response:        234 ms
```

---

## Baseline Comparison

When comparing against a baseline:

- **Green (+10%)** - Performance improved
- **Red (-10%)** - Performance degraded
- **No color** - Within normal range

---

## Related Commands

- [perf](PERF.md) - Performance profiling
- [scale](SCALE.md) - Service scaling
- [status](STATUS.md) - Service status

---

*Last Updated: January 24, 2026 | Version: 0.4.8*
