#!/usr/bin/env bash
# Atheris-style fuzzing for CLI validators via random input generation
# 10,000 iterations of random input against validation functions
# No atheris dependency — uses pure Bash random generation
#
# Usage: ./tests/fuzz/test-cli-validators.sh [--iterations N] [--verbose]
# Exit code: 0 = no crashes detected
#            1 = crash, infinite loop, or SIGSEGV detected in a validator

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ITERATIONS=10000
VERBOSE=0
TIMEOUT_PER_CHECK=1   # 1 second max per validator call
CRASH_LOG="/tmp/nself-fuzz-crashes.log"

# Parse args
i=1
while [ "$i" -le "$#" ]; do
  arg="$(eval echo \${${i}})"
  case "$arg" in
    --iterations)
      i=$(( i + 1 ))
      ITERATIONS="$(eval echo \${${i}})"
      ;;
    --verbose)
      VERBOSE=1
      ;;
  esac
  i=$(( i + 1 ))
done

log() {
  printf '[fuzz-validators] %s\n' "$1"
}

log_verbose() {
  if [ "$VERBOSE" -eq 1 ]; then
    printf '[fuzz-validators] %s\n' "$1"
  fi
}

# Load CLI source — source validation functions for direct testing
# If files don't exist yet, the validators below are stubs
DOMAIN_SRC="${REPO_ROOT}/src/services/domain/domain.sh"
LICENSE_SRC="${REPO_ROOT}/src/cli/license.sh"
PLUGIN_SRC="${REPO_ROOT}/src/lib/plugin/licensing.sh"

# Source available files — ignore missing files
for src in "${DOMAIN_SRC}" "${LICENSE_SRC}" "${PLUGIN_SRC}"; do
  if [ -f "${src}" ]; then
    # shellcheck disable=SC1090
    . "${src}" 2>/dev/null || true
  fi
done

# -----------------------------------------------------------------------
# Validator wrappers
# Each wrapper calls the real validator if available, else a stub.
# The key requirement: no panic, no crash, no infinite loop, exit within
# $TIMEOUT_PER_CHECK seconds.
# -----------------------------------------------------------------------

_validate_domain() {
  local input="$1"
  if command -v nself_validate_domain >/dev/null 2>&1; then
    nself_validate_domain "${input}" >/dev/null 2>&1
  else
    # Stub: basic domain character check
    printf '%s' "${input}" | grep -qE '^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z]{2,})+$'
  fi
  return 0
}

_validate_port() {
  local input="$1"
  if command -v nself_validate_port >/dev/null 2>&1; then
    nself_validate_port "${input}" >/dev/null 2>&1
  else
    # Stub: check if numeric in range
    case "${input}" in
      ''|*[!0-9]*) return 1 ;;
    esac
    [ "${input}" -ge 1 ] && [ "${input}" -le 65535 ] 2>/dev/null
  fi
  return 0
}

_validate_plugin_name() {
  local input="$1"
  if command -v nself_validate_plugin_name >/dev/null 2>&1; then
    nself_validate_plugin_name "${input}" >/dev/null 2>&1
  else
    # Stub: alphanumeric + dash, 1-64 chars
    printf '%s' "${input}" | grep -qE '^[a-z0-9][a-z0-9\-]{0,62}[a-z0-9]?$'
  fi
  return 0
}

_validate_env_var() {
  local input="$1"
  if command -v nself_validate_env_var >/dev/null 2>&1; then
    nself_validate_env_var "${input}" >/dev/null 2>&1
  else
    # Stub: uppercase letters, digits, underscore; must start with letter
    printf '%s' "${input}" | grep -qE '^[A-Z][A-Z0-9_]{0,254}$'
  fi
  return 0
}

_validate_license_key() {
  local input="$1"
  if command -v nself_validate_license >/dev/null 2>&1; then
    nself_validate_license "${input}" >/dev/null 2>&1
  else
    # Stub: check prefix + minimum length
    case "${input}" in
      nself_pro_*|nself_max_*|nself_biz_*|nself_ent_*)
        [ "$(printf '%s' "${input}" | wc -c)" -ge 42 ]
        ;;
      *) return 1 ;;
    esac
  fi
  return 0
}

