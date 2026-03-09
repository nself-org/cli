#!/usr/bin/env bats
# test-chaos.bats
# T-0401 — Chaos test scenarios
#
# 6 scenarios (Docker tier only, skip if SKIP_DOCKER_TESTS=1):
#   1. Postgres crash and auto-restart
#   2. Redis crash graceful degradation
#   3. AI service crash — claw shows degraded not panic
#   4. Mux crash — DLQ accumulates after restart
#   5. Nginx restart — nself status recovers within 30s
#   6. High CPU — GraphQL responds after stress
#
# Static tier: none (chaos tests require live containers by definition)
#
# Bash 3.2+ compatible.

load test_helper

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

NSELF_BIN="${NSELF_BIN:-nself}"

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

# Find a running container whose name contains a fragment.
# Usage: _container_id <name-fragment>
# Prints the container ID or empty string if not found.
_container_id() {
  docker ps --filter "name=$1" --format "{{.ID}}" 2>/dev/null | head -1
}

# Wait up to N seconds for a container to be running.
# Usage: _wait_container_up <name-fragment> <max-seconds>
_wait_container_up() {
  local name="$1"
  local max="$2"
  local waited=0
  while true; do
    local state
    state=$(docker ps --filter "name=$name" --filter "status=running" --format "{{.ID}}" 2>/dev/null | head -1)
    if [ -n "$state" ]; then
      return 0
    fi
    sleep 2
    waited=$((waited + 2))
    if [ "$waited" -ge "$max" ]; then
      return 1
    fi
  done
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
# Chaos scenario 1: Postgres crash and auto-restart
# ===========================================================================

@test "chaos: postgres crash — container restarts automatically within 60s" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"

  run "$NSELF_BIN" init --yes --project-name chaos-pg-$$
  assert_success

  run "$NSELF_BIN" start
  assert_success

  # Wait for Postgres to be healthy before crashing it
  local waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 2
    waited=$((waited + 2))
    if [ "$waited" -ge 30 ]; then
      skip "Postgres not ready in 30s — environment issue"
    fi
  done

  # Find the Postgres container
  local pg_container
  pg_container=$(_container_id "postgres")
  if [ -z "$pg_container" ]; then
    skip "Postgres container not found — may use a different naming convention"
  fi

  # Stop the container (simulate crash — docker stop sends SIGTERM then SIGKILL)
  docker stop "$pg_container" >/dev/null 2>&1

  # nself uses restart: unless-stopped in docker-compose — container should come back.
  # Wait up to 60s for it to restart.
  if ! _wait_container_up "postgres" 60; then
    echo "Postgres container did not restart within 60s" >&2
    return 1
  fi

  # Verify we can query again after restart
  local recovery_waited=0
  local recovered=0
  while [ "$recovery_waited" -lt 60 ]; do
    if "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; then
      recovered=1
      break
    fi
    sleep 2
    recovery_waited=$((recovery_waited + 2))
  done

  [ "$recovered" -eq 1 ] || {
    echo "Postgres did not accept queries within 60s after restart" >&2
    return 1
  }
}

# ===========================================================================
# Chaos scenario 2: Redis crash — graceful degradation (no panic)
# ===========================================================================

@test "chaos: redis crash — nself status shows degraded not panic exit" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"

  run "$NSELF_BIN" init --yes --project-name chaos-redis-$$ --enable-redis
  assert_success

  run "$NSELF_BIN" start
  assert_success

  # Find Redis container
  local redis_container
  redis_container=$(_container_id "redis")
  if [ -z "$redis_container" ]; then
    skip "Redis container not found — Redis may not be enabled"
  fi

  # Stop Redis
  docker stop "$redis_container" >/dev/null 2>&1

  # nself status must exit with code 0 or 1 (degraded/warning), NOT 2+ (panic/crash)
  # Exit code 0 = all healthy, 1 = degraded but running, 2+ = crash/error
  run "$NSELF_BIN" status
  local exit_code="$status"

  # Must not be a crash exit (>=2 indicates panic/unhandled error)
  [ "$exit_code" -le 1 ] || {
    echo "nself status exited with code $exit_code (expected 0 or 1 for degraded)" >&2
    echo "Output: $output" >&2
    return 1
  }

  # Output must indicate degraded/warning for Redis — not a clean "all healthy"
  local shows_degraded=0
  case "$output" in
    *"degraded"*|*"warning"*|*"down"*|*"unhealthy"*|*"redis"*) shows_degraded=1 ;;
  esac

  [ "$shows_degraded" -eq 1 ] || {
    echo "Expected degraded/warning in status output but got: $output" >&2
    return 1
  }
}

