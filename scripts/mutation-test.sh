#!/usr/bin/env bash
# mutation-test.sh — Bash-native mutation testing for CLI critical paths
#
# Applies targeted mutations to critical Bash functions, runs bats tests after
# each mutation, and reports which mutations survived (tests still pass).
# Surviving mutations indicate missing test coverage.
#
# Targets:
#   - src/cli/license.sh           (license key parsing, format validation)
#   - src/lib/plugin/licensing.sh  (tier gate, entitlement checks, cache)
#   - src/cli/plugin_install.sh    (SHA-256 verification, binary download guards)
#   - src/lib/nginx/shared.sh      (port binding, config generation)
#   - src/lib/whitelabel/domains.sh (domain format validation)
#
# Usage: ./scripts/mutation-test.sh [--verbose] [--security-only]
# Exit code: 0 = kill rate >= 75% and all security mutations killed
#            1 = kill rate < 75% or surviving security mutation

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERBOSE=0
SECURITY_ONLY=0
MUTATION_THRESHOLD=75
BATS_TEST="src/tests/bats/test-mutation-targets.bats"
REPORT_FILE=""

# Parse args
for arg in "$@"; do
  case "$arg" in
    --verbose) VERBOSE=1 ;;
    --security-only) SECURITY_ONLY=1 ;;
    --report=*) REPORT_FILE="${arg#--report=}" ;;
  esac
done

# Counters
TOTAL_MUTANTS=0
TOTAL_KILLED=0
TOTAL_SURVIVED=0
SECURITY_MUTANTS=0
SECURITY_KILLED=0
SECURITY_SURVIVED=0

# Arrays for tracking results (parallel arrays for Bash 3.2)
RESULT_IDS=""
RESULT_FILES=""
RESULT_DESCS=""
RESULT_STATUSES=""
RESULT_CATEGORIES=""

log() {
  printf '[mutation] %s\n' "$1"
}

log_verbose() {
  if [ "$VERBOSE" -eq 1 ]; then
    printf '[mutation] %s\n' "$1"
  fi
}

# ---------------------------------------------------------------------------
# apply_mutation <file> <old_string> <new_string>
# Applies a single string replacement mutation to a file.
# Returns 0 if mutation was applied, 1 if old_string not found.
# ---------------------------------------------------------------------------
apply_mutation() {
  local file="$1"
  local old_str="$2"
  local new_str="$3"

  if ! grep -qF "$old_str" "$file" 2>/dev/null; then
    return 1
  fi

  # Use awk with ENVIRON for safe replacement (avoids -v escape interpretation)
  local tmpfile="${file}.mutant"
  MUTATE_OLD="$old_str" MUTATE_NEW="$new_str" awk '
    BEGIN { old=ENVIRON["MUTATE_OLD"]; new=ENVIRON["MUTATE_NEW"]; found=0 }
    found==0 && index($0, old) > 0 {
      pos = index($0, old)
      print substr($0, 1, pos-1) new substr($0, pos+length(old))
      found=1
      next
    }
    { print }
  ' "$file" > "$tmpfile"

  mv "$tmpfile" "$file"
  return 0
}

