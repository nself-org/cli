#!/usr/bin/env bash
# test-harness.sh: tests for the benchmark harness guarantees (P7-TRUTH-22).
#   1. an unparseable step duration fails the run and is never written as 0
#   2. a stale golden-path report is deleted before a run can exit early
#   3. the artifact's sha is the measured commit, never empty
# Usage: .github/benchmarks/test-harness.sh   (from anywhere; needs go, python3, git)
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/bench-harness-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT
fails=0
check() { # check <name> <command...>
  local name=$1; shift
  if "$@"; then echo "PASS $name"; else echo "FAIL $name"; fails=$((fails + 1)); fi
}

# --- 1. unparseable duration -------------------------------------------------
# Run the report writer exactly as golden-path.sh embeds it.
sed -n "/python3 - <<'PYEOF'/,/^PYEOF$/p" "$root/scripts/golden-path.sh" | sed '1d;$d' > "$work/writer.py"
printf '3\tpass\t2\tinit\n4\tpass\tabc\tbuild\n' > "$work/steps.tsv"
unparseable_fails() {
  ! STEPS_DATA="$work/steps.tsv" REPORT_FILE="$work/report.json" OVERALL=pass PASS=2 FAIL=0 WARN_COUNT=0 \
    python3 "$work/writer.py" 2> "$work/writer.err" && [ ! -e "$work/report.json" ] \
    && grep -q "unparseable duration" "$work/writer.err"
}
check "unparseable duration: writer exits non-zero, no report" unparseable_fails
# golden-path.sh then writes its empty-steps fallback; run.sh must reject it.
printf '{"overall":"pass","pass":0,"fail":0,"warn":0,"steps":{}}\n' > "$work/fallback.json"
# runs_fail <report> <out>: run.sh exits non-zero and writes no output file.
runs_fail() {
  ! bash "$root/.github/benchmarks/run.sh" "$1" "$2" > /dev/null 2>&1 && [ ! -e "$2" ]
}
check "empty-steps fallback report: run.sh fails" runs_fail "$work/fallback.json" "$work/f.json"

# --- 2. stale report ---------------------------------------------------------
echo '{"stale":true}' > "$work/stale.json"
stale_removed() {
  # No licence key: golden-path.sh exits in pre-flight, after the start-up rm.
  ! env -u NSELF_PLUGIN_LICENSE_KEY_OWNER GOLDEN_PATH_REPORT_FILE="$work/stale.json" \
    bash "$root/scripts/golden-path.sh" > "$work/gp.out" 2>&1 && [ ! -e "$work/stale.json" ]
}
check "stale report deleted when the golden path exits early" stale_removed
check "run.sh fails on the deleted report" runs_fail "$work/stale.json" "$work/s.json"

# --- 3. sha ------------------------------------------------------------------
fixture="$root/tools/perfbench/testdata/golden-report-pass.json"
sha_is() { # sha_is <expected-prefix-regex> [GITHUB_SHA]
  local out="$work/sha.json"
  if [ -n "${2:-}" ]; then GITHUB_SHA=$2 bash "$root/.github/benchmarks/run.sh" "$fixture" "$out" > /dev/null
  else env -u GITHUB_SHA bash "$root/.github/benchmarks/run.sh" "$fixture" "$out" > /dev/null; fi
  python3 -c '
import json, re, sys
d = json.load(open(sys.argv[1]))
ok = re.fullmatch(sys.argv[2], d["sha"]) and d["schema"] == "perfbench/v1" and d["metrics"][0]["name"] == "time_to_healthy"
sys.exit(0 if ok else 1)' "$out" "$1"
}
check "sha is GITHUB_SHA in CI" sha_is 'abc123def456' abc123def456
check "sha is the checkout HEAD locally" sha_is '[0-9a-f]{40}(-dirty)?'

if [ "$fails" -ne 0 ]; then echo "$fails check(s) failed" >&2; exit 1; fi
echo "all checks passed"