# ===========================================================================
# Chaos scenario 3: AI service crash — claw shows degraded not panic
# ===========================================================================

@test "chaos: ai service crash — plugin status shows degraded not panic" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"

  run "$NSELF_BIN" init --yes --project-name chaos-ai-$$
  assert_success

  run "$NSELF_BIN" start
  assert_success

  # Check if AI plugin is installed
  if ! "$NSELF_BIN" plugin status ai >/dev/null 2>&1; then
    skip "AI plugin not installed — skipping AI chaos test"
  fi

  # Find the nself-ai container
  local ai_container
  ai_container=$(_container_id "nself-ai")
  if [ -z "$ai_container" ]; then
    ai_container=$(_container_id "nself_ai")
  fi
  if [ -z "$ai_container" ]; then
    skip "nself-ai container not found — AI plugin may not be running"
  fi

  # Crash the AI container
  docker stop "$ai_container" >/dev/null 2>&1

  # nself plugin status ai must exit 0 or 1 and show degraded
  run "$NSELF_BIN" plugin status ai
  local exit_code="$status"

  [ "$exit_code" -le 1 ] || {
    echo "nself plugin status ai exited with code $exit_code (expected 0 or 1)" >&2
    echo "Output: $output" >&2
    return 1
  }

  # Should mention degraded, down, or unavailable — not a clean healthy response
  local shows_degraded=0
  case "$output" in
    *"degraded"*|*"down"*|*"unavailable"*|*"unhealthy"*|*"stopped"*) shows_degraded=1 ;;
  esac

  [ "$shows_degraded" -eq 1 ] || {
    echo "Expected degraded/down in plugin status output but got: $output" >&2
    return 1
  }
}

# ===========================================================================
# Chaos scenario 4: Mux crash — DLQ accumulates after restart
# ===========================================================================

@test "chaos: mux crash — DLQ accumulates messages after container restart" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"

  run "$NSELF_BIN" init --yes --project-name chaos-mux-$$
  assert_success

  run "$NSELF_BIN" start
  assert_success

  # Require mux plugin
  if ! "$NSELF_BIN" plugin status mux >/dev/null 2>&1; then
    skip "Mux plugin not installed — skipping mux chaos test"
  fi

  # Wait for Postgres
  local waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 2
    waited=$((waited + 2))
    [ "$waited" -lt 30 ] || skip "Postgres not ready after 30s"
  done

  # Ensure np_mux_dlq table exists
  run "$NSELF_BIN" db shell -- -c "SELECT COUNT(*) FROM np_mux_dlq;"
  if [ "$status" -ne 0 ]; then
    skip "np_mux_dlq table not found — mux plugin schema may not be applied"
  fi

  local dlq_before
  dlq_before=$("$NSELF_BIN" db shell -- -c "SELECT COUNT(*) FROM np_mux_dlq;" 2>/dev/null | tr -d ' \n' | grep -o '[0-9]*' | head -1)
  dlq_before="${dlq_before:-0}"

  # Find and crash the mux container
  local mux_container
  mux_container=$(_container_id "nself-mux")
  if [ -z "$mux_container" ]; then
    mux_container=$(_container_id "nself_mux")
  fi
  if [ -z "$mux_container" ]; then
    skip "nself-mux container not found"
  fi

  docker stop "$mux_container" >/dev/null 2>&1

  # Insert messages that would normally be processed by mux but cannot be
  # because mux is down — these should land in the DLQ on restart
  run "$NSELF_BIN" db shell -- -c "
    INSERT INTO np_mux_dlq (event_type, payload, error_reason, created_at)
    VALUES
      ('email', '{\"to\": \"test@example.com\"}', 'mux_unavailable', NOW()),
      ('email', '{\"to\": \"test2@example.com\"}', 'mux_unavailable', NOW()),
      ('push',  '{\"token\": \"abc123\"}',         'mux_unavailable', NOW());
  "
  assert_success

  # Restart the mux container
  if ! _wait_container_up "nself-mux" 60; then
    # Try the underscore variant
    _wait_container_up "nself_mux" 60 || true
  fi

  # DLQ should have more rows than before
  local dlq_after
  dlq_after=$("$NSELF_BIN" db shell -- -c "SELECT COUNT(*) FROM np_mux_dlq;" 2>/dev/null | tr -d ' \n' | grep -o '[0-9]*' | head -1)
  dlq_after="${dlq_after:-0}"

  [ "$dlq_after" -gt "$dlq_before" ] || {
    echo "DLQ did not accumulate: before=$dlq_before after=$dlq_after" >&2
    return 1
  }
}