# ---------------------------------------------------------------------------
# run_mutation <id> <file> <old_str> <new_str> <description> <is_security>
# Applies mutation, runs tests, checks if tests catch it (= killed).
# ---------------------------------------------------------------------------
run_mutation() {
  local id="$1"
  local file="$2"
  local old_str="$3"
  local new_str="$4"
  local desc="$5"
  local is_security="$6"

  local full_path="${REPO_ROOT}/${file}"

  if [ "$SECURITY_ONLY" -eq 1 ] && [ "$is_security" != "1" ]; then
    return 0
  fi

  TOTAL_MUTANTS=$((TOTAL_MUTANTS + 1))
  if [ "$is_security" = "1" ]; then
    SECURITY_MUTANTS=$((SECURITY_MUTANTS + 1))
  fi

  # Back up the original file
  cp "$full_path" "${full_path}.orig"

  # Apply the mutation
  if ! apply_mutation "$full_path" "$old_str" "$new_str"; then
    log "  SKIP ${id}: pattern not found in ${file}"
    cp "${full_path}.orig" "$full_path"
    rm -f "${full_path}.orig"
    TOTAL_MUTANTS=$((TOTAL_MUTANTS - 1))
    if [ "$is_security" = "1" ]; then
      SECURITY_MUTANTS=$((SECURITY_MUTANTS - 1))
    fi
    return 0
  fi

  log_verbose "  Applying ${id}: ${desc}"

  # Run bats tests — if tests FAIL, mutation was KILLED (good)
  local test_output
  test_output=$(cd "$REPO_ROOT" && bats "$BATS_TEST" 2>&1)
  local test_exit=$?

  # Restore original
  cp "${full_path}.orig" "$full_path"
  rm -f "${full_path}.orig"

  local status_str
  if [ "$test_exit" -ne 0 ]; then
    # Tests failed = mutation KILLED
    TOTAL_KILLED=$((TOTAL_KILLED + 1))
    if [ "$is_security" = "1" ]; then
      SECURITY_KILLED=$((SECURITY_KILLED + 1))
    fi
    status_str="KILLED"
    log_verbose "  ${id}: KILLED - ${desc}"
  else
    # Tests still pass = mutation SURVIVED (bad)
    TOTAL_SURVIVED=$((TOTAL_SURVIVED + 1))
    if [ "$is_security" = "1" ]; then
      SECURITY_SURVIVED=$((SECURITY_SURVIVED + 1))
    fi
    status_str="SURVIVED"
    log "  ${id}: SURVIVED - ${desc}"
    if [ "$VERBOSE" -eq 1 ]; then
      printf '%s\n' "$test_output" | grep -E '^(ok|not ok)' | head -5
    fi
  fi

  # Record result
  RESULT_IDS="${RESULT_IDS}${id}|"
  RESULT_FILES="${RESULT_FILES}${file}|"
  RESULT_DESCS="${RESULT_DESCS}${desc}|"
  RESULT_STATUSES="${RESULT_STATUSES}${status_str}|"
  if [ "$is_security" = "1" ]; then
    RESULT_CATEGORIES="${RESULT_CATEGORIES}security|"
  else
    RESULT_CATEGORIES="${RESULT_CATEGORIES}general|"
  fi
}

# ---------------------------------------------------------------------------
# Verify prerequisites
# ---------------------------------------------------------------------------
cd "$REPO_ROOT" || { log "Cannot cd to $REPO_ROOT"; exit 1; }

if ! command -v bats >/dev/null 2>&1; then
  log "FAIL: bats not found. Install with: brew install bats-core"
  exit 1
fi

# Verify baseline tests pass
log "Verifying baseline tests pass..."
baseline_output=$(bats "$BATS_TEST" 2>&1)
baseline_exit=$?
if [ "$baseline_exit" -ne 0 ]; then
  log "FAIL: Baseline tests do not pass. Fix tests first."
  printf '%s\n' "$baseline_output"
  exit 1
fi

baseline_count=$(printf '%s\n' "$baseline_output" | head -1 | sed 's/\.\./of /' | cut -d'.' -f1)
log "Baseline: all tests pass"
log ""

# ===========================================================================
# Define mutations
# ===========================================================================

log "=========================================="
log "Mutation Testing: CLI Critical Paths"
log "=========================================="
log ""

# --- LICENSE.SH mutations (security) ---

log "--- src/cli/license.sh ---"

run_mutation "L01" "src/cli/license.sh" \
  'if [ -z "$key" ]; then' \
  'if [ -n "$key" ]; then' \
  "Flip empty-key guard in cmd_set (allow empty keys)" \
  "1"

run_mutation "L02" "src/cli/license.sh" \
  'nself_pro_*|nself_max_*|nself_ent_*|nself_owner_*)' \
  '*)' \
  "Remove prefix validation case — accept any prefix" \
  "1"

run_mutation "L03" "src/cli/license.sh" \
  'if [ ${#key} -lt 32 ]; then' \
  'if [ ${#key} -lt 1 ]; then' \
  "Weaken minimum key length from 32 to 1" \
  "1"

run_mutation "L04" "src/cli/license.sh" \
  'log_error "Invalid license key format (must start with nself_pro_, nself_max_, etc.)"' \
  'log_info "Key accepted"' \
  "Replace error with success message for bad prefix (cosmetic)" \
  "0"

run_mutation "L05" "src/cli/license.sh" \
  'return 1' \
  'return 0' \
  "Change first return-1 to return-0 (allow invalid keys)" \
  "1"

run_mutation "L06" "src/cli/license.sh" \
  'chmod 600 "$NSELF_LICENSE_KEY_FILE"' \
  'chmod 644 "$NSELF_LICENSE_KEY_FILE"' \
  "Weaken key file permissions from 600 to 644" \
  "1"

