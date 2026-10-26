#!/usr/bin/env bash
# Purpose: prove scripts/golden-path.sh writes its JSON report however it ends,
#          so a run that aborts outside the per-step failure paths still names
#          the step (D-1482). Three cases against stub nself and docker
#          binaries: a missing prerequisite, a signal while a step is in flight,
#          and a health wait that times out (the deliberate path).
# Usage:   bash scripts/ci/golden-path_test.sh
# Inputs:  none; needs bash >= 4 (golden-path.sh uses associative arrays) and
#          python3. Prints "skip" and exits 0 when no bash >= 4 is found.
# Outputs: "ok <case>" per passing case; exit 0 when all pass, exit 1 on the
#          first mismatch.
# Constraints: offline; no real nself, docker or network is touched (stubs lead
#          PATH, the report, work dir and diagnostics paths are per-run).
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="${HERE}/../golden-path.sh"
BASH4=""
for b in "$(command -v bash)" /opt/homebrew/bin/bash /usr/local/bin/bash; do
  if [ -x "${b}" ] && [ "$("${b}" -c 'echo ${BASH_VERSINFO[0]}')" -ge 4 ]; then BASH4="${b}"; break; fi
done
[ -n "${BASH4}" ] || { echo "skip: no bash >= 4 on this host"; exit 0; }

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
mkdir -p "${WORK}/stubs"
# doctor fails fast so the health wait keeps looping; everything else succeeds.
cat > "${WORK}/stubs/nself" <<'STUB'
#!/bin/sh
case "$1" in doctor) exit 1 ;; *) exit 0 ;; esac
STUB
printf '#!/bin/sh\nexit 0\n' > "${WORK}/stubs/docker"
chmod +x "${WORK}/stubs/nself" "${WORK}/stubs/docker"

# A dry run skips step 2's mkdir, so give the health wait a project dir to enter.
mkdir -p "${WORK}/w2/testproject" "${WORK}/w3/testproject"

# report FILE EXPR: evaluate EXPR over the report JSON "d".
report() { python3 -c 'import json,sys;d=json.load(open(sys.argv[1]));print(eval(sys.argv[2]))' "$1" "$2"; }
die() { echo "FAIL $1" >&2; exit 1; }

# 1. Missing prerequisite: exits before any step, still reports.
R1="${WORK}/r1.json"
rc=0
env -u NSELF_PLUGIN_LICENSE_KEY_OWNER PATH="${WORK}/stubs:${PATH}" GOLDEN_PATH_REPORT_FILE="${R1}" \
  "${BASH4}" "${SCRIPT}" > "${WORK}/o1" 2>&1 || rc=$?
[ "${rc}" -eq 1 ] || die "missing-prerequisite: exit ${rc}, want 1"
[ -f "${R1}" ] || die "missing-prerequisite: no report written"
[ "$(report "${R1}" 'd["overall"]')" = fail ] || die "missing-prerequisite: overall is not fail"
[ "$(report "${R1}" 'd["steps"]["1"]["status"]')" = fail ] || die "missing-prerequisite: step 1 not failed"
report "${R1}" 'd["steps"]["1"]["note"]' | grep -q NSELF_PLUGIN_LICENSE_KEY_OWNER || die "missing-prerequisite: note lacks the reason"
echo "ok missing-prerequisite"

# 2. A signal while the health wait (step 6, outside run_step) is in flight.
R2="${WORK}/r2.json"
NSELF_PLUGIN_LICENSE_KEY_OWNER=dummy GOLDEN_PATH_DRY_RUN=1 GOLDEN_PATH_HEALTH_TIMEOUT=6 \
  GOLDEN_PATH_REPORT_FILE="${R2}" GOLDEN_PATH_WORK_DIR="${WORK}/w2" PATH="${WORK}/stubs:${PATH}" \
  "${BASH4}" "${SCRIPT}" > "${WORK}/o2" 2>&1 &
PID=$!
for _ in $(seq 1 50); do grep -q "Waiting for services healthy" "${WORK}/o2" 2>/dev/null && break; sleep 0.2; done
grep -q "Waiting for services healthy" "${WORK}/o2" || { kill "${PID}" 2>/dev/null || true; die "signal: never reached the health wait"; }
# bash runs the TERM trap once the health-wait subshell returns (the 6s timeout
# above), before the step's own failure handling would write the report.
kill -TERM "${PID}"
rc=0
wait "${PID}" || rc=$?
[ "${rc}" -eq 143 ] || die "signal: exit ${rc}, want 143"
[ -f "${R2}" ] || die "signal: no report written"
[ "$(report "${R2}" 'd["overall"]')" = fail ] || die "signal: overall is not fail"
[ "$(report "${R2}" 'd["steps"]["6"]["status"]')" = fail ] || die "signal: step 6 not named as failed"
report "${R2}" 'd["steps"]["6"]["note"]' | grep -q "exited 143" || die "signal: note lacks the exit status"
echo "ok signal-during-step-6"

# 3. Deliberate failure (health never arrives): the explicit path still reports once.
R3="${WORK}/r3.json"
rc=0
NSELF_PLUGIN_LICENSE_KEY_OWNER=dummy GOLDEN_PATH_DRY_RUN=1 GOLDEN_PATH_HEALTH_TIMEOUT=1 \
  GOLDEN_PATH_REPORT_FILE="${R3}" GOLDEN_PATH_WORK_DIR="${WORK}/w3" PATH="${WORK}/stubs:${PATH}" \
  "${BASH4}" "${SCRIPT}" > "${WORK}/o3" 2>&1 || rc=$?
[ "${rc}" -eq 1 ] || die "health-timeout: exit ${rc}, want 1"
[ "$(report "${R3}" 'd["steps"]["6"]["note"]')" = "services never became healthy" ] || die "health-timeout: note was overwritten"
[ "$(report "${R3}" 'd["fail"]')" = 1 ] || die "health-timeout: fail count is not 1"
echo "ok health-timeout-keeps-its-note"

# capture_diagnostics writes a fixed /tmp path; remove what case 3 left.
rm -f /tmp/golden-path-diag-step6.txt