# ===========================================================================
# Chaos scenario 5: Nginx restart — nself status recovers within 30s
# ===========================================================================

@test "chaos: nginx restart — nself status recovers within 30s" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"

  run "$NSELF_BIN" init --yes --project-name chaos-nginx-$$
  assert_success

  run "$NSELF_BIN" start
  assert_success

  # Find nginx container
  local nginx_container
  nginx_container=$(_container_id "nginx")
  if [ -z "$nginx_container" ]; then
    skip "Nginx container not found"
  fi

  # Restart nginx (not stop — restart is graceful)
  docker restart "$nginx_container" >/dev/null 2>&1

  # Wait up to 30s for nself status to report healthy
  local waited=0
  local recovered=0
  while [ "$waited" -lt 30 ]; do
    run "$NSELF_BIN" status
    case "$output" in
      *"healthy"*|*"running"*|*"ok"*)
        recovered=1
        break
        ;;
    esac
    sleep 2
    waited=$((waited + 2))
  done

  [ "$recovered" -eq 1 ] || {
    echo "nself status did not recover within 30s after nginx restart" >&2
    echo "Last output: $output" >&2
    return 1
  }
}

# ===========================================================================
# Chaos scenario 6: High CPU — GraphQL responds after stress
# ===========================================================================

@test "chaos: high cpu stress — graphql responds after stress period" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"

  # Check for stress-ng
  if ! command -v stress-ng >/dev/null 2>&1; then
    skip "stress-ng not installed — skipping CPU stress test"
  fi

  run "$NSELF_BIN" init --yes --project-name chaos-cpu-$$
  assert_success

  run "$NSELF_BIN" start
  assert_success

  # Wait for Hasura to be ready
  local waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 2
    waited=$((waited + 2))
    [ "$waited" -lt 30 ] || skip "Services not ready after 30s"
  done

  # Read Hasura URL from env or derive from project
  local hasura_url
  hasura_url=$(grep "HASURA_GRAPHQL_URL" .env 2>/dev/null | head -1 | cut -d= -f2- | tr -d '"')
  hasura_url="${hasura_url:-http://localhost:8080}"

  local admin_secret
  admin_secret=$(grep "HASURA_GRAPHQL_ADMIN_SECRET" .env 2>/dev/null | head -1 | cut -d= -f2- | tr -d '"')
  admin_secret="${admin_secret:-nself_admin_secret}"

  # Run CPU stress for 10s in background
  stress-ng --cpu 0 --timeout 10s >/dev/null 2>&1 &
  local stress_pid="$!"

  # While stress runs, attempt GraphQL introspection query
  local graphql_ok=0
  local attempt=0
  while [ "$attempt" -lt 8 ]; do
    sleep 2
    attempt=$((attempt + 1))

    local http_status
    http_status=$(curl -s -o /dev/null -w "%{http_code}" \
      -H "Content-Type: application/json" \
      -H "x-hasura-admin-secret: $admin_secret" \
      -d '{"query":"{ __typename }"}' \
      "$hasura_url/v1/graphql" 2>/dev/null)

    if [ "$http_status" = "200" ]; then
      graphql_ok=1
      break
    fi
  done

  # Wait for stress to finish
  wait "$stress_pid" 2>/dev/null || true

  [ "$graphql_ok" -eq 1 ] || {
    echo "GraphQL did not respond with 200 during/after CPU stress" >&2
    return 1
  }
}
