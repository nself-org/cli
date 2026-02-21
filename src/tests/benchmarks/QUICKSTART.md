# Performance Benchmarking Quick Start

Get started with nself performance benchmarks in under 5 minutes.

## TL;DR

```bash
cd benchmarks

# Run all benchmarks (medium scale)
./run-all-benchmarks.sh medium

# Or run individual benchmarks
./billing-benchmarks.sh medium
./whitelabel-benchmarks.sh 100
./tenant-benchmarks.sh 100
./realtime-benchmarks.sh 1000
```

## Quick Start Guide

### 1. Run Your First Benchmark (30 seconds)

```bash
cd /path/to/nself/benchmarks
./billing-benchmarks.sh small
```

You'll see output like:
```
✓ Usage Tracking: 24,261 ops/sec (baseline: 1,000)
✓ Quota Checks: 30,577 ops/sec (baseline: 5,000)
✓ Invoice Generation: 5,724 ops/sec (baseline: 100)
```

### 2. Run All Benchmarks (2-5 minutes)

```bash
./run-all-benchmarks.sh medium
```

This runs:
- ✅ Billing system performance
- ✅ White-label performance
- ✅ Multi-tenant performance
- ✅ Real-time messaging performance

### 3. View Results

Results are saved to `benchmarks/results/`:

```bash
# View summary
cat results/billing-benchmark-summary.txt

# View CSV for analysis
open results/billing-benchmark-YYYYMMDD-HHMMSS.json.csv

# View JSON for automation
cat results/billing-benchmark-YYYYMMDD-HHMMSS.json
```

## Common Use Cases

### Before Production Deploy

```bash
# Run large-scale benchmarks to validate performance
./run-all-benchmarks.sh large

# Check for any failures or warnings
cat results/all-benchmarks-*.txt | grep -E "WARN|FAIL"
```

### Performance Regression Testing

```bash
# Run benchmarks before changes
./billing-benchmarks.sh medium
mv results/billing-benchmark-summary.txt baseline.txt

# Make your code changes...

# Run benchmarks again
./billing-benchmarks.sh medium

# Compare results
diff baseline.txt results/billing-benchmark-summary.txt
```

### Continuous Integration

Add to your CI pipeline:

```yaml
# .github/workflows/performance.yml
- name: Run Performance Benchmarks
  run: |
    cd benchmarks
    ./run-all-benchmarks.sh medium
    # Fail if any test shows performance degradation
    if grep -q "FAIL" results/*.csv; then
      exit 1
    fi
```

## Understanding Results

### Status Indicators

| Status | Meaning | Throughput | Latency |
|--------|---------|------------|---------|
| ✓ PASS | Great! | ≥100% of baseline | ≤100% of baseline |
| ⚠ WARN | Acceptable | 80-100% of baseline | 100-120% of baseline |
| ✗ FAIL | Needs attention | <80% of baseline | >120% of baseline |

### Performance Baselines

#### Billing System (medium scale)
- Usage Tracking: 5,000 ops/sec
- Quota Checks: 10,000 ops/sec
- Invoice Generation: 500 ops/sec

#### Multi-Tenant System (100 tenants)
- Isolated Queries: 3,000 qps
- RLS Prevention: 5,000 qps
- Tenant Switching: 7,000 switches/sec

#### Real-Time System (1000 connections)
- Connection Rate: 50,000 conn/sec
- Message Latency: <10ms
- Presence Updates: 5,000 updates/sec

## Troubleshooting

### Benchmark Fails to Run

**Error**: `Permission denied`
```bash
chmod +x benchmarks/*.sh
```

**Error**: `bc: command not found`
```bash
# macOS
brew install bc

# Ubuntu/Debian
sudo apt-get install bc
```

### Inconsistent Results

Run multiple times and average:
```bash
for i in {1..3}; do
  ./billing-benchmarks.sh medium
  sleep 30
done
```

### Results Too Slow

1. Check system resources:
   ```bash
   top
   iostat -x 1
   ```

2. Stop other services:
   ```bash
   docker stop $(docker ps -q)
   ```

3. Clear caches:
   ```bash
   sync; echo 3 | sudo tee /proc/sys/vm/drop_caches  # Linux
   ```

## Next Steps

- 📖 Read the full [README.md](README.md) for detailed documentation
- 🔧 Review optimization suggestions in benchmark output
- 📊 Set up performance monitoring with Prometheus/Grafana
- 🚀 Run benchmarks regularly to catch regressions early

## Help & Support

```bash
# Show help for any benchmark
./billing-benchmarks.sh --help
./whitelabel-benchmarks.sh --help
./tenant-benchmarks.sh --help
./realtime-benchmarks.sh --help
./run-all-benchmarks.sh --help
```

For issues: https://github.com/nself-org/cli/issues

---

**Ready to optimize?** Start with `./run-all-benchmarks.sh medium` and go from there! 🚀