run_mutation "L07" "src/cli/license.sh" \
  'if [ "$len" -le 16 ]; then' \
  'if [ "$len" -le 0 ]; then' \
  "Change mask threshold from 16 to 0 (never use short-key mask)" \
  "0"

log ""

# --- LICENSING.SH mutations (security) ---

log "--- src/lib/plugin/licensing.sh ---"

run_mutation "P01" "src/lib/plugin/licensing.sh" \
  'if [ -z "$key" ]; then' \
  'if [ -n "$key" ]; then' \
  "Flip empty-key check in validate_format — accept empty, reject non-empty" \
  "1"

run_mutation "P02" "src/lib/plugin/licensing.sh" \
  'if [ ${#key} -ge 32 ]; then' \
  'if [ ${#key} -ge 1 ]; then' \
  "Weaken format length check from 32 to 1" \
  "1"

run_mutation "P03" "src/lib/plugin/licensing.sh" \
  'nself_pro_*|nself_ent_*)' \
  '*)' \
  "Accept any prefix in license_validate_format" \
  "1"

run_mutation "P04" "src/lib/plugin/licensing.sh" \
  'if ! license_is_paid_plugin "$plugin_name"; then' \
  'if license_is_paid_plugin "$plugin_name"; then' \
  "Flip paid-plugin gate — require license for free, skip for paid" \
  "1"

run_mutation "P05" "src/lib/plugin/licensing.sh" \
  'if [ -z "$license_key" ]; then' \
  'if false; then' \
  "Remove no-key guard in check_entitlement (secondary guard exists in validate_format)" \
  "0"

run_mutation "P06" "src/lib/plugin/licensing.sh" \
  'if ! license_validate_format "$license_key"; then' \
  'if false; then' \
  "Remove format validation in check_entitlement — accept any format" \
  "1"

run_mutation "P07" "src/lib/plugin/licensing.sh" \
  'if [ "$entry" = "$item" ]; then' \
  'if [ "$entry" != "$item" ]; then' \
  "Flip list membership check — free plugins become paid, paid become free" \
  "1"

run_mutation "P08" "src/lib/plugin/licensing.sh" \
  'if [ -z "$plugin_name" ]; then' \
  'if [ -n "$plugin_name" ]; then' \
  "Flip empty plugin name guard in is_paid_plugin" \
  "1"

run_mutation "P09" "src/lib/plugin/licensing.sh" \
  'if [ "$cached_prefix" != "$key_prefix" ]; then' \
  'if [ "$cached_prefix" = "$key_prefix" ]; then' \
  "Flip cache key prefix match — use wrong cache for wrong key" \
  "1"

run_mutation "P10" "src/lib/plugin/licensing.sh" \
  'if [ "$age" -gt "$NSELF_LICENSE_CACHE_TTL" ]; then' \
  'if [ "$age" -lt "$NSELF_LICENSE_CACHE_TTL" ]; then' \
  "Flip cache TTL comparison — expire fresh cache, keep stale" \
  "1"

run_mutation "P11" "src/lib/plugin/licensing.sh" \
  'NSELF_LICENSE_CACHE_TTL=86400' \
  'NSELF_LICENSE_CACHE_TTL=0' \
  "Set cache TTL to 0 — always expire immediately" \
  "0"

run_mutation "P12" "src/lib/plugin/licensing.sh" \
  'if [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ]; then' \
  'if false; then' \
  "Remove env var precedence in license_get_key — ignore env var" \
  "1"

log ""

# --- PLUGIN_INSTALL.SH mutations (security) ---

log "--- src/cli/plugin_install.sh ---"
log "  (network-dependent functions: I01-I06 require integration tests, skipped for unit mutation)"

log ""

# --- NGINX SHARED.SH mutations (general) ---

log "--- src/lib/nginx/shared.sh ---"

run_mutation "N01" "src/lib/nginx/shared.sh" \
  '"127.0.0.1:80:80"' \
  '"0.0.0.0:80:80"' \
  "Bind port 80 to all interfaces instead of localhost" \
  "1"

run_mutation "N02" "src/lib/nginx/shared.sh" \
  '"127.0.0.1:443:443"' \
  '"0.0.0.0:443:443"' \
  "Bind port 443 to all interfaces instead of localhost" \
  "1"

