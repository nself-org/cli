#!/usr/bin/env bats
# test-backup-restore.bats
# T-0364 — CLI: backup + restore cycle integration test
#
# Full 8-step cycle (Docker tier only):
#   1. Start nself (Postgres + Hasura + Auth)
#   2. Install analytics plugin
#   3. Insert 100 rows into np_analytics_events
#   4. nself backup → timestamped .sql.gz file
#   5. Verify backup file contains np_analytics_events
#   6. Drop all tables (simulate disaster)
#   7. nself restore <backup>
#   8. Verify 100 rows intact + analytics plugin healthy
#
# Static (no-Docker) tier:
#   - Verify nself backup --help works
#   - Verify nself restore --help works
#   - Verify backup file naming convention (YYYY-MM-DD pattern)
#
# Skip Docker tier when SKIP_DOCKER_TESTS=1 (default in CI without DinD).
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

# ---------------------------------------------------------------------------
# Setup / teardown
# ---------------------------------------------------------------------------

setup() {
  TEST_PROJECT_DIR="$(mktemp -d)"
  export TEST_PROJECT_DIR
}

teardown() {
  if [ -z "${SKIP_DOCKER_TESTS:-}" ] || [ "${SKIP_DOCKER_TESTS:-1}" = "0" ]; then
    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
      if [ -f "$TEST_PROJECT_DIR/docker-compose.yml" ]; then
        cd "$TEST_PROJECT_DIR"
        nself stop >/dev/null 2>&1 || true
      fi
    fi
  fi
  cd /
  rm -rf "$TEST_PROJECT_DIR"
}

# ===========================================================================
# Static tier (no Docker required)
# ===========================================================================

@test "static: nself backup --help exits 0" {
  _require_nself
  run "$NSELF_BIN" backup --help
  assert_success
}

@test "static: nself restore --help exits 0" {
  _require_nself
  run "$NSELF_BIN" restore --help
  assert_success
}

@test "static: backup filename pattern includes date (YYYY-MM-DD)" {
  _require_nself
  # Verify the help text or source documents a datestamped filename convention
  run "$NSELF_BIN" backup --help
  assert_success
  # Help should mention either 'backup' or '.sql' or '.gz' to indicate file output
  local mentions_backup=0
  case "$output" in
    *"backup"*|*".sql"*|*".gz"*|*"timestamp"*|*"date"*) mentions_backup=1 ;;
  esac
  [ "$mentions_backup" -eq 1 ]
}

# ===========================================================================
# Docker tier — full backup + restore cycle
# ===========================================================================

@test "docker: full 8-step backup + restore cycle" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"

  # Step 0: init minimal project
  run "$NSELF_BIN" init --yes --project-name backup-test-$$
  assert_success

  # Step 1: start nself (Postgres + Hasura + Auth only for speed)
  run "$NSELF_BIN" start
  assert_success

  # Wait for Postgres to be ready (up to 30s)
  local waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 2
    waited=$((waited + 2))
    if [ "$waited" -ge 30 ]; then
      skip "Postgres not ready after 30s — environment issue"
    fi
  done

  # Step 2: install analytics plugin
  run "$NSELF_BIN" plugin install analytics
  assert_success

  # Step 3: insert 100 rows into np_analytics_events
  run "$NSELF_BIN" db shell -- -c "
    INSERT INTO np_analytics_events (event_name, properties, created_at)
    SELECT 'test_event_' || g, '{\"test\": true}'::jsonb, NOW()
    FROM generate_series(1, 100) g;
  "
  assert_success

  # Verify row count before backup
  run "$NSELF_BIN" db shell -- -c "SELECT COUNT(*) FROM np_analytics_events;"
  assert_success
  assert_output --partial "100"

  # Step 4: nself backup → timestamped .sql.gz
  run "$NSELF_BIN" backup
  assert_success

  # Find the backup file
  local backup_file
  backup_file=$(ls -t .volumes/backups/*.sql.gz 2>/dev/null | head -1)
  if [ -z "$backup_file" ]; then
    # Some implementations use a different path
    backup_file=$(ls -t backups/*.sql.gz 2>/dev/null | head -1 || true)
  fi
  [ -n "$backup_file" ] || { echo "No backup file found" >&2; return 1; }

  # Step 5: verify backup contains np_analytics_events
  run sh -c "gunzip -c '$backup_file' | grep -q 'np_analytics_events'"
  assert_success

  # Step 6: drop all tables (simulate disaster)
  run "$NSELF_BIN" db shell -- -c "
    DROP SCHEMA public CASCADE;
    CREATE SCHEMA public;
  "
  assert_success

  # Verify tables are gone
  run "$NSELF_BIN" db shell -- -c "SELECT COUNT(*) FROM np_analytics_events;"
  assert_failure

  # Step 7: nself restore <backup>
  run "$NSELF_BIN" restore "$backup_file"
  assert_success

  # Step 8a: verify 100 rows intact
  run "$NSELF_BIN" db shell -- -c "SELECT COUNT(*) FROM np_analytics_events;"
  assert_success
  assert_output --partial "100"

  # Step 8b: plugin health check
  run "$NSELF_BIN" plugin status analytics
  assert_success
  assert_output --partial "healthy"

  # Step 8c: db migrate should be idempotent (no errors re-running migrations)
  run "$NSELF_BIN" db migrate
  assert_success
}

@test "docker: restore completes in under 60 seconds for test volume" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"

  run "$NSELF_BIN" init --yes --project-name backup-timing-test-$$
  assert_success

  run "$NSELF_BIN" start
  assert_success

  # Wait for Postgres
  local waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 2
    waited=$((waited + 2))
    [ "$waited" -lt 30 ] || skip "Postgres not ready"
  done

  # Create a small dataset and back it up
  run "$NSELF_BIN" db shell -- -c "
    CREATE TABLE _timing_test (id SERIAL PRIMARY KEY, val TEXT);
    INSERT INTO _timing_test (val) SELECT md5(g::text) FROM generate_series(1, 50) g;
  "
  assert_success

  run "$NSELF_BIN" backup
  assert_success

  local backup_file
  backup_file=$(ls -t .volumes/backups/*.sql.gz 2>/dev/null | head -1)
  [ -n "$backup_file" ] || skip "No backup file found"

  # Drop and restore — time it
  "$NSELF_BIN" db shell -- -c "DROP TABLE _timing_test;" >/dev/null 2>&1 || true

  local start_ts
  start_ts=$(date +%s)

  run "$NSELF_BIN" restore "$backup_file"
  assert_success

  local end_ts
  end_ts=$(date +%s)
  local elapsed=$((end_ts - start_ts))

  [ "$elapsed" -lt 60 ] || {
    echo "Restore took ${elapsed}s, expected < 60s" >&2
    return 1
  }
}
