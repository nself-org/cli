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
| `cold_start.status` | `nself status` | Same, in an empty directory (exits 1 on purpose) |
| `json_overhead.version`, `json_overhead.status` | `nself version --json`, `nself status --json` | The machine-readable path, for before and after comparisons of anything that wraps it |
| `time_to_healthy` | golden-path steps 3 to 6 | Sum of the per-step durations in the golden-path report (seconds); exits 1 unless all four steps are `pass` or `warn` |

Each probe states the exit code it must produce (0, or 1 for `status`). Any other code, a signal death, or a run longer than `-timeout` (default 30 s) fails `run` and the `ab` verdict, so a binary that crashes at startup never reports a fast time. The result `sha` is the checkout's HEAD only for a binary `perfbench` built itself; with `-bin` it is empty unless you pass `-sha`.

Every probe runs with a fresh empty `HOME`, an empty working directory, `NSELF_TELEMETRY_OPT_OUT=1`, stdin at `/dev/null` and output discarded. Percentiles are nearest-rank on the sorted samples, rounded to 0.1. The output is a `perfbench/v1` JSON document with a fixed key order.

`e2e-golden-path.yml` prints `time_to_healthy` after the smoke step on every run. It is informational and never gates; the golden path keeps its own per-step thresholds.

### Comparing two binaries

`perfbench ab` runs the probes of one scenario against two binaries, interleaved (base, head, base, head, ...) so machine drift hits both alike:

```bash
go run -mod=vendor ./tools/perfbench ab -base /path/to/base/nself -head /path/to/head/nself -runs 40 -json
go run -mod=vendor ./tools/perfbench ab -scenario json-overhead -base ... -head ...
```

It exits 1 when, for any probe, `head_p50 > 1.25 x base_p50` and `head_p50 - base_p50 > 3 ms` (both flags are adjustable), or when either binary dies, exits with the wrong code or times out. `go run` reports every non-zero exit of the program as 1; build `./tools/perfbench` first when you need to tell exit 1 (regression) from exit 2 (bad usage). `-inject-slowdown F` makes each head sample take F times its measured time; it exists only to prove the comparison can fire. The measured order alternates each round (base first, then head first), so a fixed order effect cannot favour a side. `perf.yml` runs `ab` on every PR (see Cold start gate below).

New scenarios are registered with `scenarios.Register` from a file in `tools/perfbench/scenarios/`; see the package comment.

### Checking a result against the budget

```bash
go run -mod=vendor ./tools/perfbench run -bin /path/to/nself -json > run.json
go run -mod=vendor ./tools/perfbench check -budget .github/perf-budget.json -in run.json
```

`check` reads a `perfbench-budget/v1` file (`p95_ms`: metric name to ceiling in ms) and a `perfbench/v1` result (an `ab` document also works; its run fields describe the head). It exits 1 and prints `<metric> p95 <v> ms > budget <b> ms` for every metric over its ceiling. It fails closed: exit 2 for an unreadable, wrong-schema or unknown-field budget or result, an empty budget, a result produced with `-inject-slowdown` (`injected: true`), and exit 1 for a budgeted metric missing from the result, a metric with fewer than `-min-n` samples (default 20, because nearest-rank p95 of fewer samples is just the maximum), a unit other than ms, or a p95 that is zero or not a number.

## Cold start gate

`perf.yml` job **Cold Start Gate** (ubuntu-latest) runs on every pull request that touches Go code, `go.mod`, `go.sum`, `vendor/` or the workflow, and on `workflow_dispatch`. It never uses `continue-on-error` and has no skip path: a failed base build, a crashed or hung probe, an unparseable result or too few samples fails the job.

