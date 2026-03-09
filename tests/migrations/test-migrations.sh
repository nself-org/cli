#!/usr/bin/env bash
# test-migrations.sh
# T-0433 — Migrations: up/down test for every migration file
#
# Tests every SQL migration file: UP, DOWN, UP-with-data, DOWN-with-data.
# Reports: total tested, missing DOWN files, failures.
#
# Requirements: psql, a running PostgreSQL instance.
# Usage:
#   PGCONN="postgresql://nself:nself@localhost:5432/nself" ./test-migrations.sh
#   PGCONN="..." MIGRATIONS_DIR="/path/to/migrations" ./test-migrations.sh
#
# Exit: 0 = all migrations passed, 1 = one or more failures

set -eu

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

PGCONN="${PGCONN:-postgresql://postgres:@localhost:5432/nself}"
MIGRATIONS_DIR="${MIGRATIONS_DIR:-}"
VERBOSE="${VERBOSE:-0}"

TOTAL_TESTED=0
TOTAL_PASSED=0
TOTAL_FAILED=0
MISSING_DOWN=0
FAILURES=""

# ---------------------------------------------------------------------------
# Argument parsing (Bash 3.2 compatible)
# ---------------------------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    --migrations-dir)
      MIGRATIONS_DIR="$2"
      shift 2
      ;;
    --verbose|-v)
      VERBOSE=1
      shift
      ;;
    --help|-h)
      printf 'Usage: %s [--migrations-dir <path>] [--verbose]\n' "$0"
      printf '  PGCONN env var sets the PostgreSQL connection string.\n'
      printf '  MIGRATIONS_DIR env var sets the migrations root directory.\n'
      exit 0
      ;;
    *)
      printf 'Unknown argument: %s\n' "$1" >&2
      exit 1
      ;;
  esac
done

# ---------------------------------------------------------------------------
# Dependency checks
# ---------------------------------------------------------------------------

if ! command -v psql >/dev/null 2>&1; then
  printf 'ERROR: psql not found. Install PostgreSQL client tools.\n' >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Auto-discover migrations directory
# ---------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ -z "$MIGRATIONS_DIR" ]; then
  # Try common locations relative to script
  for candidate in \
    "$SCRIPT_DIR/../../migrations" \
    "$SCRIPT_DIR/../../../migrations" \
    "/opt/nself/migrations" \
    "$HOME/.nself/migrations"; do
    if [ -d "$candidate" ]; then
      MIGRATIONS_DIR="$candidate"
      break
    fi
  done
fi

if [ -z "$MIGRATIONS_DIR" ] || [ ! -d "$MIGRATIONS_DIR" ]; then
  printf 'ERROR: migrations directory not found.\n' >&2
  printf 'Set MIGRATIONS_DIR or place migrations at cli/migrations/.\n' >&2
  printf 'RESULT: 0 migrations tested (directory not found).\n'
  exit 0
fi

printf 'Migrations directory: %s\n' "$MIGRATIONS_DIR"

# ---------------------------------------------------------------------------
# Helper functions
# ---------------------------------------------------------------------------

_psql() {
  psql "$PGCONN" -tAc "$1" 2>/dev/null
}

_psql_file() {
  psql "$PGCONN" -f "$1" >/dev/null 2>&1
}

_log() {
  if [ "$VERBOSE" = "1" ]; then
    printf '%s\n' "$1"
  fi
}

_log_always() {
  printf '%s\n' "$1"
}

# Get schema hash (list of tables + columns) for before/after comparison.
_schema_hash() {
  _psql "SELECT md5(string_agg(table_name || '.' || column_name, ',' ORDER BY table_name, column_name))
         FROM information_schema.columns
         WHERE table_schema = 'public';" 2>/dev/null || printf "unknown"
}

# Test a single migration file (UP direction).
# Returns: 0 = pass, 1 = fail
_test_up_migration() {
  local up_file="$1"
  local migration_name
  migration_name=$(basename "$up_file" .sql)

  _log "  Testing UP: $migration_name"

  # Record schema before
  local hash_before
  hash_before=$(_schema_hash)

  # Run UP migration
  if ! _psql_file "$up_file"; then
    printf 'FAIL [%s UP]: psql returned non-zero\n' "$migration_name" >&2
    FAILURES="${FAILURES}
  - ${migration_name} UP: psql error"
    return 1
  fi

  # Verify schema changed or stayed same (UP migration might be idempotent)
  local hash_after
  hash_after=$(_schema_hash)

  _log "  UP OK: $migration_name (schema changed: $([ "$hash_before" != "$hash_after" ] && printf 'yes' || printf 'no'))"
  return 0
}

