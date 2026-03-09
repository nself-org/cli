#!/usr/bin/env bats
# test-dr-drill.bats
# T-0403 — Disaster Recovery drill
#
# 8-step DR drill (Docker tier, SKIP_DOCKER_TESTS=1 default):
#
# Static tier (always runs):
#   - nself backup --help exits 0
#   - nself restore --help exits 0
#
# Docker tier — full DR cycle (8 steps):
#   1.  Init + start services
#   2.  Populate 50 rows of test data
#   3.  Run nself backup → capture backup file path
#   4.  Verify backup file is non-empty and contains expected table
#   5.  Copy backup to a simulated "new server" temp dir
#   6.  Init a fresh nself project (simulates new server)
#   7.  nself restore <backup> on the fresh project
#   8.  Verify 50 rows intact + nself status healthy
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
  TEST_RESTORE_DIR="$(mktemp -d)"
  export TEST_PROJECT_DIR TEST_RESTORE_DIR
}

teardown() {
  if [ "${SKIP_DOCKER_TESTS:-1}" = "0" ]; then
    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
      for dir in "$TEST_PROJECT_DIR" "$TEST_RESTORE_DIR"; do
        if [ -f "$dir/docker-compose.yml" ]; then
          cd "$dir"
          "$NSELF_BIN" stop >/dev/null 2>&1 || true
        fi
      done
    fi
  fi
  cd /
  rm -rf "$TEST_PROJECT_DIR" "$TEST_RESTORE_DIR"
}

# ===========================================================================
# Static tier — no Docker required
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

# ===========================================================================
# Docker tier — full 8-step DR drill
# ===========================================================================

@test "docker: full 8-step DR drill — backup, copy, restore, verify" {
  _require_nself
  _require_docker

  # ------------------------------------------------------------------
  # Step 1: Init + start on "original server"
  # ------------------------------------------------------------------
  cd "$TEST_PROJECT_DIR"

  run "$NSELF_BIN" init --yes --project-name dr-source-$$
  assert_success

  run "$NSELF_BIN" start
  assert_success

  # Wait for Postgres (up to 60s)
  local waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 2
    waited=$((waited + 2))
    if [ "$waited" -ge 60 ]; then
      skip "Postgres not ready after 60s — environment issue"
    fi
  done

  # ------------------------------------------------------------------
  # Step 2: Populate 50 rows of test data
  # ------------------------------------------------------------------
  run "$NSELF_BIN" db shell -- -c "
    CREATE TABLE IF NOT EXISTS _dr_drill_data (
      id    SERIAL PRIMARY KEY,
      label TEXT NOT NULL,
      ts    TIMESTAMPTZ DEFAULT NOW()
    );
    INSERT INTO _dr_drill_data (label)
    SELECT 'dr_row_' || g
    FROM generate_series(1, 50) g;
  "
  assert_success

  run "$NSELF_BIN" db shell -- -c "SELECT COUNT(*) FROM _dr_drill_data;"
  assert_success
  assert_output --partial "50"

  # ------------------------------------------------------------------
  # Step 3: Run backup — capture the backup file path
  # ------------------------------------------------------------------
  run "$NSELF_BIN" backup
  assert_success

  local backup_file
  backup_file=$(ls -t "$TEST_PROJECT_DIR/.volumes/backups/"*.sql.gz 2>/dev/null | head -1)
  if [ -z "$backup_file" ]; then
    backup_file=$(ls -t "$TEST_PROJECT_DIR/backups/"*.sql.gz 2>/dev/null | head -1 || true)
  fi

  [ -n "$backup_file" ] || {
    echo "No backup file found after nself backup" >&2
    return 1
  }

  # ------------------------------------------------------------------
  # Step 4: Verify backup file is non-empty and contains expected table
  # ------------------------------------------------------------------
  local backup_size
  backup_size=$(ls -la "$backup_file" 2>/dev/null | awk '{print $5}')
  [ "${backup_size:-0}" -gt 0 ] || {
    echo "Backup file is empty: $backup_file" >&2
    return 1
  }

  run sh -c "gunzip -c '$backup_file' | grep -q '_dr_drill_data'"
  assert_success

  # ------------------------------------------------------------------
  # Step 5: Copy backup to simulated "new server" temp dir
  # ------------------------------------------------------------------
  mkdir -p "$TEST_RESTORE_DIR/backups"
  cp "$backup_file" "$TEST_RESTORE_DIR/backups/"

  local restore_backup
  restore_backup="$TEST_RESTORE_DIR/backups/$(basename "$backup_file")"

  [ -f "$restore_backup" ] || {
    echo "Failed to copy backup to restore dir" >&2
    return 1
  }

  # ------------------------------------------------------------------
  # Step 6: Init a fresh nself project (simulates new server)
  # ------------------------------------------------------------------
  cd "$TEST_RESTORE_DIR"

  run "$NSELF_BIN" init --yes --project-name dr-restore-$$
  assert_success

  run "$NSELF_BIN" start
  assert_success

  # Wait for fresh Postgres to be ready
  local restore_waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 2
    restore_waited=$((restore_waited + 2))
    if [ "$restore_waited" -ge 60 ]; then
      skip "Fresh Postgres not ready after 60s"
    fi
  done

  # Confirm the test table does NOT exist on the fresh instance
  run "$NSELF_BIN" db shell -- -c "SELECT COUNT(*) FROM _dr_drill_data;"
  assert_failure

  # ------------------------------------------------------------------
  # Step 7: Restore on fresh project
  # ------------------------------------------------------------------
  run "$NSELF_BIN" restore "$restore_backup"
  assert_success

  # ------------------------------------------------------------------
  # Step 8a: Verify 50 rows intact
  # ------------------------------------------------------------------
  run "$NSELF_BIN" db shell -- -c "SELECT COUNT(*) FROM _dr_drill_data;"
  assert_success
  assert_output --partial "50"

  # Step 8b: Verify specific row content
  run "$NSELF_BIN" db shell -- -c "SELECT label FROM _dr_drill_data WHERE id = 1;"
  assert_success
  assert_output --partial "dr_row_1"

  # Step 8c: Health check
  run "$NSELF_BIN" status
  assert_success

  # Step 8d: DB migrate should be idempotent after restore
  run "$NSELF_BIN" db migrate
  assert_success
}