1. **Build.** The base is `git merge-base origin/<base branch> <PR head>`, built in a detached `git worktree` beside the head checkout (the checkout is the PR head commit, not GitHub's synthetic merge commit, so the log line `Base build: <merge-base sha> head: <head sha>` can be re-checked locally with `git merge-base origin/main <head sha>`). The job fails if the two are the same commit. On a dispatch run the base is the head, so the run is an A/A comparison.
2. **A/B regression gate.** `perfbench ab` runs 40 interleaved rounds per probe after 3 warm-up rounds, for `nself version`, `nself --help` and `nself status`. A probe fails when `head_p50 > 1.25 x base_p50` **and** `head_p50 - base_p50 > 3 ms`. The ratio alone would fire on timer noise at a few milliseconds, and a large fixed delta (the old 20 ms floor) would let a 2x regression of an 11 ms probe through, so both conditions are required. The job summary and the `cold-start-gate-json` artifact (`perf-ab.json`, `perf-run.json`) carry the full table, so a flake names the probe and both p50s.
3. **Absolute ceiling.** `perfbench run` (30 runs per probe) then `perfbench check -budget .github/perf-budget.json`: p95 at or below the ceiling per probe. It catches slow drift that no single PR trips, and **has no override label**.

Why a relative gate: base and head run alternately in the same job on the same VM, so runner speed and neighbour noise hit both sides and cancel. A fixed budget cannot do that.

### Why these thresholds

The ratio, the minimum delta, 40 rounds and 3 warm-up rounds are decision G8 of the P7-GUARD epic. They are validated, not tuned: the self-tests below must show 20 of 20 A/A comparisons passing and an injected 2x slowdown failing on ubuntu-latest before the gate merges, and the measured A/A spread is recorded in `hq` (`sport/reference/performance-slos.md`). If a self-test ever fails, re-measure the variance and escalate; do not loosen a threshold quietly. The 150 ms ceiling is the declared cold-start SLO, far above the measured p95 on the same runner (15-20 ms p95 on ubuntu-latest, run 37269982940), so it only fires on a large drift.

### Self-test dispatches

Run from the Actions tab or `gh workflow run perf.yml --ref <branch> -f self_test=<name>`:

| `self_test` | Proves | Passes only when |
|---|---|---|
| `none` (default) | normal dispatch run, base = head | the A/B and ceiling steps pass |
| `aa20` | the gate does not fire on noise | 20 A/A comparisons of the head against itself all pass; the summary lists the spread of p50 ratio and delta |
| `inject` | the gate can fire | `perfbench ab -inject-slowdown 2` exits 1 with all three probes failed, and `check` refuses its result |
| `size` | the size regression check can fire (Binary Size Gate) | the head binary padded by 15% is reported as a regression against the real base build |

### Approved regressions

A PR labelled `perf-regression-approved` still runs and prints the A/B table, but the fail step is skipped (the same pattern as `size-grow-approved`). A probe that died, hung or exited with the wrong code is not covered by the label. The PR description must state the measured delta and the reason, and the merging Ticket records the new numbers as a baseline row in `performance-slos.md`.

### Updating a budget

A PR that raises a ceiling in `.github/perf-budget.json` states the measured regression (probe, old and new p95, runner) and the reason in its description, the same way a `size-grow-approved` PR does, and the merging Ticket records the new baseline row in `performance-slos.md`. Lowering a ceiling needs no justification. Do not raise a ceiling to make a red run green without a measurement.

## CI Pipeline

`.github/workflows/perf.yml` runs on pull requests that touch Go code, on pushes to main, weekly, and on demand. See the file for the current size caps and the benchmark list; they live there, not here.

- **Binary Size Gate** (macOS): builds the `darwin/arm64` binary and checks it against the hard caps in the workflow `env`. It then builds the real base (the merge-base with `origin/main`, in a git worktree) and fails a PR whose binary grew by more than the regression threshold without the `size-grow-approved` label. A failed base build fails the job, and the log prints `Base build: <sha> head: <sha>`.
- **Cold Start Gate** (ubuntu): described above.
- **CLI Go Benchmarks** (ubuntu): runs the Go benchmark suites and uploads the results as an artifact.

← [[Contributing]] | [[Commands]] | [[Home]] →