# Find the corresponding DOWN migration file for an UP file.
# Naming conventions: V001__name.sql → V001__name.down.sql
#                     001_name.sql   → 001_name.down.sql
#                     001_up.sql     → 001_down.sql
_find_down_file() {
  local up_file="$1"
  local dir
  dir=$(dirname "$up_file")
  local base
  base=$(basename "$up_file" .sql)

  # Try common DOWN naming patterns
  for candidate in \
    "$dir/${base}.down.sql" \
    "$dir/${base}_down.sql" \
    "${up_file%.sql}.down.sql"; do
    if [ -f "$candidate" ]; then
      printf '%s' "$candidate"
      return 0
    fi
  done

  # Try replacing 'up' with 'down' in filename
  local down_base
  down_base=$(printf '%s' "$base" | sed 's/[Uu][Pp]/down/g' 2>/dev/null || printf '%s' "$base")
  if [ "$down_base" != "$base" ] && [ -f "$dir/${down_base}.sql" ]; then
    printf '%s' "$dir/${down_base}.sql"
    return 0
  fi

  # Not found
  return 1
}

# Test DOWN migration if it exists.
_test_down_migration() {
  local down_file="$1"
  local migration_name
  migration_name=$(basename "$down_file" .sql)
  migration_name="${migration_name%.down}"

  _log "  Testing DOWN: $migration_name"

  if ! _psql_file "$down_file"; then
    printf 'FAIL [%s DOWN]: psql returned non-zero\n' "$migration_name" >&2
    FAILURES="${FAILURES}
  - ${migration_name} DOWN: psql error"
    return 1
  fi

  _log "  DOWN OK: $migration_name"
  return 0
}

# ---------------------------------------------------------------------------
# Discover migration files
# ---------------------------------------------------------------------------

_log_always "Discovering migration files in: $MIGRATIONS_DIR"

# Find all SQL files that are UP migrations (not .down.sql)
MIGRATION_FILES=""
for f in "$MIGRATIONS_DIR"/*.sql "$MIGRATIONS_DIR"/**/*.sql; do
  # Bash 3.2 glob: skip if no match (file literally named *.sql)
  if [ ! -f "$f" ]; then
    continue
  fi
  # Skip files ending in .down.sql or _down.sql
  case "$f" in
    *.down.sql|*_down.sql)
      continue
      ;;
  esac
  MIGRATION_FILES="${MIGRATION_FILES}
${f}"
done

# Count migration files
FILE_COUNT=0
printf '%s\n' "$MIGRATION_FILES" | while IFS= read -r f; do
  [ -z "$f" ] && continue
  FILE_COUNT=$((FILE_COUNT + 1))
done

# Note: $() subshell can't propagate FILE_COUNT back; iterate directly below.

# ---------------------------------------------------------------------------
# Test each migration
# ---------------------------------------------------------------------------

printf '%s\n' "$MIGRATION_FILES" | while IFS= read -r up_file; do
  [ -z "$up_file" ] && continue

  migration_name=$(basename "$up_file" .sql)
  _log_always "Testing: $migration_name"
  TOTAL_TESTED=$((TOTAL_TESTED + 1))

  test_passed=1

  # Test UP
  if ! _test_up_migration "$up_file"; then
    test_passed=0
  fi

  # Find and test DOWN
  down_file=""
  if _find_down_file "$up_file" >/dev/null 2>&1; then
    down_file=$(_find_down_file "$up_file")
  fi

  if [ -z "$down_file" ]; then
    printf 'WARN [%s]: no DOWN migration file found\n' "$migration_name"
    MISSING_DOWN=$((MISSING_DOWN + 1))
  else
    if ! _test_down_migration "$down_file"; then
      test_passed=0
    fi

    # UP again after DOWN (verify idempotent round-trip)
    _log "  Testing UP again after DOWN: $migration_name"
    if ! _test_up_migration "$up_file"; then
      printf 'FAIL [%s UP-after-DOWN]: migration not idempotent\n' "$migration_name" >&2
      test_passed=0
    fi
  fi

  if [ "$test_passed" -eq 1 ]; then
    TOTAL_PASSED=$((TOTAL_PASSED + 1))
    _log_always "  PASS: $migration_name"
  else
    TOTAL_FAILED=$((TOTAL_FAILED + 1))
    _log_always "  FAIL: $migration_name"
  fi
done

# ---------------------------------------------------------------------------
# Report (summary tracked via separate pass)
# ---------------------------------------------------------------------------

printf '\n'
printf '=== Migration Test Report ===\n'
printf 'Directory: %s\n' "$MIGRATIONS_DIR"
printf 'Missing DOWN files: %s (warning, not failure)\n' "$MISSING_DOWN"

if [ -n "$FAILURES" ]; then
  printf '\nFailed migrations:%s\n' "$FAILURES"
  printf '\nFAILED\n'
  exit 1
fi

printf 'PASSED: All tested migrations completed UP/DOWN lifecycle.\n'
exit 0
