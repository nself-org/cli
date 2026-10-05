# CLI Benchmarks

The ɳSelf CLI ships with Go benchmark tests for the top-5 commands. These benchmarks measure cobra dispatch overhead, flag-parsing cost, and early business-logic traversal, not external I/O (Docker, network, filesystem beyond a temp dir).

## Running Benchmarks

```bash
# Run all benchmarks once (quick check)
go test -mod=vendor -bench=. -benchtime=1x -benchmem ./cmd/commands/

# Run a specific benchmark (3s per b.N, 3 runs)
go test -mod=vendor -bench=BenchmarkVersion -benchtime=3s -count=3 -benchmem ./cmd/commands/

# Run all 5 primary benchmarks
go test -mod=vendor \
  -bench='BenchmarkVersion$|BenchmarkInit$|BenchmarkBuild$|BenchmarkPluginInstallCached$|BenchmarkDoctor$' \
  -benchtime=3s -count=3 -benchmem \
  ./cmd/commands/
```

## Benchmark Files

| File | Benchmarks | What it measures |
|------|-----------|-----------------|
| `version_bench_test.go` | `BenchmarkVersion`, `BenchmarkVersionJSON`, `BenchmarkVersionHelp` | Cobra startup + version string format |
| `init_bench_test.go` | `BenchmarkInit`, `BenchmarkInitFlagParsing` | Flag parse + early input sanitisation for `nself init` |
| `build_bench_test.go` | `BenchmarkBuild`, `BenchmarkBuildCheck`, `BenchmarkBuildFlagParsing` | Flag parse + FindNSelfRoot for `nself build` |
| `plugin_install_bench_test.go` | `BenchmarkPluginInstallCached`, `BenchmarkPluginList`, `BenchmarkPluginFlagParsing` | Plugin dispatch + gate overhead |
| `doctor_bench_test.go` | `BenchmarkDoctor`, `BenchmarkDoctorDeep`, `BenchmarkDoctorFlagParsing` | Doctor command dispatch (stub RunE, no Docker required) |

## Emitting Results for CI

Set `BENCH_RESULTS_FILE=/path/to/output.ndjson` before running benchmarks to emit
NDJSON result entries. The CI pipeline (`perf.yml`) uses this to feed `perf-compare.sh`.

```bash
BENCH_RESULTS_FILE=/tmp/bench.ndjson \
  go test -mod=vendor -bench=. -benchtime=3s ./cmd/commands/
```

## SLO Targets

Per F16-PERF-SLOS.md:

| Command | p95 SLO |
|---------|---------|
| `nself version` | ≤ 150ms |
| `nself build` | ≤ 5s |
| `nself doctor` | ≤ 8s |

These benchmarks measure pre-check overhead only. Full command execution (including Docker and network) is measured by the integration test suite.

## Cold start and time to healthy

The benchmarks above run inside one process and measure dispatch. Startup cost a user feels is the wall time of a fresh process, and the cost of getting a stack healthy is a stack-level number. `tools/perfbench` (stdlib only, no `internal/` imports, so it builds and measures any sha) measures both.

```bash
# Cold start: builds ./cmd/nself once, then 3 warm-up and 30 measured runs per probe
go run -mod=vendor ./tools/perfbench run -json

# Measure an existing binary (for example a tagged release you built earlier)
go run -mod=vendor ./tools/perfbench run -bin /path/to/nself -runs 30 -json

# Time to healthy: read from a golden-path report, never re-timed
go run -mod=vendor ./tools/perfbench healthy -report /tmp/golden-path-report.json -json

# JSON invocation overhead (the --json path of version and status)
go run -mod=vendor ./tools/perfbench run -scenario json-overhead -json
```

| Metric | Probe | What it measures |
|--------|-------|------------------|
| `cold_start.version` | `nself version` | Process start to exit of the version subcommand |
| `cold_start.help` | `nself --help` | Process start to exit of the root help |
| `cold_start.status` | `nself status` | Same, in an empty directory (exits 1 on purpose; the exit code is recorded and must not change between runs) |
| `json_overhead.version`, `json_overhead.status` | `nself version --json`, `nself status --json` | The machine-readable path, for before and after comparisons of anything that wraps it |
| `time_to_healthy` | golden-path steps 3 to 6 | Sum of the per-step durations in the golden-path report (seconds); exits 1 unless all four steps are `pass` or `warn` |

Every probe runs with a fresh empty `HOME`, an empty working directory, `NSELF_TELEMETRY_OPT_OUT=1`, stdin at `/dev/null` and output discarded. Percentiles are nearest-rank on the sorted samples, rounded to 0.1. The output is a `perfbench/v1` JSON document with a fixed key order.

`e2e-golden-path.yml` prints `time_to_healthy` after the smoke step on every run. It is informational and never gates; the golden path keeps its own per-step thresholds.

### Comparing two binaries

`perfbench ab` runs the probes of one scenario against two binaries, interleaved (base, head, base, head, ...) so machine drift hits both alike:

```bash
go run -mod=vendor ./tools/perfbench ab -base /path/to/base/nself -head /path/to/head/nself -runs 40 -json
go run -mod=vendor ./tools/perfbench ab -scenario json-overhead -base ... -head ...
```

It exits 1 when, for any probe, `head_p50 > 1.25 x base_p50` and `head_p50 - base_p50 > 3 ms` (both flags are adjustable). `-inject-slowdown F` makes each head sample take F times its measured time; it exists only to prove the comparison can fire. P7-GUARD-08 wires `ab` into `perf.yml` and adds `check` and the budget file.

New scenarios are registered with `scenarios.Register` from a file in `tools/perfbench/scenarios/`; see the package comment.

## CI Pipeline

`cli/.github/workflows/perf.yml` runs on every PR and push to main:
- Builds the `darwin/arm64` binary and checks against hard limits: ≤35 MB uncompressed, ≤6 MB compressed.
- Runs the 5 benchmark suites and uploads results as artifacts.
- A >10% binary size growth without the `size-grow-approved` PR label fails the gate.

← [[Contributing]] | [[Commands]] | [[Home]] →