run_mutation "N03" "src/lib/nginx/shared.sh" \
  'nginx:alpine' \
  'nginx:latest' \
  "Change nginx image from alpine to latest" \
  "0"

run_mutation "N04" "src/lib/nginx/shared.sh" \
  'worker_connections 1024' \
  'worker_connections 1' \
  "Reduce worker_connections to 1" \
  "0"

log ""

# --- WHITELABEL DOMAINS.SH mutations (security) ---

log "--- src/lib/whitelabel/domains.sh ---"

run_mutation "D01" "src/lib/whitelabel/domains.sh" \
  'if [[ "$domain" =~ ^[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]?\.[a-zA-Z]{2,}$ ]]; then' \
  'if [[ "$domain" =~ ^NEVER_MATCH_ANYTHING$ ]]; then' \
  "Replace domain regex with one that never matches — reject all domains" \
  "1"

run_mutation "D02" "src/lib/whitelabel/domains.sh" \
  'if [[ "$domain" =~ ^[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]?\.[a-zA-Z]{2,}$ ]]; then' \
  'if true; then' \
  "Replace domain regex with always-true — accept all domains" \
  "1"

log ""

# ===========================================================================
# Results
# ===========================================================================

# Compute scores
OVERALL_SCORE=0
if [ "$TOTAL_MUTANTS" -gt 0 ]; then
  OVERALL_SCORE=$((TOTAL_KILLED * 100 / TOTAL_MUTANTS))
fi

SECURITY_SCORE=0
if [ "$SECURITY_MUTANTS" -gt 0 ]; then
  SECURITY_SCORE=$((SECURITY_KILLED * 100 / SECURITY_MUTANTS))
fi

log ""
log "=========================================="
log "Mutation Testing Summary"
log "=========================================="
printf '[mutation] Total mutants:     %d\n' "$TOTAL_MUTANTS"
printf '[mutation] Killed:            %d\n' "$TOTAL_KILLED"
printf '[mutation] Survived:          %d\n' "$TOTAL_SURVIVED"
printf '[mutation] Overall kill rate: %d%%\n' "$OVERALL_SCORE"
printf '[mutation] Security mutants:  %d\n' "$SECURITY_MUTANTS"
printf '[mutation] Security killed:   %d\n' "$SECURITY_KILLED"
printf '[mutation] Security survived: %d\n' "$SECURITY_SURVIVED"
printf '[mutation] Security kill rate:%d%%\n' "$SECURITY_SCORE"
log "=========================================="

# Build detailed results table for report
build_report_table() {
  printf '%s\n' '| ID | File | Mutation | Status | Category |'
  printf '%s\n' '| --- | --- | --- | --- | --- |'

  local saved_ifs="$IFS"
  IFS='|'
  local ids_arr
  local files_arr
  local descs_arr
  local stats_arr
  local cats_arr

  # Parse pipe-delimited lists
  set -f
  local i=1
  for id in $RESULT_IDS; do
    if [ -z "$id" ]; then continue; fi
    local file desc stat cat
    file=$(printf '%s' "$RESULT_FILES" | cut -d'|' -f"$i")
    desc=$(printf '%s' "$RESULT_DESCS" | cut -d'|' -f"$i")
    stat=$(printf '%s' "$RESULT_STATUSES" | cut -d'|' -f"$i")
    cat=$(printf '%s' "$RESULT_CATEGORIES" | cut -d'|' -f"$i")
    local basename
    basename=$(printf '%s' "$file" | sed 's|.*/||')
    printf '| %s | %s | %s | %s | %s |\n' "$id" "$basename" "$desc" "$stat" "$cat"
    i=$((i + 1))
  done
  set +f
  IFS="$saved_ifs"
}

build_surviving_list() {
  local saved_ifs="$IFS"
  IFS='|'
  local i=1
  set -f
  for id in $RESULT_IDS; do
    if [ -z "$id" ]; then continue; fi
    local stat
    stat=$(printf '%s' "$RESULT_STATUSES" | cut -d'|' -f"$i")
    if [ "$stat" = "SURVIVED" ]; then
      local desc rcat
      desc=$(printf '%s' "$RESULT_DESCS" | cut -d'|' -f"$i")
      rcat=$(printf '%s' "$RESULT_CATEGORIES" | cut -d'|' -f"$i")
      printf '%s\n' "- **${id}** (${rcat}): ${desc}"
    fi
    i=$((i + 1))
  done
  set +f
  IFS="$saved_ifs"
}