@test "docker: DR restore completes within RTO target of 20 minutes" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"

  run "$NSELF_BIN" init --yes --project-name dr-rto-$$
  assert_success

  run "$NSELF_BIN" start
  assert_success

  local waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 2
    waited=$((waited + 2))
    [ "$waited" -lt 60 ] || skip "Postgres not ready"
  done

  # Create a modest dataset
  run "$NSELF_BIN" db shell -- -c "
    CREATE TABLE IF NOT EXISTS _rto_data (id SERIAL PRIMARY KEY, val TEXT);
    INSERT INTO _rto_data (val)
    SELECT md5(g::text) FROM generate_series(1, 200) g;
  "
  assert_success

  run "$NSELF_BIN" backup
  assert_success

  local backup_file
  backup_file=$(ls -t .volumes/backups/*.sql.gz 2>/dev/null | head -1)
  [ -n "$backup_file" ] || skip "No backup file found"

  # Start timing: includes stop, destroy data, restore cycle
  local start_ts
  start_ts=$(date +%s)

  # Simulate disaster — drop all data
  run "$NSELF_BIN" db shell -- -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
  assert_success

  # Restore
  run "$NSELF_BIN" restore "$backup_file"
  assert_success

  local end_ts
  end_ts=$(date +%s)
  local elapsed=$((end_ts - start_ts))

  # RTO target: < 20 minutes = 1200 seconds (for a real prod dataset this
  # covers nself init + restore; this test uses a minimal dataset so
  # the real timer is the full restore cycle overhead)
  [ "$elapsed" -lt 1200 ] || {
    echo "DR restore took ${elapsed}s, exceeds 20-minute RTO target" >&2
    return 1
  }

  # Verify data returned
  run "$NSELF_BIN" db shell -- -c "SELECT COUNT(*) FROM _rto_data;"
  assert_success
  assert_output --partial "200"
}
