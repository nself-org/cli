# nself perf

**Category**: Performance Commands

Performance tuning, benchmarking, and optimization tools.

## Overview

All performance operations use `nself perf <subcommand>` for benchmarking, profiling, and optimizing nself applications.

**Features**:
- ✅ Performance benchmarking
- ✅ Database query optimization
- ✅ Service profiling
- ✅ Load testing
- ✅ Resource optimization

## Subcommands

| Subcommand | Description | Use Case |
|------------|-------------|----------|
| [bench](#nself-perf-bench) | Run performance benchmarks | Measure performance |
| [profile](#nself-perf-profile) | Profile services | Find bottlenecks |
| [optimize](#nself-perf-optimize) | Optimize configuration | Improve performance |
| [scale](#nself-perf-scale) | Scaling recommendations | Growth planning |
| [load-test](#nself-perf-load-test) | Load testing | Stress testing |

## nself perf bench

Run comprehensive performance benchmarks.

**Usage**:
```bash
nself perf bench [OPTIONS]
```

**Options**:
- `--duration N` - Benchmark duration (seconds)
- `--concurrency N` - Concurrent requests
- `--output FILE` - Save results to file

**Benchmarks**:
- Database queries (read/write)
- GraphQL API performance
- Auth service performance
- Cache hit rates
- File upload/download

**Examples**:
```bash
# Run benchmarks
nself perf bench

# Long benchmark
nself perf bench --duration 300

# High concurrency
nself perf bench --concurrency 100

# Save results
nself perf bench --output bench-$(date +%Y%m%d).json
```

**Output**:
```
╔═══════════════════════════════════════════════════════════╗
║          nself Performance Benchmark Results              ║
╚═══════════════════════════════════════════════════════════╝

Database Performance
──────────────────────────────────────────────────────────
Simple SELECT queries:      12,450 ops/sec
Complex JOIN queries:       3,210 ops/sec
INSERT operations:          8,930 ops/sec
UPDATE operations:          7,540 ops/sec

Average query time:         8ms
95th percentile:           15ms
99th percentile:           45ms

GraphQL API Performance
──────────────────────────────────────────────────────────
Simple queries:            5,680 req/sec
Complex queries:           1,240 req/sec
Mutations:                 2,890 req/sec
Subscriptions:             4,120 active

Average response time:     25ms
95th percentile:          62ms
99th percentile:         135ms

Cache Performance
──────────────────────────────────────────────────────────
Cache hit rate:            89.2%
Cache miss rate:           10.8%
Average hit latency:       0.8ms
Average miss latency:      18.3ms

Overall Score: 8.5/10 (Excellent)
```

## nself perf profile

Profile services to identify performance bottlenecks.

**Usage**:
```bash
nself perf profile <service> [OPTIONS]
```

**Options**:
- `--duration N` - Profile duration (seconds)
- `--format FORMAT` - Output format (flamegraph/json/text)
- `--output FILE` - Save profile

**Services**:
- postgres
- hasura
- auth
- custom services

**Examples**:
```bash
# Profile PostgreSQL
nself perf profile postgres --duration 60

# Generate flamegraph
nself perf profile hasura --format flamegraph --output hasura-profile.svg

# Profile custom service
nself perf profile api --duration 120
```

**Identifies**:
- Slow queries
- Hot code paths
- Memory leaks
- CPU bottlenecks
- I/O waits

## nself perf optimize

Optimize service configuration for performance.

**Usage**:
```bash
nself perf optimize [SERVICE] [OPTIONS]
```

**Options**:
- `--target TARGET` - Optimization target (latency/throughput/memory)
- `--apply` - Auto-apply optimizations
- `--dry-run` - Show recommendations only

**Optimizations**:

**PostgreSQL**:
- Connection pool sizing
- Query plan optimization
- Index recommendations
- Vacuum scheduling
- Cache sizing

**Hasura**:
- Query caching
- Connection pooling
- Batch size tuning

**Redis**:
- Memory eviction policies
- Persistence settings
- Key expiration

**Examples**:
```bash
# Get optimization recommendations
nself perf optimize

# Optimize for latency
nself perf optimize --target latency

# Optimize PostgreSQL
nself perf optimize postgres --apply

# Dry run
nself perf optimize --dry-run
```

**Output**:
```
Performance Optimization Recommendations

PostgreSQL
──────────────────────────────────────────────────────────
✓ Applied: Increased shared_buffers (256MB → 1GB)
✓ Applied: Increased effective_cache_size (1GB → 4GB)
✓ Applied: Enabled query plan caching
! Manual: Add index on users(email) - improves login by 65%
! Manual: Add index on posts(created_at) - improves timeline by 42%

Hasura
──────────────────────────────────────────────────────────
✓ Applied: Enabled query caching (TTL: 60s)
✓ Applied: Increased connection pool (10 → 20)
! Manual: Review and simplify complex queries

Redis
──────────────────────────────────────────────────────────
✓ Applied: Changed eviction policy to allkeys-lru
✓ Applied: Increased maxmemory (256MB → 512MB)

Expected Improvements:
  Database queries:      +35% faster
  GraphQL API:          +28% faster
  Cache hit rate:       +12%
  Overall latency:      -40%

Run 'nself restart' to apply changes
```

## nself perf scale

Get scaling recommendations based on usage patterns.

**Usage**:
```bash
nself perf scale [OPTIONS]
```

**Options**:
- `--target-users N` - Target user count
- `--target-rps N` - Target requests/second
- `--budget USD` - Monthly budget constraint

**Analyzes**:
- Current resource usage
- Growth trends
- Bottlenecks
- Cost per user

**Examples**:
```bash
# General recommendations
nself perf scale

# Scale to 10,000 users
nself perf scale --target-users 10000

# Scale to 1000 RPS
nself perf scale --target-rps 1000

# Budget-constrained
nself perf scale --target-users 10000 --budget 500
```

**Output**:
```
Scaling Recommendations

Current State
──────────────────────────────────────────────────────────
Users:              1,200
Requests/sec:       45
Database size:      12 GB
Storage used:       85 GB

Target State
──────────────────────────────────────────────────────────
Users:              10,000
Requests/sec:       375 (est.)
Database size:      100 GB (est.)
Storage used:       700 GB (est.)

Recommended Changes
──────────────────────────────────────────────────────────

Infrastructure:
  Database:  Upgrade to cx31 (2 vCPU, 8GB) → cx41 (4 vCPU, 16GB)
  App Server: Add 2 more cx21 instances (load balanced)
  Redis: Upgrade to 2GB memory
  Storage: Increase to 1TB

Cost Impact:
  Current:    €25/month
  Projected:  €95/month
  Per user:   €0.0095/month

Performance Impact:
  Response time: -30% (improved)
  Capacity: 8.3x current
  Headroom: 25% buffer

Timeline:
  1. Scale database (2 hours, no downtime)
  2. Add app servers (30 minutes)
  3. Configure load balancer (15 minutes)
  4. Test and monitor (1 hour)
```

## nself perf load-test

Run load tests to determine system limits.

**Usage**:
```bash
nself perf load-test [OPTIONS]
```

**Options**:
- `--users N` - Virtual users
- `--duration N` - Test duration (seconds)
- `--ramp-up N` - Ramp-up time (seconds)
- `--scenario FILE` - Load test scenario

**Test Types**:
- Constant load
- Ramp-up
- Spike test
- Stress test
- Endurance test

**Examples**:
```bash
# Basic load test
nself perf load-test --users 100 --duration 300

# Ramp-up test
nself perf load-test --users 1000 --duration 600 --ramp-up 120

# Spike test
nself perf load-test --scenario spike-test.yaml

# Stress test (find breaking point)
nself perf load-test --stress --max-users 10000
```

**Output**:
```
Load Test Results

Configuration
──────────────────────────────────────────────────────────
Virtual users:     100
Duration:          300s
Ramp-up:          30s

Results
──────────────────────────────────────────────────────────
Total requests:    45,230
Successful:        45,124 (99.8%)
Failed:            106 (0.2%)

Response Times:
  Average:         245ms
  Median:          189ms
  95th %ile:       520ms
  99th %ile:       1,240ms
  Max:             3,450ms

Throughput:
  Requests/sec:    150.8
  Transfer/sec:    2.3 MB

Resource Usage:
  CPU avg:         45%
  Memory avg:      62%
  Network:         12 Mbps

Breaking point:    ~180 concurrent users
Recommended max:   150 concurrent users (with 20% headroom)
```

## Performance Monitoring

### Real-Time Monitoring

```bash
# Monitor performance metrics
nself perf monitor
```

**Shows**:
- Request rate
- Response times
- Error rates
- Resource usage
- Active connections

### Performance Alerts

```bash
# Configure alerts
nself perf alerts set \
  --response-time-p95 500ms \
  --error-rate 1% \
  --cpu-usage 80%
```

## Best Practices

### 1. Regular Benchmarking

```bash
# Weekly benchmarks
0 0 * * 0 nself perf bench --output weekly-bench-$(date +%Y%m%d).json
```

### 2. Continuous Profiling

```bash
# Profile production periodically
nself perf profile postgres --duration 60
nself perf profile hasura --duration 60
```

### 3. Load Test Before Scaling

```bash
# Test current limits
nself perf load-test --stress

# Then scale
nself perf scale --target-rps 500
```

### 4. Monitor After Changes

```bash
# After optimization
nself perf optimize --apply
nself restart

# Monitor impact
nself perf monitor --duration 3600
nself perf bench --compare baseline.json
```

## Related Commands

- `nself monitor` - Real-time monitoring
- `nself status --resources` - Resource usage
- `nself db migrate` - Database optimization

## See Also

- [Performance Guide](../../guides/PERFORMANCE.md)
- [Optimization Techniques](../../guides/OPTIMIZATION.md)
- [Scaling Guide](../../guides/SCALING.md)
- [Load Testing](../../guides/LOAD-TESTING.md)
