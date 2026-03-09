#!/usr/bin/env bats
# test-migrations.bats
# T-0433 — Migrations: up/down test for every migration file
#
# Wraps test-migrations.sh as bats tests.
# Runs against Docker Postgres fixture.
#
# Static tier (always runs):
#   - test-migrations.sh exists and is executable
#   - Migrations directory exists with at least 1 migration
#
# Docker tier (requires SKIP_DOCKER_TESTS=0):
#   - Core migrations batch: UP/DOWN round-trip
#   - Plugin migrations batch: UP/DOWN round-trip
#   - Missing DOWN file count is within acceptable threshold

load test_helper

NSELF_BIN="${NSELF_BIN:-nself}"
MIGRATIONS_SCRIPT="${BATS_TEST_DIRNAME}/../../tests/migrations/test-migrations.sh"
PG_HOST="${POSTGRES_HOST:-127.0.0.1}"
PG_PORT="${POSTGRES_PORT:-5432}"
PG_USER="${POSTGRES_USER:-postgres}"
PG_DB="${POSTGRES_DB:-nself}"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_require_docker() {
  if [ "${SKIP_DOCKER_TESTS:-1}" != "0" ]; then
    skip "SKIP_DOCKER_TESTS is not 0 — Docker tier skipped"
  fi
  if ! command -v docker >/dev/null 2>&1; then
    skip "docker not available in this environment"
  fi
  if ! docker info >/dev/null 2>&1; then
    skip "Docker daemon not running"
  fi
}

_require_psql() {
  if ! command -v psql >/dev/null 2>&1; then
    skip "psql not available"
  fi
}

_pgconn() {
  printf 'postgresql://%s:%s@%s:%s/%s' \
    "$PG_USER" \
    "${POSTGRES_PASSWORD:-}" \
    "$PG_HOST" \
    "$PG_PORT" \
    "$PG_DB"
}

_find_migrations_dir() {
  # Try common locations
  for candidate in \
    "${BATS_TEST_DIRNAME}/../../../../migrations" \
    "${BATS_TEST_DIRNAME}/../../../migrations" \
    "${BATS_TEST_DIRNAME}/../../migrations" \
    "/opt/nself/migrations" \
    "${HOME}/.nself/migrations"; do
    if [ -d "$candidate" ]; then
      printf '%s' "$candidate"
      return 0
    fi
  done
  return 1
}

# ---------------------------------------------------------------------------
# Static tier — script exists
# ---------------------------------------------------------------------------

@test "test-migrations.sh exists" {
  if [ ! -f "$MIGRATIONS_SCRIPT" ]; then
    printf "Missing: %s\n" "$MIGRATIONS_SCRIPT" >&2
    return 1
  fi
}

@test "test-migrations.sh is executable" {
  if [ ! -x "$MIGRATIONS_SCRIPT" ]; then
    printf "Not executable: %s\n" "$MIGRATIONS_SCRIPT" >&2
    return 1
  fi
}

@test "test-migrations.sh --help exits 0" {
  run "$MIGRATIONS_SCRIPT" --help
  assert_success
}

# ---------------------------------------------------------------------------
# Static tier — migrations directory check
# ---------------------------------------------------------------------------

@test "migrations directory contains at least 1 SQL file" {
  migrations_dir=$(_find_migrations_dir 2>/dev/null || printf "")
  if [ -z "$migrations_dir" ]; then
    skip "migrations directory not found — skipping directory check"
  fi

  # Count SQL files (not .down.sql)
  count=0
  for f in "$migrations_dir"/*.sql; do
    if [ -f "$f" ]; then
      case "$f" in
        *.down.sql|*_down.sql) continue ;;
      esac
      count=$((count + 1))
    fi
  done

  if [ "$count" -eq 0 ]; then
    printf "No UP migration SQL files found in: %s\n" "$migrations_dir" >&2
    return 1
  fi
}

# ---------------------------------------------------------------------------
# Docker tier — core migrations: UP/DOWN round-trip
# ---------------------------------------------------------------------------

@test "docker: core migrations pass UP/DOWN round-trip test" {
  _require_docker
  _require_psql

  if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q 'postgres\|nself-postgres'; then
    skip "postgres container not running"
  fi

  migrations_dir=$(_find_migrations_dir 2>/dev/null || printf "")
  if [ -z "$migrations_dir" ]; then
    skip "migrations directory not found"
  fi

  # Run test-migrations.sh for core migrations
  run bash "$MIGRATIONS_SCRIPT" --migrations-dir "$migrations_dir" --verbose
  assert_success
}

@test "docker: plugin migrations pass UP/DOWN round-trip test" {
  _require_docker
  _require_psql

  if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q 'postgres\|nself-postgres'; then
    skip "postgres container not running"
  fi

  # Look for plugin-specific migrations directory
  plugin_migrations_dir=""
  for candidate in \
    "${BATS_TEST_DIRNAME}/../../../../plugins-pro/migrations" \
    "${BATS_TEST_DIRNAME}/../../../plugins-pro/migrations" \
    "${HOME}/.nself/plugins/migrations"; do
    if [ -d "$candidate" ]; then
      plugin_migrations_dir="$candidate"
      break
    fi
  done

  if [ -z "$plugin_migrations_dir" ]; then
    skip "plugin migrations directory not found"
  fi

  run bash "$MIGRATIONS_SCRIPT" --migrations-dir "$plugin_migrations_dir" --verbose
  assert_success
}

@test "docker: nself db migrate exits 0 on clean instance" {
  _require_docker
  _require_psql

  if ! command -v "$NSELF_BIN" >/dev/null 2>&1; then
    skip "nself not installed"
  fi

  if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q 'postgres\|nself-postgres'; then
    skip "postgres container not running"
  fi

  run "$NSELF_BIN" db migrate
  assert_success
}

@test "docker: nself db migrate is idempotent (run twice, still exits 0)" {
  _require_docker
  _require_psql

  if ! command -v "$NSELF_BIN" >/dev/null 2>&1; then
    skip "nself not installed"
  fi

  if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q 'postgres\|nself-postgres'; then
    skip "postgres container not running"
  fi

  # Run once
  run "$NSELF_BIN" db migrate
  assert_success

  # Run again — should be a no-op, not an error
  run "$NSELF_BIN" db migrate
  assert_success
}

@test "docker: missing DOWN file count is within threshold (< 20% of total)" {
  _require_docker
  _require_psql

  migrations_dir=$(_find_migrations_dir 2>/dev/null || printf "")
  if [ -z "$migrations_dir" ]; then
    skip "migrations directory not found"
  fi

  # Count UP files
  total_up=0
  for f in "$migrations_dir"/*.sql; do
    if [ ! -f "$f" ]; then continue; fi
    case "$f" in
      *.down.sql|*_down.sql) continue ;;
    esac
    total_up=$((total_up + 1))
  done

  if [ "$total_up" -eq 0 ]; then
    skip "no UP migration files found"
  fi

  # Count DOWN files
  total_down=0
  for f in "$migrations_dir"/*.down.sql "$migrations_dir"/*_down.sql; do
    if [ -f "$f" ]; then
      total_down=$((total_down + 1))
    fi
  done

  missing=$((total_up - total_down))
  if [ "$missing" -lt 0 ]; then
    missing=0
  fi

  # Threshold: < 20% missing
  threshold=$((total_up / 5))
  if [ "$missing" -gt "$threshold" ]; then
    printf "Too many missing DOWN files: %s/%s (threshold: %s)\n" \
      "$missing" "$total_up" "$threshold" >&2
    return 1
  fi
}
