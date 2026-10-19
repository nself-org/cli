# Registry Performance Targets

**Last updated:** 2026-09-11  
**Status:** Active (enforced nightly via `.github/workflows/nightly-registry-perf.yml`)

## SLO Targets

| Operation | Target | Threshold (20% buffer) | Measured at | Notes |
|-----------|--------|------------------------|------------|-------|
| `nself plugin search` p95 latency | <100ms | <120ms | warm on-disk registry cache | Cache read + JSON parse + query match + JSON marshal -- code path only, no network. Measured as the job's *second* `plugin search` call so `internal/plugin/registry_cache.go`'s 5-minute TTL cache is already warm (see the `network` step below). |
| Registry network fetch (cold cache) | informational | WARN >10s | first `plugin search` call in the job | DNS + TLS + HTTP GET to `plugins.nself.org` (Cloudflare Worker). Reported as `network_ms`, never gates the search p95 above -- three nightly runs (2026-09-07/10/11) failed at 703-1124ms before this was split out, all of it third-party network latency, none of it search code. |
| `nself plugin install <name>` cold cache | <30s | <36s | Single plugin metadata fetch | No cache hit; includes download start |
| `nself plugin install --bundle nsentry` | <60s | <72s | large bundle install | ɳSentry bundle (core + expansion plugins) |
| Registry JSON parse + index build | <50ms | <60ms | In-process go test benchmark | Hasura introspection equivalent |

## Baseline Tracking

- Current baseline: `.github/registry-perf-baseline.json` (auto-updated after each nightly run)
- Regression threshold: >20% slower than target
- Nightly run: 2 AM UTC daily (schedule trigger `0 2 * * *`)
- Manual trigger: `gh workflow run nightly-registry-perf.yml`

## Critical Path Operations

All registry ops are blocking on user install experience:
- Blocking `nself plugin search` — user exploring plugins
- Blocking `nself plugin install` — user acquiring capability
- Blocking `nself build` startup — parsing registry on every build

## Optimization Guidelines

### If search_ms breaches <120ms
1. Index plugins in memory cache (in-process LRU, 10MB max)
2. Implement trie for prefix matching (O(k) not O(n))
3. Profile marshaling cost via pprof (consider msgpack or protobuf)
4. Reduce JSON payload (cache non-changing fields)

### If single_ms breaches <36s
1. Parallel plugin metadata fetch (currently sequential)
2. HTTP/2 multiplexing via stdlib (Go 1.20+)
3. CDN or regional mirror if origin <300ms latency
4. Defer signature verification to install-time (not fetch-time)

### If bundle_ms breaches <72s
1. Batch registry queries (N plugins in 1 round-trip)
2. Implement plugin dependency pre-fetch (fetch all deps in parallel)
3. Reduce bundle size (split the ɳSentry bundle into tiers)
4. Cache bundle manifest separately

### If parse_ms breaches <60ms
1. Benchmark against registry size (see the generated plugin count)
2. Switch to streaming parser if the registry grows by an order of magnitude
3. Profile allocations via `go test -bench -benchmem`
4. Precompile schema validation (jsonschema → code gen)

## Reference: Plugin Count by Bundle

Plugin counts per bundle are generated (`nself plugin count`, the plugins catalogue); this page does not carry hand counts.

## Regression Response Protocol

**If nightly workflow fails:**

1. Check baseline drift (recent plugin additions?)
2. Run locally: `go test -bench=BenchmarkRegistryParse ./internal/plugin`
3. Profile: `go tool pprof -http=:8080 cpu.prof`
4. Isolate: single plugin? bundle? parse overhead?
5. File optimization ticket with pprof evidence
6. Temporary threshold bump OK only if cause is validated

**Never:**
- Disable the workflow
- Increase thresholds without root-cause fix
- Merge perf regression PR without mitigation plan