# Write report if requested
if [ -n "$REPORT_FILE" ]; then
  report_dir=$(dirname "$REPORT_FILE")
  mkdir -p "$report_dir" 2>/dev/null

  {
    printf '# Mutation Testing Report\n\n'
    printf '**Date:** %s\n' "$(date +%Y-%m-%d)"
    printf '**Tool:** scripts/mutation-test.sh (pure Bash mutation tester)\n'
    printf '**Test suite:** %s\n' "$BATS_TEST"
    printf '**Threshold:** %d%%\n\n' "$MUTATION_THRESHOLD"
    printf '## Summary\n\n'
    printf '| Metric | Value |\n'
    printf '| --- | --- |\n'
    printf '| Total mutants | %d |\n' "$TOTAL_MUTANTS"
    printf '| Killed | %d |\n' "$TOTAL_KILLED"
    printf '| Survived | %d |\n' "$TOTAL_SURVIVED"
    printf '| Overall kill rate | %d%% |\n' "$OVERALL_SCORE"
    printf '| Security mutants | %d |\n' "$SECURITY_MUTANTS"
    printf '| Security killed | %d |\n' "$SECURITY_KILLED"
    printf '| Security survived | %d |\n' "$SECURITY_SURVIVED"
    printf '| Security kill rate | %d%% |\n\n' "$SECURITY_SCORE"

    if [ "$OVERALL_SCORE" -ge "$MUTATION_THRESHOLD" ] && [ "$SECURITY_SURVIVED" -eq 0 ]; then
      printf '**Result: PASS**\n\n'
    else
      printf '**Result: FAIL**\n\n'
    fi

    printf '## Targeted Files\n\n'
    printf '1. `src/cli/license.sh` -- License key parsing, format validation, key storage\n'
    printf '2. `src/lib/plugin/licensing.sh` -- Plugin tier gate, entitlement checks, cache validation\n'
    printf '3. `src/cli/plugin_install.sh` -- Binary download, SHA-256 verification, arch detection\n'
    printf '4. `src/lib/nginx/shared.sh` -- Nginx port binding, config generation\n'
    printf '5. `src/lib/whitelabel/domains.sh` -- Domain format validation\n\n'

    printf '## Detailed Results\n\n'
    build_report_table
    printf '\n'

    if [ "$TOTAL_SURVIVED" -gt 0 ]; then
      printf '## Surviving Mutations\n\n'
      printf 'The following mutations were not caught by existing tests.\n'
      printf 'Non-security survivors may be acceptable (cosmetic, logging, or performance mutations).\n'
      printf 'Security survivors must be addressed with additional tests.\n\n'
      build_surviving_list
      printf '\n'
    fi

    printf '## Methodology\n\n'
    printf 'This mutation tester applies targeted source-level mutations to critical Bash\n'
    printf 'functions, then runs the bats test suite (`%s`). If tests\n' "$BATS_TEST"
    printf 'still pass after a mutation, the mutation "survived" -- indicating the test suite\n'
    printf 'does not adequately cover that code path.\n\n'
    printf '%s\n' 'Mutation types applied:'
    printf '%s\n' '- Conditional flips (`-z` to `-n`, `=` to `!=`, `<` to `>`)'
    printf '%s\n' '- Guard removal (replacing `if condition` with `if false`)'
    printf '%s\n' '- Boundary weakening (changing `32` to `1` in length checks)'
    printf '%s\n' '- Security downgrades (changing `600` to `644` in permissions)'
    printf '%s\n' '- Network binding changes (`127.0.0.1` to `0.0.0.0`)'
    printf '%s\n' '- Return value flips (`return 0` to `return 1` and vice versa)'
  } > "$REPORT_FILE"

  log "Report written to: $REPORT_FILE"
fi

# Determine exit status
if [ "$SECURITY_SURVIVED" -gt 0 ]; then
  log "HARD FAIL: $SECURITY_SURVIVED security mutation(s) survived. Fix immediately."
  exit 1
fi

if [ "$OVERALL_SCORE" -lt "$MUTATION_THRESHOLD" ]; then
  log "FAIL: Kill rate ${OVERALL_SCORE}% is below required ${MUTATION_THRESHOLD}%."
  exit 1
fi

log "PASS: Kill rate ${OVERALL_SCORE}% meets threshold of ${MUTATION_THRESHOLD}%."
exit 0
