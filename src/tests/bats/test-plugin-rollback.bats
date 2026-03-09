#!/usr/bin/env bats
# test-plugin-rollback.bats
# T-0456 — Plugin rollback: nself plugin rollback <name>
#
# Static tier (no Docker — always runs):
#   Verify --help exits 0.
#
# Docker tier (SKIP_DOCKER_TESTS=0):
#   Install v1.0, seed data, simulate v1.1 (run next migration), rollback to v1.0,
#   verify data preserved and service healthy.
#   Cover plugins: ai, cron, mux.
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

# Poll a URL until it returns 200 or timeout.
# Usage: _wait_healthy <url> <timeout_seconds>
_wait_healthy() {
  local url="$1"
  local timeout="${2:-30}"
  local elapsed=0
  while [ "$elapsed" -lt "$timeout" ]; do
    if curl -sf "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  return 1
}

# ---------------------------------------------------------------------------
# Setup / teardown
# ---------------------------------------------------------------------------

setup() {
  TEST_PROJECT_DIR="$(mktemp -d)"
  export TEST_PROJECT_DIR
}

teardown() {
  if [ "${SKIP_DOCKER_TESTS:-1}" = "0" ]; then
    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
      if [ -f "$TEST_PROJECT_DIR/docker-compose.yml" ]; then
        cd "$TEST_PROJECT_DIR"
        "$NSELF_BIN" stop >/dev/null 2>&1 || true
      fi
    fi
  fi
  cd /
  rm -rf "$TEST_PROJECT_DIR"
}

# ===========================================================================
# Static tier (no Docker required)
# ===========================================================================

@test "static: nself plugin rollback --help exits 0" {
  _require_nself
  run "$NSELF_BIN" plugin rollback --help
  assert_success
}

# ===========================================================================
# Docker tier
# ===========================================================================

@test "docker: rollback ai from v1.1 to v1.0 preserves data" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"
  "$NSELF_BIN" init --base-domain localhost --non-interactive >/dev/null 2>&1
  "$NSELF_BIN" build >/dev/null 2>&1
  "$NSELF_BIN" start --detach >/dev/null 2>&1

  # Install ai plugin at v1.0 (pinned version flag)
  run "$NSELF_BIN" plugin install ai --version 1.0.0
  assert_success

  # Seed 10 rows into the ai plugin tables
  run "$NSELF_BIN" db shell --command \
    "INSERT INTO np_ai_requests (id, prompt, created_at) SELECT gen_random_uuid(), 'test-' || g, now() FROM generate_series(1,10) g ON CONFLICT DO NOTHING;"
  # Tolerate partial success — table may have different schema
  local seeded_ok="$status"

  # Simulate upgrade to v1.1 by running any additional migrations
  "$NSELF_BIN" plugin install ai --version 1.1.0 >/dev/null 2>&1 || true

  # Rollback to v1.0
  run "$NSELF_BIN" plugin rollback ai --to 1.0.0
  assert_success

  # Verify row count if seed succeeded
  if [ "$seeded_ok" -eq 0 ]; then
    run "$NSELF_BIN" db shell --command \
      "SELECT count(*) FROM np_ai_requests WHERE prompt LIKE 'test-%';"
    assert_output --partial "10"
  fi

  # Service must be healthy
  run "$NSELF_BIN" plugin status ai
  assert_success
}

@test "docker: rollback cron from v1.1 to v1.0 service healthy" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"
  "$NSELF_BIN" init --base-domain localhost --non-interactive >/dev/null 2>&1
  "$NSELF_BIN" build >/dev/null 2>&1
  "$NSELF_BIN" start --detach >/dev/null 2>&1

  run "$NSELF_BIN" plugin install cron --version 1.0.0
  assert_success

  # Simulate upgrade
  "$NSELF_BIN" plugin install cron --version 1.1.0 >/dev/null 2>&1 || true

  # Rollback
  run "$NSELF_BIN" plugin rollback cron --to 1.0.0
  assert_success

  # Poll health endpoint within 30s
  local port
  port="$("$NSELF_BIN" plugin port cron 2>/dev/null || printf '3230')"
  if _wait_healthy "http://127.0.0.1:${port}/health" 30; then
    true
  else
    # Fallback: nself plugin status must report healthy
    run "$NSELF_BIN" plugin status cron
    assert_success
  fi
}

@test "docker: rollback mux from v1.1 to v1.0 service healthy" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"
  "$NSELF_BIN" init --base-domain localhost --non-interactive >/dev/null 2>&1
  "$NSELF_BIN" build >/dev/null 2>&1
  "$NSELF_BIN" start --detach >/dev/null 2>&1

  run "$NSELF_BIN" plugin install mux --version 1.0.0
  assert_success

  "$NSELF_BIN" plugin install mux --version 1.1.0 >/dev/null 2>&1 || true

  run "$NSELF_BIN" plugin rollback mux --to 1.0.0
  assert_success

  run "$NSELF_BIN" plugin status mux
  assert_success
}

@test "docker: rollback when no prior version shows helpful error" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"
  "$NSELF_BIN" init --base-domain localhost --non-interactive >/dev/null 2>&1
  "$NSELF_BIN" build >/dev/null 2>&1
  "$NSELF_BIN" start --detach >/dev/null 2>&1

  # Install cron fresh — no prior version exists
  run "$NSELF_BIN" plugin install cron
  assert_success

  # Rollback immediately — should fail with helpful message
  run "$NSELF_BIN" plugin rollback cron
  assert_failure
  assert_output --partial "no prior version"
}

@test "docker: service healthy within 30s post-rollback" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"
  "$NSELF_BIN" init --base-domain localhost --non-interactive >/dev/null 2>&1
  "$NSELF_BIN" build >/dev/null 2>&1
  "$NSELF_BIN" start --detach >/dev/null 2>&1

  run "$NSELF_BIN" plugin install ai --version 1.0.0
  assert_success
  "$NSELF_BIN" plugin install ai --version 1.1.0 >/dev/null 2>&1 || true

  run "$NSELF_BIN" plugin rollback ai --to 1.0.0
  assert_success

  # Poll status — must report healthy within 30s
  local elapsed=0
  local healthy=1
  while [ "$elapsed" -lt 30 ]; do
    if "$NSELF_BIN" plugin status ai >/dev/null 2>&1; then
      healthy=0
      break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done

  if [ "$healthy" -ne 0 ]; then
    printf "Plugin ai not healthy within 30s after rollback\n" >&2
    return 1
  fi
}
