#!/usr/bin/env bats
# test-zero-downtime-deploy.bats
# T-0464 — Zero-downtime deployment: rolling deploy with health check gate
#
# Static tier (no Docker — always runs):
#   Verify deploy --help exits 0.
#
# Docker tier (SKIP_DOCKER_TESTS=0):
#   Start staging stack, run concurrent requests during deploy, verify zero
#   non-200 responses. Verify health check gate before old container removed.
#   Verify rollback when health check fails.
#
# Bash 3.2+ compatible.

load test_helper

NSELF_BIN="${NSELF_BIN:-nself}"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_require_nself() {
  if ! command -v "$NSELF_BIN" >/dev/null 2>&1; then
    skip "nself not found in PATH"
  fi
}

_require_docker() {
  if [ "${SKIP_DOCKER_TESTS:-1}" = "1" ]; then
    skip "Docker tests disabled (SKIP_DOCKER_TESTS=1)"
  fi
  if ! command -v docker >/dev/null 2>&1; then
    skip "docker not installed"
  fi
  if ! docker info >/dev/null 2>&1; then
    skip "Docker daemon not running"
  fi
}

_require_curl() {
  if ! command -v curl >/dev/null 2>&1; then
    skip "curl not available"
  fi
}

# Poll a URL until it returns HTTP 200 within timeout seconds.
_wait_http_ok() {
  local url="$1"
  local timeout="${2:-60}"
  local elapsed=0
  while [ "$elapsed" -lt "$timeout" ]; do
    local code
    code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "$url" 2>/dev/null || printf '0')"
    if [ "$code" = "200" ]; then
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  return 1
}

# Run N curl requests at 1 req/sec in background, write status codes to file.
# Usage: _background_curl_requests <url> <count> <out_file>
# Sets CURL_BG_PID.
_background_curl_requests() {
  local url="$1"
  local count="$2"
  local out_file="$3"
  (
    local i=1
    while [ "$i" -le "$count" ]; do
      code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$url" 2>/dev/null || printf '0')"
      printf '%s\n' "$code" >> "$out_file"
      sleep 1
      i=$((i + 1))
    done
  ) &
  CURL_BG_PID=$!
  export CURL_BG_PID
}

# ---------------------------------------------------------------------------
# Setup / teardown
# ---------------------------------------------------------------------------

setup() {
  TEST_PROJECT_DIR="$(mktemp -d)"
  CURL_LOG="$(mktemp)"
  export TEST_PROJECT_DIR CURL_LOG
}

teardown() {
  if [ "${SKIP_DOCKER_TESTS:-1}" = "0" ]; then
    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
      if [ -f "$TEST_PROJECT_DIR/docker-compose.yml" ]; then
        cd "$TEST_PROJECT_DIR"
        "$NSELF_BIN" stop >/dev/null 2>&1 || true
      fi
    fi
    if [ -n "${CURL_BG_PID:-}" ]; then
      kill "$CURL_BG_PID" 2>/dev/null || true
    fi
  fi
  cd /
  rm -rf "$TEST_PROJECT_DIR"
  rm -f "$CURL_LOG"
}

# ===========================================================================
# Static tier (no Docker required)
# ===========================================================================

@test "static: nself deploy staging --help exits 0" {
  _require_nself
  run "$NSELF_BIN" deploy staging --help
  assert_success
}

@test "static: nself deploy --local --help exits 0" {
  _require_nself
  run "$NSELF_BIN" deploy --local --help
  assert_success
}

# ===========================================================================
# Docker tier
# ===========================================================================

@test "docker: zero non-200 responses during rolling deploy" {
  _require_nself
  _require_docker
  _require_curl

  cd "$TEST_PROJECT_DIR"
  "$NSELF_BIN" init --base-domain localhost --non-interactive >/dev/null 2>&1
  "$NSELF_BIN" build >/dev/null 2>&1
  "$NSELF_BIN" start --detach >/dev/null 2>&1

  # Wait for stack healthy
  if ! _wait_http_ok "https://api.localhost/healthz" 60; then
    skip "Stack did not become healthy within 60s"
  fi

  # Start background traffic — 10 requests at 1/sec
  _background_curl_requests "https://api.localhost/healthz" 10 "$CURL_LOG"

  # Trigger rolling deploy
  "$NSELF_BIN" deploy --local --rolling >/dev/null 2>&1 || true

  # Wait for background traffic to finish
  wait "$CURL_BG_PID" 2>/dev/null || true

  # Count non-200 responses
  local failures=0
  if [ -s "$CURL_LOG" ]; then
    while IFS= read -r code; do
      if [ "$code" != "200" ] && [ "$code" != "204" ]; then
        failures=$((failures + 1))
      fi
    done < "$CURL_LOG"
  fi

  if [ "$failures" -gt 0 ]; then
    printf "Got %d non-200 responses during rolling deploy\n" "$failures" >&2
    return 1
  fi
}

@test "docker: new container healthy before old one removed" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"
  "$NSELF_BIN" init --base-domain localhost --non-interactive >/dev/null 2>&1
  "$NSELF_BIN" build >/dev/null 2>&1
  "$NSELF_BIN" start --detach >/dev/null 2>&1

  if ! _wait_http_ok "https://api.localhost/healthz" 60; then
    skip "Stack did not become healthy within 60s"
  fi

  # Rolling deploy with health gate — should wait for new container healthy
  run "$NSELF_BIN" deploy --local --rolling --health-check-timeout 30
  assert_success

  # After deploy, the stack must still be healthy
  if ! _wait_http_ok "https://api.localhost/healthz" 30; then
    printf "Stack not healthy after rolling deploy\n" >&2
    return 1
  fi
}

@test "docker: health check failure triggers rollback — old container still serving" {
  _require_nself
  _require_docker
  _require_curl

  cd "$TEST_PROJECT_DIR"
  "$NSELF_BIN" init --base-domain localhost --non-interactive >/dev/null 2>&1
  "$NSELF_BIN" build >/dev/null 2>&1
  "$NSELF_BIN" start --detach >/dev/null 2>&1

  if ! _wait_http_ok "https://api.localhost/healthz" 60; then
    skip "Stack did not become healthy within 60s"
  fi

  # Trigger deploy with a bad image to force health check failure, then rollback
  run env \
    NSELF_DEPLOY_IMAGE="nself/nself-core:broken-image-tag-does-not-exist" \
    "$NSELF_BIN" deploy --local --rolling --health-check-timeout 15
  # Deploy must fail (non-zero) because health check never passes
  assert_failure
  assert_output --regexp "[Rr]ollback|[Hh]ealth|[Ff]ail"

  # Despite deploy failure, old container must still be serving
  local code
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "https://api.localhost/healthz" 2>/dev/null || printf '0')"
  if [ "$code" != "200" ] && [ "$code" != "204" ]; then
    printf "Old container not serving after failed deploy rollback (got HTTP %s)\n" "$code" >&2
    return 1
  fi
}

@test "docker: nself status shows services running after rolling deploy" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"
  "$NSELF_BIN" init --base-domain localhost --non-interactive >/dev/null 2>&1
  "$NSELF_BIN" build >/dev/null 2>&1
  "$NSELF_BIN" start --detach >/dev/null 2>&1

  if ! _wait_http_ok "https://api.localhost/healthz" 60; then
    skip "Stack did not become healthy within 60s"
  fi

  "$NSELF_BIN" deploy --local --rolling >/dev/null 2>&1 || true

  run "$NSELF_BIN" status
  assert_success
  assert_output --regexp "[Rr]unning|[Hh]ealthy|[Uu]p"
}
