#!/usr/bin/env bash
# Purpose: static guard for scripts/ci/hasura-upgrade-smoke.sh (D-1152). The switch
#          `nself start` (step 4) must pass --skip-db-init: a plain start re-applies the
#          saved Hasura metadata to the new engine (start.go: hasura.ApplyIfPresent), so
#          the preservation checks in step 5 could never fail. The smoke itself needs
#          Docker; this check does not.
# Inputs:  none (reads the smoke script next to this file).
# Outputs: "ok <case>" per passing case; exit 0 when all pass, 1 otherwise.
# Constraints: bash 3.2 compatible; offline.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SMOKE="${1:-${HERE}/hasura-upgrade-smoke.sh}"

# The `nsf start` lines between the step 4 and step 5 headers.
STEP4="$(awk '/^step "4 /{on=1} /^step "5 /{on=0} on && /^nsf start/' "${SMOKE}")"

if [ -z "${STEP4}" ]; then
  printf 'FAIL step 4 has no `nsf start` line (script layout changed: update this check)\n' >&2
  exit 1
fi
case "${STEP4}" in
  *--skip-db-init*) printf 'ok step 4 start passes --skip-db-init\n' ;;
  *) printf 'FAIL step 4 start re-applies saved metadata (no --skip-db-init): %s\n' "${STEP4}" >&2; exit 1 ;;
esac
