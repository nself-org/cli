# Benchmark methodology

## What is measured

`time_to_healthy`, in seconds: the sum of the durations of golden-path steps 3
to 6, taken from the report `scripts/golden-path.sh` writes to
`/tmp/golden-path-report.json`.

| Step | Name in the report | What it does |
|---|---|---|
| 3 | init | `nself init` in a fresh project directory |
| 4 | build | `nself build` |
| 5 | start | `nself start` |
| 6 | health | waits until the stack answers its health checks |

Each step's duration is the whole-second wall-clock figure the golden path
recorded when it ran the step. The harness adds the four numbers and re-times
nothing. A step that did not pass (or warned past its budget and still passed)
is judged by the golden path itself: `pass` and `warn` count, anything else makes
the time not measurable and the harness exits non-zero.

## Environment

The scheduled run is a GitHub-hosted `ubuntu-latest` runner with its Docker
daemon, building the checked-out ref (`GOLDEN_PATH_SOURCE=local`) with
`AI_AUTO_INSTALL=false`, so no AI model download is timed. Numbers from a shared
runner vary between runs; one quarterly run is one sample (`n` is 1 in the
output), not a distribution.

## Output

A `perfbench/v1` JSON document (see `tools/perfbench`) holding the single metric
`time_to_healthy` with unit `s`, uploaded as the `benchmark-results-<run id>`
workflow artifact together with the golden-path report it was read from.

## What is not measured

Requests per second, setup time of a competitor, cost per request and feature
coverage are not measured by this harness and are not reported. A competitor
comparison needs a measured, repeatable method for each competitor first; until
one exists, none is published.
