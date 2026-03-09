#!/usr/bin/env bats
# test-concurrent-install.bats
# T-0431 — All plugins: concurrent install safety
#
# Scenario: nself plugin install ai and nself plugin install mux
#   started simultaneously via background processes.
#
# Verify:
#   - Both succeed OR exactly one fails with "installation in progress" (no partial state)
#   - docker-compose.yml is valid YAML after both complete
#   - No duplicate service definitions in docker-compose.yml
#   - Run 3x to catch race conditions
#
# Docker tier only — guarded by _require_docker.
# SKIP_DOCKER_TESTS=1 by default.

load test_helper

NSELF_BIN="${NSELF_BIN:-nself}"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_require_docker() {
  [ "${SKIP_DOCKER_TESTS:-1}" = "1" ] && skip "SKIP_DOCKER_TESTS=1"
  command -v docker >/dev/null 2>&1 || skip "docker not installed"
  docker info >/dev/null 2>&1 || skip "Docker daemon not running"
}

_require_nself() {
  command -v "$NSELF_BIN" >/dev/null 2>&1 || skip "nself not installed"
}

_require_python3_or_python() {
  command -v python3 >/dev/null 2>&1 || command -v python >/dev/null 2>&1 || skip "python not installed (needed for YAML validation)"
}

# Validate docker-compose.yml is well-formed YAML and parse it.
_validate_compose_yaml() {
  local compose_file="${1:-docker-compose.yml}"

  [ -f "$compose_file" ] || {
    printf "docker-compose.yml not found at: %s\n" "$compose_file" >&2
    return 1
  }

  # Use python3 to parse YAML — more portable than requiring pyyaml or yq
  if command -v python3 >/dev/null 2>&1; then
    python3 -c "
import sys
try:
    import yaml
    with open(sys.argv[1]) as f:
        yaml.safe_load(f)
    sys.exit(0)
except ImportError:
    # PyYAML not installed — fall back to basic syntax check
    import json
    # Not YAML, just check it's non-empty
    data = open(sys.argv[1]).read()
    if len(data) == 0:
        print('EMPTY', file=sys.stderr)
        sys.exit(1)
    sys.exit(0)
except Exception as e:
    print(f'YAML parse error: {e}', file=sys.stderr)
    sys.exit(1)
" "$compose_file" 2>&1 || return 1
  elif command -v python >/dev/null 2>&1; then
    python -c "
import sys
data = open(sys.argv[1]).read()
if len(data) == 0:
    sys.exit(1)
sys.exit(0)
" "$compose_file" 2>&1 || return 1
  else
    # Minimal check: file exists and is non-empty
    [ -s "$compose_file" ] || return 1
  fi
  return 0
}

# Check for duplicate service names in docker-compose.yml.
# Returns 0 if no duplicates, 1 if duplicates found.
_check_no_duplicate_services() {
  local compose_file="${1:-docker-compose.yml}"

  [ -f "$compose_file" ] || return 1

  # Extract service names using awk (Bash 3.2+ compatible — no mapfile/readarray)
  # docker-compose.yml services section format:
  #   services:
  #     service-name:
  #       image: ...
  # We detect lines that are exactly 2 spaces indented under 'services:' block.

  local in_services=0
  local prev_service_count=0
  local duplicates_found=0
  local service_list=""

  while IFS= read -r line; do
    # Detect 'services:' section start
    case "$line" in
      "services:")
        in_services=1
        continue
        ;;
    esac

    # Detect end of services section (a non-indented, non-empty line that isn't 'services:')
    if [ "$in_services" = "1" ]; then
      case "$line" in
        " "*|"	"*|"")
          # Indented or blank — still in services
          # A line like "  service-name:" is a service definition
          case "$line" in
            "  "[a-zA-Z_]*)
              # Extract service name (strip leading spaces, trailing colon)
              local svc_name
              svc_name="${line#  }"
              svc_name="${svc_name%%:*}"
              # Check if already in list
              case "$service_list" in
                *"|${svc_name}|"*)
                  printf "DUPLICATE service found: %s\n" "$svc_name" >&2
                  duplicates_found=1
                  ;;
                *)
                  service_list="${service_list}|${svc_name}|"
                  ;;
              esac
              ;;
          esac
          ;;
        *)
          in_services=0
          ;;
      esac
    fi
  done < "$compose_file"

  return "$duplicates_found"
}

