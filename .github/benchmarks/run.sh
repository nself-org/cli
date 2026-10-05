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
# The document's "sha" is the measured commit: GITHUB_SHA in CI, else the git HEAD
# of this checkout (plus -dirty when tracked files are modified). perfbench's
# healthy subcommand leaves it empty, so run.sh fills it in; with no commit to
# name, run.sh fails rather than publish an unattributed number.
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
trap 'rm -f "$tmp" "$tmp.sha"' EXIT

sha="${GITHUB_SHA:-}"
if [ -z "$sha" ]; then
  sha="$(git -C "$root" rev-parse HEAD)"
  if [ -n "$(git -C "$root" status --porcelain --untracked-files=no)" ]; then
    sha="${sha}-dirty"
  fi
fi

(cd "$root" && go run -mod=vendor ./tools/perfbench healthy -report "$report" -json) > "$tmp"
# Set "sha" in place; key order and 2-space indent are kept.
BENCH_SHA="$sha" python3 -c '
import json, os, sys
doc = json.load(sys.stdin)
doc["sha"] = os.environ["BENCH_SHA"]
sys.stdout.write(json.dumps(doc, indent=2) + "\n")
' < "$tmp" > "$tmp.sha"
mv "$tmp.sha" "$out"
rm -f "$tmp"
cat "$out"
