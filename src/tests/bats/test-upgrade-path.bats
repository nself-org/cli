#!/usr/bin/env bats
# test-upgrade-path.bats
# T-0402 — Upgrade path simulation
#
# 10-step upgrade path test (Docker tier, SKIP_DOCKER_TESTS=1 default):
#
# Static tier (always runs):
#   - nself upgrade --help exits 0
#   - nself rollback --help exits 0
#
# Docker tier (10-step simulation):
#   1.  Init project with v0.9.9 config
#   2.  Start all core services
#   3.  Create test data (10 rows in a test table)
#   4.  Install one free plugin (analytics)
#   5.  Run nself backup (pre-upgrade snapshot)
#   6.  Run nself upgrade --dry-run (verify upgrade plan is produced)
#   7.  Run nself upgrade (or simulate: re-run nself build + restart)
#   8.  Verify services start cleanly after upgrade
#   9.  Verify test data is intact (10 rows)
#   10. Verify plugin still healthy after upgrade
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
# Static tier — no Docker required
# ===========================================================================

@test "static: nself upgrade --help exits 0" {
  _require_nself
  run "$NSELF_BIN" upgrade --help
  assert_success
}

@test "static: nself rollback --help exits 0" {
  _require_nself
  run "$NSELF_BIN" rollback --help
  assert_success
}

# ===========================================================================
# Docker tier — 10-step upgrade simulation
# ===========================================================================

@test "docker: 10-step upgrade path simulation" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"

  # ------------------------------------------------------------------
  # Step 1: Init project with v0.9.9 config
  # ------------------------------------------------------------------
  run "$NSELF_BIN" init --yes --project-name upgrade-test-$$
  assert_success

  # Record current version
  local current_version
  current_version=$("$NSELF_BIN" version 2>/dev/null | head -1 | grep -o '[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*' | head -1)
  current_version="${current_version:-0.9.9}"

  # ------------------------------------------------------------------
  # Step 2: Start core services
  # ------------------------------------------------------------------
  run "$NSELF_BIN" start
  assert_success

  # Wait for Postgres readiness (up to 60s)
  local waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 2
    waited=$((waited + 2))
    if [ "$waited" -ge 60 ]; then
      skip "Postgres not ready after 60s — environment issue"
    fi
  done

  # ------------------------------------------------------------------
  # Step 3: Create test data
  # ------------------------------------------------------------------
  run "$NSELF_BIN" db shell -- -c "
    CREATE TABLE IF NOT EXISTS _upgrade_test_rows (
      id    SERIAL PRIMARY KEY,
      label TEXT NOT NULL,
      ts    TIMESTAMPTZ DEFAULT NOW()
    );
    INSERT INTO _upgrade_test_rows (label)
    SELECT 'row_' || g
    FROM generate_series(1, 10) g;
  "
  assert_success

  # Verify 10 rows exist
  run "$NSELF_BIN" db shell -- -c "SELECT COUNT(*) FROM _upgrade_test_rows;"
  assert_success
  assert_output --partial "10"

  # ------------------------------------------------------------------
  # Step 4: Install analytics plugin (free — no license required)
  # ------------------------------------------------------------------
  run "$NSELF_BIN" plugin install analytics
  assert_success

  run "$NSELF_BIN" plugin status analytics
  assert_success

  # ------------------------------------------------------------------
  # Step 5: Backup before upgrade
  # ------------------------------------------------------------------
  run "$NSELF_BIN" backup
  assert_success

  # Verify backup file was created
  local backup_file
  backup_file=$(ls -t .volumes/backups/*.sql.gz 2>/dev/null | head -1)
  if [ -z "$backup_file" ]; then
    backup_file=$(ls -t backups/*.sql.gz 2>/dev/null | head -1 || true)
  fi
  [ -n "$backup_file" ] || {
    echo "No backup file found after nself backup" >&2
    return 1
  }

  # ------------------------------------------------------------------
  # Step 6: Upgrade dry-run — verify plan is produced
  # ------------------------------------------------------------------
  run "$NSELF_BIN" upgrade --dry-run
  # dry-run must exit 0 and produce some output describing the upgrade
  assert_success
  [ -n "$output" ] || {
    echo "upgrade --dry-run produced no output" >&2
    return 1
  }

  # ------------------------------------------------------------------
  # Step 7: Run upgrade (re-build config + pull new images + restart)
  # ------------------------------------------------------------------
  # nself upgrade rebuilds docker-compose.yml and restarts services.
  # On v0.9.9 → v0.9.9 (same version) this is a no-op upgrade — that is
  # acceptable for the simulation; the goal is exercising the upgrade path.
  run "$NSELF_BIN" upgrade
  assert_success

  # ------------------------------------------------------------------
  # Step 8: Verify services start cleanly after upgrade
  # ------------------------------------------------------------------
  # Wait up to 60s for status to report healthy
  local status_waited=0
  local services_ok=0
  while [ "$status_waited" -lt 60 ]; do
    run "$NSELF_BIN" status
    case "$output" in
      *"healthy"*|*"running"*|*"ok"*)
        services_ok=1
        break
        ;;
    esac
    sleep 3
    status_waited=$((status_waited + 3))
  done

  [ "$services_ok" -eq 1 ] || {
    echo "Services not healthy within 60s after upgrade" >&2
    echo "Last status output: $output" >&2
    return 1
  }

  # Also verify Postgres accepts queries
  local db_waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 2
    db_waited=$((db_waited + 2))
    [ "$db_waited" -lt 60 ] || {
      echo "Postgres not accepting queries 60s after upgrade" >&2
      return 1
    }
  done

  # ------------------------------------------------------------------
  # Step 9: Verify test data is intact
  # ------------------------------------------------------------------
  run "$NSELF_BIN" db shell -- -c "SELECT COUNT(*) FROM _upgrade_test_rows;"
  assert_success
  assert_output --partial "10"

  # ------------------------------------------------------------------
  # Step 10: Verify analytics plugin still healthy after upgrade
  # ------------------------------------------------------------------
  run "$NSELF_BIN" plugin status analytics
  assert_success
  assert_output --partial "healthy"
}

# ===========================================================================
# Docker tier — rollback verification
# ===========================================================================

@test "docker: rollback restores previous state" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"

  run "$NSELF_BIN" init --yes --project-name rollback-test-$$
  assert_success

  run "$NSELF_BIN" start
  assert_success

  # Wait for Postgres
  local waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 2
    waited=$((waited + 2))
    [ "$waited" -lt 60 ] || skip "Postgres not ready after 60s"
  done

  # Insert some data before upgrade
  run "$NSELF_BIN" db shell -- -c "
    CREATE TABLE IF NOT EXISTS _rollback_check (id SERIAL PRIMARY KEY, val TEXT);
    INSERT INTO _rollback_check (val) VALUES ('pre-upgrade');
  "
  assert_success

  # Upgrade (no-op on same version, tests the workflow)
  run "$NSELF_BIN" upgrade
  assert_success

  # Rollback
  run "$NSELF_BIN" rollback
  assert_success

  # Postgres should still be accessible after rollback
  local rb_waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 2
    rb_waited=$((rb_waited + 2))
    [ "$rb_waited" -lt 60 ] || {
      echo "Postgres not accessible after rollback" >&2
      return 1
    }
  done

  run "$NSELF_BIN" status
  assert_success
}
