# nSelf benchmark harness

Measures one thing: **time to healthy**, the seconds a fresh `nself init` and
`nself start` take to reach a healthy stack. The number comes from a real run of
the 13-step golden path (`scripts/golden-path.sh`), read by `tools/perfbench`.
Nothing is estimated, typed in by hand or scored by hand, and no competitor is
measured.

## What it produces

`run.sh <golden-path-report.json> <out.json>` writes a `perfbench/v1` document to
`<out.json>`. Its one metric is `time_to_healthy` (unit `s`): the sum of the
report's step 3 (init) through step 6 (health wait) durations. It fails, and
writes nothing, when the report is missing or any of those four steps did not
pass.

## Running it

```bash
# 1. run the golden path on this machine (needs Docker and an owner licence key)
GOLDEN_PATH_SOURCE=local bash scripts/golden-path.sh   # writes /tmp/golden-path-report.json

# 2. read the time to healthy out of its report
.github/benchmarks/run.sh /tmp/golden-path-report.json /tmp/time-to-healthy.json
```

## Quarterly run

`.github/workflows/benchmarks.yml` runs on the first Monday of March, June,
September and December, and on manual dispatch. It builds the checked-out ref
(`GOLDEN_PATH_SOURCE=local`), runs the golden path on `ubuntu-latest`, then runs
`run.sh` even when a later golden-path step failed, so a healthy-stack time is
still recorded. The result and the golden-path report are uploaded as the
`benchmark-results-<run id>` workflow artifact and kept 14 days. The workflow is
read-only: it never changes the repository, so nothing is published from it.

See [METHODOLOGY.md](METHODOLOGY.md) for exactly what is timed.
