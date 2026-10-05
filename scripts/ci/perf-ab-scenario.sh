#!/usr/bin/env bash
# Purpose: compare a perfbench scenario that `perfbench ab` cannot drive (it needs
#          a generated HOME, so it is not a fixed-probe scenario) between a base and
#          a head binary, with the EPIC P7-GUARD G8 rule: a metric regressed when
#          head p50 > 1.25 x base p50 AND head p50 - base p50 > 3 ms.
# Usage:   perf-ab-scenario.sh <scenario> <base-bin> <head-bin> <outdir>
# Inputs:  scenario: cold-start-plugins | cold-start-plugins-v15 (any scenario that
#          `perfbench run` knows); the two binaries; an output directory.
#          PERF_AB_RUNS (default 40), PERF_AB_RATIO (1.25), PERF_AB_MIN_DELTA_MS (3).
# Outputs: <outdir>/base.json and <outdir>/head.json (perfbench/v1 documents: per
#          metric the mean of the two passes' p50, the larger p95 and max, n summed),
#          the two raw passes of each side, and a verdict table on stdout.
#          Exit 0: no regression. Exit 1: a regression (or a metric missing on the
#          head side). Exit 2: usage error or a measurement failure.
# Constraints: bash 3.2 compatible; needs go (the harness is built once) and jq.
#          Base and head run interleaved in two passes (base,head then head,base) so
#          machine drift hits both alike. A probe that exits unexpectedly fails the
#          run (perfbench run exits non-zero), never a pass.
set -euo pipefail

if [ "$#" -ne 4 ]; then
  echo "usage: perf-ab-scenario.sh <scenario> <base-bin> <head-bin> <outdir>" >&2
  exit 2
fi
SC="$1"; BASE="$2"; HEAD="$3"; OUT="$4"
RUNS="${PERF_AB_RUNS:-40}"
RATIO="${PERF_AB_RATIO:-1.25}"
DELTA="${PERF_AB_MIN_DELTA_MS:-3}"
for b in "$BASE" "$HEAD"; do
  [ -x "$b" ] || { echo "perf-ab-scenario: $b is not an executable" >&2; exit 2; }
done
command -v jq >/dev/null 2>&1 || { echo "perf-ab-scenario: jq is required" >&2; exit 2; }
cd "$(git rev-parse --show-toplevel)"
mkdir -p "$OUT"

PB="$OUT/perfbench"
CGO_ENABLED=0 go build -mod=vendor -o "$PB" ./tools/perfbench || exit 2

run() { # <bin> <out.json>
  "$PB" run -scenario "$SC" -bin "$1" -runs "$RUNS" -json > "$2" || {
    echo "perf-ab-scenario: perfbench run failed for $1 (scenario $SC)" >&2
    exit 2
  }
}
run "$BASE" "$OUT/base.pass1.json"
run "$HEAD" "$OUT/head.pass1.json"
run "$HEAD" "$OUT/head.pass2.json"
run "$BASE" "$OUT/base.pass2.json"

merge() { # <side>
  jq -s '
    .[0] as $a | .[1] as $b
    | $a
    | .runs = ($a.runs + $b.runs)
    | .metrics = [ $a.metrics[] as $x
        | ($b.metrics[] | select(.name == $x.name)) as $y
        | { name: $x.name, unit: $x.unit,
            p50: ((($x.p50 + $y.p50) / 2 * 10 | round) / 10),
            p95: ([$x.p95, $y.p95] | max), max: ([$x.max, $y.max] | max), n: ($x.n + $y.n) } ]' \
    "$OUT/$1.pass1.json" "$OUT/$1.pass2.json" > "$OUT/$1.json"
}
merge base
merge head

FAILED="$(jq -n --slurpfile b "$OUT/base.json" --slurpfile h "$OUT/head.json" \
  --argjson ratio "$RATIO" --argjson delta "$DELTA" '
  [ $b[0].metrics[] as $x
    | ([ $h[0].metrics[] | select(.name == $x.name) ][0]) as $y
    | select($y == null or ($y.p50 > $ratio * $x.p50 and ($y.p50 - $x.p50) > $delta))
    | $x.name ] | .[]' -r)"

jq -rn --slurpfile b "$OUT/base.json" --slurpfile h "$OUT/head.json" '
  $b[0].metrics[] as $x | ([ $h[0].metrics[] | select(.name == $x.name) ][0]) as $y
  | "\($x.name)\tbase p50 \($x.p50) ms\thead p50 \($y.p50 // "missing") ms"' | column -t -s "$(printf '\t')" || true

if [ -n "$FAILED" ]; then
  echo "perf-ab-scenario: $SC regressed (head p50 > ${RATIO} x base and > ${DELTA} ms): $(echo "$FAILED" | tr '\n' ' ')" >&2
  exit 1
fi
echo "perf-ab-scenario: $SC ok"
