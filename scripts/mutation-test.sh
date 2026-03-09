#!/usr/bin/env bash
# Mutation testing for CLI critical paths using mutmut
# Target functions: license key parsing, plugin install tier gate,
# nginx config generation, domain activation
# Requires: mutmut, python3, bats
#
# Usage: ./scripts/mutation-test.sh [--verbose]
# Exit code: 0 = score >= 75% and no surviving security mutants
#            1 = score < 75% or surviving security mutant detected

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERBOSE=0
MUTATION_THRESHOLD=75

# Parse args
for arg in "$@"; do
  case "$arg" in
    --verbose) VERBOSE=1 ;;
  esac
done

log() {
  printf '[mutation-test] %s\n' "$1"
}

log_verbose() {
  if [ "$VERBOSE" -eq 1 ]; then
    printf '[mutation-test] %s\n' "$1"
  fi
}

fail() {
  printf '[mutation-test] FAIL: %s\n' "$1" >&2
  exit 1
}

# Critical source paths to mutate
MUTMUT_PATH_LICENSE="src/cli/license.sh"
MUTMUT_PATH_PLUGIN_TIER="src/lib/plugin/licensing.sh"
MUTMUT_PATH_NGINX="src/services/nginx/nginx-setup.sh"
MUTMUT_PATH_DOMAIN="src/services/domain/domain.sh"

# Security-critical function names — any surviving mutant here = hard fail
SECURITY_FUNCTIONS="verify_license parse_license_key check_tier validate_key is_valid_license"

cd "${REPO_ROOT}" || fail "Cannot cd to repo root: ${REPO_ROOT}"

# Verify mutmut is available
if ! command -v mutmut >/dev/null 2>&1; then
  fail "mutmut not found. Install with: pip install mutmut"
fi

# Verify python3 is available
if ! command -v python3 >/dev/null 2>&1; then
  fail "python3 not found. Required for mutmut."
fi

log "Starting mutation testing for CLI critical paths"
log "Threshold: ${MUTATION_THRESHOLD}%"
log ""

OVERALL_PASS=1
TOTAL_MUTANTS=0
TOTAL_CAUGHT=0
TOTAL_MISSED=0
SECURITY_FAIL=0

run_mutmut_on_path() {
  local path_arg="$1"
  local label="$2"

  log "Running mutmut on: ${path_arg}"

  # Run mutmut — we run + results in sequence
  mutmut run --paths-to-mutate "${path_arg}" 2>/dev/null || true

  # Capture results
  local results
  results="$(mutmut results 2>/dev/null)"

  log_verbose "Raw results for ${label}:"
  log_verbose "${results}"

  # Parse counts from mutmut results output
  # mutmut results format: "Survived (N)" or "Killed (N)"
  local survived
  survived="$(printf '%s\n' "${results}" | grep -c 'Survived' 2>/dev/null || printf '0')"
  local killed
  killed="$(printf '%s\n' "${results}" | grep -c 'Killed' 2>/dev/null || printf '0')"

  local total
  total=$(( survived + killed ))

  local score=0
  if [ "$total" -gt 0 ]; then
    # Integer division — use awk for percentage
    score=$(awk -v caught="$killed" -v tot="$total" 'BEGIN { printf "%d", (caught / tot) * 100 }')
  fi

  printf '[mutation-test]   %s: %d/%d killed (%d%%)\n' "${label}" "$killed" "$total" "$score"

  TOTAL_MUTANTS=$(( TOTAL_MUTANTS + total ))
  TOTAL_CAUGHT=$(( TOTAL_CAUGHT + killed ))
  TOTAL_MISSED=$(( TOTAL_MISSED + survived ))

  if [ "$score" -lt "$MUTATION_THRESHOLD" ]; then
    log "  WARNING: Score ${score}% is below threshold ${MUTATION_THRESHOLD}%"
    OVERALL_PASS=0
  fi

  # Check for surviving mutants in security-critical functions
  if [ "$survived" -gt 0 ]; then
    local survived_details
    survived_details="$(mutmut results --status survived 2>/dev/null || printf '')"

    for fn in $SECURITY_FUNCTIONS; do
      if printf '%s\n' "${survived_details}" | grep -q "${fn}"; then
        printf '[mutation-test]   SECURITY FAIL: Surviving mutant in %s (function: %s)\n' "${label}" "${fn}" >&2
        SECURITY_FAIL=1
      fi
    done

    if [ "$VERBOSE" -eq 1 ]; then
      log "  Surviving mutants in ${label}:"
      printf '%s\n' "${survived_details}"
    fi
  fi
}

# Run mutation testing on each critical file
run_mutmut_on_path "${MUTMUT_PATH_LICENSE}" "license.sh"
run_mutmut_on_path "${MUTMUT_PATH_PLUGIN_TIER}" "licensing.sh (plugin tier)"
run_mutmut_on_path "${MUTMUT_PATH_NGINX}" "nginx-setup.sh"
run_mutmut_on_path "${MUTMUT_PATH_DOMAIN}" "domain.sh"

# Compute overall score
OVERALL_SCORE=0
if [ "$TOTAL_MUTANTS" -gt 0 ]; then
  OVERALL_SCORE=$(awk -v caught="$TOTAL_CAUGHT" -v tot="$TOTAL_MUTANTS" \
    'BEGIN { printf "%d", (caught / tot) * 100 }')
fi

log ""
log "========================================"
log "Mutation Testing Summary"
log "========================================"
printf '[mutation-test] Total mutants : %d\n' "$TOTAL_MUTANTS"
printf '[mutation-test] Caught        : %d\n' "$TOTAL_CAUGHT"
printf '[mutation-test] Survived      : %d\n' "$TOTAL_MISSED"
printf '[mutation-test] Overall score : %d%%\n' "$OVERALL_SCORE"
log "========================================"

# Determine final exit status
if [ "$SECURITY_FAIL" -eq 1 ]; then
  fail "HARD FAIL: Surviving mutant(s) detected in auth/security code. Score is irrelevant — fix immediately."
fi

if [ "$OVERALL_PASS" -eq 0 ]; then
  fail "Score ${OVERALL_SCORE}% is below required threshold of ${MUTATION_THRESHOLD}%."
fi

log "PASS: Mutation score ${OVERALL_SCORE}% meets threshold of ${MUTATION_THRESHOLD}%."
exit 0