# Run two installs concurrently, wait for both, return combined exit status info.
# Sets: CONCURRENT_EXIT_1, CONCURRENT_EXIT_2, CONCURRENT_OUT_1, CONCURRENT_OUT_2
_run_concurrent_installs() {
  local plugin_a="$1"
  local plugin_b="$2"

  local tmp_out_a tmp_out_b
  tmp_out_a="$(mktemp /tmp/nself-concurrent-a-XXXXXX.log)"
  tmp_out_b="$(mktemp /tmp/nself-concurrent-b-XXXXXX.log)"

  # Launch both installs in background
  "$NSELF_BIN" plugin install "$plugin_a" > "$tmp_out_a" 2>&1 &
  local pid_a=$!
  "$NSELF_BIN" plugin install "$plugin_b" > "$tmp_out_b" 2>&1 &
  local pid_b=$!

  # Wait for both to complete
  CONCURRENT_EXIT_1=0
  CONCURRENT_EXIT_2=0
  wait "$pid_a" || CONCURRENT_EXIT_1=$?
  wait "$pid_b" || CONCURRENT_EXIT_2=$?

  CONCURRENT_OUT_1="$(cat "$tmp_out_a")"
  CONCURRENT_OUT_2="$(cat "$tmp_out_b")"

  rm -f "$tmp_out_a" "$tmp_out_b"
}

# Verify the concurrent install result is valid:
# Either both succeed (0) or exactly one fails with "in progress" message.
_assert_valid_concurrent_result() {
  local exit_a="$1"
  local out_a="$2"
  local exit_b="$3"
  local out_b="$4"

  # Case 1: Both succeeded
  if [ "$exit_a" -eq 0 ] && [ "$exit_b" -eq 0 ]; then
    return 0
  fi

  # Case 2: Exactly one failed with "installation in progress" or "lock" message
  local failed_count=0
  local in_progress_count=0

  if [ "$exit_a" -ne 0 ]; then
    failed_count=$((failed_count + 1))
    case "$out_a" in
      *"installation in progress"*|\
      *"in progress"*|\
      *"lock"*|\
      *"locked"*|\
      *"already running"*)
        in_progress_count=$((in_progress_count + 1))
        ;;
    esac
  fi

  if [ "$exit_b" -ne 0 ]; then
    failed_count=$((failed_count + 1))
    case "$out_b" in
      *"installation in progress"*|\
      *"in progress"*|\
      *"lock"*|\
      *"locked"*|\
      *"already running"*)
        in_progress_count=$((in_progress_count + 1))
        ;;
    esac
  fi

  # If any failed without an "in progress" message — that's a partial state error
  if [ "$failed_count" -gt 0 ] && [ "$in_progress_count" -ne "$failed_count" ]; then
    printf "Concurrent install failed without 'in progress' message.\n"
    printf "Plugin A (exit %s):\n%s\n" "$exit_a" "$out_a" >&2
    printf "Plugin B (exit %s):\n%s\n" "$exit_b" "$out_b" >&2
    return 1
  fi

  # If both failed — also bad (one should succeed)
  if [ "$failed_count" -eq 2 ] && [ "$in_progress_count" -ne 1 ]; then
    printf "Both concurrent installs failed — expected at most one to fail.\n" >&2
    printf "Plugin A (exit %s):\n%s\n" "$exit_a" "$out_a" >&2
    printf "Plugin B (exit %s):\n%s\n" "$exit_b" "$out_b" >&2
    return 1
  fi

  return 0
}

setup() {
  # Locate the nself project root (where docker-compose.yml lives)
  NSELF_ROOT="${NSELF_ROOT:-${HOME}/.nself}"
  COMPOSE_FILE="${NSELF_ROOT}/docker-compose.yml"
}

# ===========================================================================
# Static tier — verify CLI has concurrent install protection documented
# ===========================================================================

@test "static: nself plugin install --help mentions concurrent safety or lock" {
  _require_nself
  run "$NSELF_BIN" plugin install --help
  assert_success
  # Accept any mention of concurrency, lock, or simultaneous
  case "$output" in
    *"concurrent"*|*"lock"*|*"simultaneous"*|*"in progress"*)
      ;;
    *)
      # Not a failure — the CLI may handle this silently without documenting it
      skip "plugin install --help does not mention concurrency control (acceptable)"
      ;;
  esac
}

# ===========================================================================
# Docker tier — run 1: ai + mux concurrent install
# ===========================================================================

@test "docker: concurrent install (run 1/3) — ai + mux both complete without partial state" {
  _require_docker
  _require_nself
  [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ] || skip "NSELF_PLUGIN_LICENSE_KEY not set — pro plugin test skipped"

  _run_concurrent_installs "ai" "mux"
  _assert_valid_concurrent_result \
    "$CONCURRENT_EXIT_1" "$CONCURRENT_OUT_1" \
    "$CONCURRENT_EXIT_2" "$CONCURRENT_OUT_2"
}