# -----------------------------------------------------------------------
# Random string generator — pure Bash/POSIX, no python dependency
# Generates a random string of length 1-256 from /dev/urandom
# Uses base64 to ensure printable output then trims to requested length
# -----------------------------------------------------------------------
random_string() {
  local max_len=256
  local len=$(( (RANDOM % max_len) + 1 ))
  # Read bytes from urandom, base64-encode, strip newlines, take first $len chars
  dd if=/dev/urandom bs=256 count=1 2>/dev/null | base64 | tr -d '\n=' | cut -c1-${len}
}

# Run a single validator with timeout, capture exit code and crash status
run_validator() {
  local validator_fn="$1"
  local input="$2"
  local exit_code=0

  # Run with timeout — prevents infinite loops
  timeout "${TIMEOUT_PER_CHECK}" bash -c "
    set -u
    ${validator_fn}() {
      $(declare -f "${validator_fn}" 2>/dev/null | tail -n +2 | head -n -1 | tr '\n' ';')
    }
    ${validator_fn} \"\$1\" >/dev/null 2>&1
  " -- "${input}" 2>/dev/null
  exit_code=$?

  # Exit code 124 = timeout (potential infinite loop)
  if [ "$exit_code" -eq 124 ]; then
    printf 'TIMEOUT'
    return 124
  fi

  # Exit codes > 128 typically indicate signals (SIGSEGV=139, etc.)
  if [ "$exit_code" -gt 128 ]; then
    printf 'SIGNAL:%d' "$exit_code"
    return "$exit_code"
  fi

  # Normal exit (pass or fail is fine — crash is not)
  return 0
}

# -----------------------------------------------------------------------
# Main fuzzing loop
# -----------------------------------------------------------------------
CRASH_COUNT=0
PASS_COUNT=0

log "Starting CLI validator fuzz test"
log "Iterations : ${ITERATIONS}"
log "Timeout    : ${TIMEOUT_PER_CHECK}s per check"
log "Validators : domain, port, plugin_name, env_var, license_key"
log ""

# Clear crash log
printf '' > "${CRASH_LOG}"

VALIDATORS="_validate_domain _validate_port _validate_plugin_name _validate_env_var _validate_license_key"

iter=0
while [ "$iter" -lt "$ITERATIONS" ]; do
  iter=$(( iter + 1 ))

  # Generate random input
  input="$(random_string)"

  # Run each validator
  for validator in $VALIDATORS; do
    result="$(run_validator "${validator}" "${input}" 2>/dev/null)"
    rc=$?

    if [ "$rc" -eq 124 ] || [ "$rc" -gt 128 ]; then
      CRASH_COUNT=$(( CRASH_COUNT + 1 ))
      hex_input="$(printf '%s' "${input}" | xxd -p 2>/dev/null | head -c 64 || printf '(xxd unavailable)')"
      msg="CRITICAL FAIL [iter=${iter}] ${validator}: exit_code=${rc} input_hex=${hex_input} result=${result}"
      printf '%s\n' "$msg" >> "${CRASH_LOG}"
      printf '[fuzz-validators] %s\n' "$msg" >&2
    else
      PASS_COUNT=$(( PASS_COUNT + 1 ))
      log_verbose "  iter=${iter} ${validator}: OK"
    fi
  done

  # Progress every 1000 iterations
  if [ $(( iter % 1000 )) -eq 0 ]; then
    log "Progress: ${iter}/${ITERATIONS} iterations (${CRASH_COUNT} crashes so far)"
  fi
done

log ""
log "========================================"
log "Fuzz Test Summary"
log "========================================"
printf '[fuzz-validators] Iterations    : %d\n' "$ITERATIONS"
printf '[fuzz-validators] Validator runs: %d\n' "$PASS_COUNT"
printf '[fuzz-validators] Crashes       : %d\n' "$CRASH_COUNT"
log "========================================"

if [ "$CRASH_COUNT" -gt 0 ]; then
  log "FAIL: ${CRASH_COUNT} crash(es) detected. See ${CRASH_LOG} for details."
  cat "${CRASH_LOG}" >&2
  exit 1
fi

log "PASS: No crashes detected across ${ITERATIONS} iterations."
exit 0
