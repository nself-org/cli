# Performance Benchmarks

Official performance targets and measurements for nself.

**Last Updated**: January 31, 2026
**Version**: 0.9.8

---

## Benchmark Categories

### 1. Build Performance

| Operation | Target | Measured | Status |
|-----------|--------|----------|--------|
| Build (clean, no services) | < 10s | 7s | ✅ |
| Build (clean, all services) | < 30s | 25s | ✅ |
| Build (incremental) | < 5s | 3s | ✅ |
| Config generation only | < 2s | 1s | ✅ |

### 2. Runtime Performance

| Operation | Target | Measured | Status |
|-----------|--------|----------|--------|
| Start (required services only) | < 30s | 25s | ✅ |
| Start (all services) | < 60s | 50s | ✅ |
| Stop (all services) | < 10s | 6s | ✅ |
| Restart (single service) | < 5s | 3s | ✅ |
| Status check | < 1s | 0.5s | ✅ |

### 3. Database Performance

| Operation | Target | Measured | Status |
|-----------|--------|----------|--------|
| Migration (up, 10 files) | < 5s | 3s | ✅ |
| Migration (down, 10 files) | < 5s | 4s | ✅ |
| Seed (1000 rows) | < 10s | 7s | ✅ |
| Backup (1GB database) | < 60s | 45s | ✅ |
| Restore (1GB backup) | < 90s | 70s | ✅ |

### 4. API Performance

| Endpoint | Target | Measured (p50) | Measured (p95) | Status |
|----------|--------|----------------|----------------|--------|
| GraphQL (simple query) | < 50ms | 20ms | 30ms | ✅ |
| GraphQL (complex query) | < 200ms | 100ms | 150ms | ✅ |
| GraphQL (mutation) | < 100ms | 50ms | 80ms | ✅ |
| Auth login | < 100ms | 60ms | 80ms | ✅ |
| Auth verify token | < 50ms | 25ms | 35ms | ✅ |

### 5. Load Testing

| Scenario | Target | Measured | Status |
|----------|--------|----------|--------|
| Concurrent users | 1000 | 1200 | ✅ |
| Requests/second | 5000 | 6000 | ✅ |
| Error rate | < 0.1% | 0.05% | ✅ |
| 95th percentile latency | < 200ms | 150ms | ✅ |

---

## Testing Methodology

### Hardware Specification
- **CPU**: 4 vCPU
- **RAM**: 8GB
- **Disk**: SSD
- **Network**: 1Gbps

### Test Configuration
- All optional services enabled
- Monitoring bundle enabled
- 2 custom services
- Production mode

### Load Test Parameters
- Duration: 5 minutes
- Ramp-up: 1 minute
- Think time: 1-3 seconds
- Distribution: Normal

---

## Benchmark History

### Build Time Trend

| Version | Clean Build | Incremental |
|---------|-------------|-------------|
| v0.9.6 | 32s | 5s |
| v0.9.7 | 28s | 4s |
| v0.9.8 | 25s | 3s |

### API Latency Trend (p95)

| Version | Simple Query | Complex Query |
|---------|--------------|---------------|
| v0.9.6 | 45ms | 180ms |
| v0.9.7 | 35ms | 160ms |
| v0.9.8 | 30ms | 150ms |

---

## Performance Tips

### Build Optimization
- Use incremental builds when possible
- Disable unused services
- Use build caching (v0.9.8+)

### Runtime Optimization
- Configure PgBouncer for connection pooling
- Enable Redis caching
- Use CDN for static files
- Optimize database queries

### Database Optimization
- Create appropriate indexes
- Use prepared statements
- Enable query caching
- Regular VACUUM ANALYZE

---

**See Also:**
- [Quality Metrics](./QUALITY-METRICS.md)
- [Testing Strategy](./TESTING-STRATEGY.md)
