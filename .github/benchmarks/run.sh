#!/usr/bin/env bash
# run.sh: time-to-healthy from a real golden-path run (P7-TRUTH-22, EPIC TRUTH D13).
#
# Usage: .github/benchmarks/run.sh <golden-path-report.json> <out.json>
#
# Reads the report scripts/golden-path.sh wrote and runs
# `perfbench healthy -report <report> -json` (tools/perfbench, P7-GUARD-07)
# into <out.json>: a perfbench/v1 document whose only metric is time_to_healthy
# (seconds, the sum of golden-path steps 3-6). Nothing else is measured or
# written here; no number is typed or estimated.
#
# Exit codes: 0 measured; 1 the report is missing or time_to_healthy is not
# measurable (steps 3-6 did not all pass); 2 bad usage. On any failure <out.json>
# is left untouched.
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: run.sh <golden-path-report.json> <out.json>" >&2
  exit 2
fi
report=$1
out=$2

if [ ! -f "$report" ]; then
  echo "run.sh: golden-path report not found: $report" >&2
  exit 1
fi

# perfbench runs from the repo root, so resolve both paths first.
report="$(cd "$(dirname "$report")" && pwd)/$(basename "$report")"
mkdir -p "$(dirname "$out")"
out="$(cd "$(dirname "$out")" && pwd)/$(basename "$out")"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

tmp="$(mktemp "${out}.XXXXXX")"
trap 'rm -f "$tmp"' EXIT

(cd "$root" && go run -mod=vendor ./tools/perfbench healthy -report "$report" -json) > "$tmp"
mv "$tmp" "$out"
cat "$out"