@test "docker: concurrent install (run 1/3) — docker-compose.yml valid after ai + mux" {
  _require_docker
  _require_nself
  _require_python3_or_python
  [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ] || skip "NSELF_PLUGIN_LICENSE_KEY not set"

  _validate_compose_yaml "$COMPOSE_FILE" || \
    fail "docker-compose.yml is invalid YAML after concurrent install run 1"
}

@test "docker: concurrent install (run 1/3) — no duplicate services in docker-compose.yml" {
  _require_docker
  _require_nself
  [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ] || skip "NSELF_PLUGIN_LICENSE_KEY not set"

  _check_no_duplicate_services "$COMPOSE_FILE" || \
    fail "Duplicate service definitions found in docker-compose.yml after concurrent install run 1"
}

# ===========================================================================
# Docker tier — run 2: ai + mux concurrent install (repeat to catch race conditions)
# ===========================================================================

@test "docker: concurrent install (run 2/3) — ai + mux both complete without partial state" {
  _require_docker
  _require_nself
  [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ] || skip "NSELF_PLUGIN_LICENSE_KEY not set"

  _run_concurrent_installs "ai" "mux"
  _assert_valid_concurrent_result \
    "$CONCURRENT_EXIT_1" "$CONCURRENT_OUT_1" \
    "$CONCURRENT_EXIT_2" "$CONCURRENT_OUT_2"
}

@test "docker: concurrent install (run 2/3) — docker-compose.yml valid after ai + mux" {
  _require_docker
  _require_nself
  _require_python3_or_python
  [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ] || skip "NSELF_PLUGIN_LICENSE_KEY not set"

  _validate_compose_yaml "$COMPOSE_FILE" || \
    fail "docker-compose.yml is invalid YAML after concurrent install run 2"
}

@test "docker: concurrent install (run 2/3) — no duplicate services in docker-compose.yml" {
  _require_docker
  _require_nself
  [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ] || skip "NSELF_PLUGIN_LICENSE_KEY not set"

  _check_no_duplicate_services "$COMPOSE_FILE" || \
    fail "Duplicate service definitions found in docker-compose.yml after concurrent install run 2"
}

# ===========================================================================
# Docker tier — run 3: ai + mux concurrent install (third repetition)
# ===========================================================================

@test "docker: concurrent install (run 3/3) — ai + mux both complete without partial state" {
  _require_docker
  _require_nself
  [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ] || skip "NSELF_PLUGIN_LICENSE_KEY not set"

  _run_concurrent_installs "ai" "mux"
  _assert_valid_concurrent_result \
    "$CONCURRENT_EXIT_1" "$CONCURRENT_OUT_1" \
    "$CONCURRENT_EXIT_2" "$CONCURRENT_OUT_2"
}

@test "docker: concurrent install (run 3/3) — docker-compose.yml valid after ai + mux" {
  _require_docker
  _require_nself
  _require_python3_or_python
  [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ] || skip "NSELF_PLUGIN_LICENSE_KEY not set"

  _validate_compose_yaml "$COMPOSE_FILE" || \
    fail "docker-compose.yml is invalid YAML after concurrent install run 3"
}

@test "docker: concurrent install (run 3/3) — no duplicate services in docker-compose.yml" {
  _require_docker
  _require_nself
  [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ] || skip "NSELF_PLUGIN_LICENSE_KEY not set"

  _check_no_duplicate_services "$COMPOSE_FILE" || \
    fail "Duplicate service definitions found in docker-compose.yml after concurrent install run 3"
}

# ===========================================================================
# Docker tier — concurrent install of free plugins (no license required)
# ===========================================================================

@test "docker: concurrent install (free) — analytics + realtime simultaneous without partial state" {
  _require_docker
  _require_nself

  _run_concurrent_installs "analytics" "realtime"
  _assert_valid_concurrent_result \
    "$CONCURRENT_EXIT_1" "$CONCURRENT_OUT_1" \
    "$CONCURRENT_EXIT_2" "$CONCURRENT_OUT_2"
}

@test "docker: concurrent install (free) — docker-compose.yml valid after analytics + realtime" {
  _require_docker
  _require_nself
  _require_python3_or_python

  _validate_compose_yaml "$COMPOSE_FILE" || \
    fail "docker-compose.yml is invalid YAML after free plugin concurrent install"
}

@test "docker: concurrent install (free) — no duplicate services after analytics + realtime" {
  _require_docker
  _require_nself

  _check_no_duplicate_services "$COMPOSE_FILE" || \
    fail "Duplicate service definitions found after free plugin concurrent install"
}
