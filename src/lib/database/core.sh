#!/usr/bin/env bash

# core.sh - Core database utilities for nself
# Provides shared functions for all database operations

# Prevent multiple sourcing
[[ -n "${NSELF_DB_CORE_LOADED:-}" ]] && return 0

set -euo pipefail

NSELF_DB_CORE_LOADED=true

# Source utilities
NSELF_DB_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$NSELF_DB_LIB_DIR/../utils/env.sh" 2>/dev/null || true
source "$NSELF_DB_LIB_DIR/../utils/display.sh" 2>/dev/null || true

# ============================================================================
# ENVIRONMENT DETECTION
# ============================================================================

# Get current environment (local, staging, production)
db_get_environment() {
  local env="${ENV:-${ENVIRONMENT:-local}}"

  # Normalize environment names
  case "$env" in
    dev | development | local)
      echo "local"
      ;;
    staging | stage)
      echo "staging"
      ;;
    prod | production)
      echo "production"
      ;;
    *)
      echo "$env"
      ;;
  esac
}

# Check if current environment is production
db_is_production() {
  local env=$(db_get_environment)
  [[ "$env" == "production" ]]
}

# Check if current environment is staging
db_is_staging() {
  local env=$(db_get_environment)
  [[ "$env" == "staging" ]]
}

# Check if current environment is local
db_is_local() {
  local env=$(db_get_environment)
  [[ "$env" == "local" ]]
}

# ============================================================================
# SAFETY GUARDS
# ============================================================================

# Block dangerous operations in production
db_require_non_production() {
  local operation="${1:-This operation}"

  if db_is_production; then
    log_error "BLOCKED: $operation is not allowed in production"
    log_info "Current environment: $(db_get_environment)"
    log_info "Use --force --i-know-what-im-doing to override (dangerous!)"
    return 1
  fi
  return 0
}

# Require confirmation for destructive operations
db_require_confirmation() {
  local message="${1:-This is a destructive operation}"
  local env=$(db_get_environment)

  if db_is_production; then
    log_warning "PRODUCTION ENVIRONMENT DETECTED"
    echo ""
    log_error "$message"
    echo ""
    printf "Type 'yes-destroy-production-data' to confirm: "
    read -r response
    if [[ "$response" != "yes-destroy-production-data" ]]; then
      log_info "Operation cancelled"
      return 1
    fi
  elif db_is_staging; then
    log_warning "Staging environment: $message"
    printf "Continue? (y/N): "
    read -r response
    if [[ ! "$response" =~ ^[Yy]$ ]]; then
      log_info "Operation cancelled"
      return 1
    fi
  fi
  return 0
}

# ============================================================================
# DATABASE CONNECTION
# ============================================================================

# Get container name for PostgreSQL
db_get_container_name() {
  local project="${PROJECT_NAME:-nself}"
  echo "${project}_postgres"
}

# Check if PostgreSQL container is running
db_is_running() {
  local container=$(db_get_container_name)
  docker ps --format "{{.Names}}" 2>/dev/null | grep -q "^${container}$"
}

# Wait for database to be ready
db_wait_ready() {
  local timeout="${1:-30}"
  local container=$(db_get_container_name)
  local user="${POSTGRES_USER:-postgres}"

  log_info "Waiting for database to be ready..."

  local count=0
  while [[ $count -lt $timeout ]]; do
    if docker exec "$container" pg_isready -U "$user" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
    count=$((count + 1))
  done

  log_error "Database not ready after ${timeout}s"
  return 1
}

# Execute SQL query
db_query() {
  local sql="$1"
  local db="${2:-${POSTGRES_DB:-nhost}}"
  local container=$(db_get_container_name)
  local user="${POSTGRES_USER:-postgres}"

  docker exec -i "$container" psql -U "$user" -d "$db" -c "$sql" 2>/dev/null
}

# Execute SQL query and return raw result
db_query_raw() {
  local sql="$1"
  local db="${2:-${POSTGRES_DB:-nhost}}"
  local container=$(db_get_container_name)
  local user="${POSTGRES_USER:-postgres}"

  # -q: quiet mode (suppresses command tags like "INSERT 0 1")
  # -t: tuple-only mode (no headers)
  # -A: unaligned output
  docker exec -i "$container" psql -U "$user" -d "$db" -q -t -A -c "$sql" 2>/dev/null
}

# Execute SQL file
db_exec_file() {
  local file="$1"
  local db="${2:-${POSTGRES_DB:-nhost}}"
  local container=$(db_get_container_name)
  local user="${POSTGRES_USER:-postgres}"

  if [[ ! -f "$file" ]]; then
    log_error "SQL file not found: $file"
    return 1
  fi

  docker exec -i "$container" psql -U "$user" -d "$db" <"$file"
}

# Open interactive shell
db_shell() {
  local db="${1:-${POSTGRES_DB:-nhost}}"
  local container=$(db_get_container_name)
  local user="${POSTGRES_USER:-postgres}"
  local readonly="${2:-false}"

  local flags=""
  if [[ "$readonly" == "true" ]]; then
    flags="-v ON_ERROR_ROLLBACK=on"
  fi

  docker exec -it "$container" psql -U "$user" -d "$db" $flags
}

# ============================================================================
# DATABASE INFO
# ============================================================================

# Get list of databases
db_list_databases() {
  local container=$(db_get_container_name)
  local user="${POSTGRES_USER:-postgres}"

  docker exec "$container" psql -U "$user" -t -c \
    "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname"
}

# Get list of tables in a database
db_list_tables() {
  local db="${1:-${POSTGRES_DB:-nhost}}"
  local schema="${2:-public}"

  db_query_raw "SELECT tablename FROM pg_tables WHERE schemaname = '$schema' ORDER BY tablename" "$db"
}

# Get row count for a table
db_table_count() {
  local table="$1"
  local db="${2:-${POSTGRES_DB:-nhost}}"

  db_query_raw "SELECT COUNT(*) FROM $table" "$db"
}

# Get table size
db_table_size() {
  local table="$1"
  local db="${2:-${POSTGRES_DB:-nhost}}"

  db_query_raw "SELECT pg_size_pretty(pg_total_relation_size('$table'))" "$db"
}

# Get database size
db_database_size() {
  local db="${1:-${POSTGRES_DB:-nhost}}"

  db_query_raw "SELECT pg_size_pretty(pg_database_size('$db'))" "$db"
}

# ============================================================================
# MIGRATIONS
# ============================================================================

# Migrations directory
db_migrations_dir() {
  echo "${NSELF_MIGRATIONS_DIR:-nself/migrations}"
}

# Get list of migration files
db_list_migrations() {
  local dir=$(db_migrations_dir)

  if [[ -d "$dir" ]]; then
    ls -1 "$dir"/*.sql 2>/dev/null | sort
  fi
}

# Get applied migrations from database
db_applied_migrations() {
  local db="${1:-${POSTGRES_DB:-nhost}}"

  # Check if migrations table exists
  local exists=$(db_query_raw "SELECT 1 FROM information_schema.tables WHERE table_name = 'schema_migrations'" "$db")

  if [[ "$exists" == "1" ]]; then
    db_query_raw "SELECT version FROM schema_migrations ORDER BY version" "$db"
  fi
}

# Create migrations tracking table
db_create_migrations_table() {
  local db="${1:-${POSTGRES_DB:-nhost}}"

  db_query "CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(255) PRIMARY KEY,
    applied_at TIMESTAMPTZ DEFAULT NOW()
  )" "$db" >/dev/null
}

# Record migration as applied
db_record_migration() {
  local version="$1"
  local db="${2:-${POSTGRES_DB:-nhost}}"

  db_query "INSERT INTO schema_migrations (version) VALUES ('$version') ON CONFLICT DO NOTHING" "$db" >/dev/null
}

# Remove migration record
db_unrecord_migration() {
  local version="$1"
  local db="${2:-${POSTGRES_DB:-nhost}}"

  db_query "DELETE FROM schema_migrations WHERE version = '$version'" "$db" >/dev/null
}

# ============================================================================
# SEEDS
# ============================================================================

# Seeds directory
db_seeds_dir() {
  echo "${NSELF_SEEDS_DIR:-nself/seeds}"
}

# Get seeds for current environment
db_env_seeds_dir() {
  local env=$(db_get_environment)
  echo "$(db_seeds_dir)/$env"
}

# ============================================================================
# BACKUPS
# ============================================================================

# Backups directory
db_backups_dir() {
  echo "${NSELF_BACKUPS_DIR:-_backups}"
}

# Generate backup filename
db_backup_filename() {
  local type="${1:-full}"
  local timestamp=$(date +%Y%m%d_%H%M%S)
  local project="${PROJECT_NAME:-nself}"

  echo "${project}_${type}_${timestamp}.sql"
}

# ============================================================================
# UTILITIES
# ============================================================================

# Validate database name (prevent injection)
db_validate_name() {
  local name="$1"

  if [[ ! "$name" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
    log_error "Invalid database/table name: $name"
    log_info "Names must start with a letter or underscore and contain only alphanumeric characters and underscores"
    return 1
  fi
  return 0
}

# Validate SQL identifier
db_validate_identifier() {
  local id="$1"

  if [[ ! "$id" =~ ^[a-zA-Z_][a-zA-Z0-9_.-]*$ ]]; then
    log_error "Invalid identifier: $id"
    return 1
  fi
  return 0
}

# Calculate file hash (cross-platform)
db_file_hash() {
  local file="$1"

  if [[ ! -f "$file" ]]; then
    echo "file_not_found"
    return 1
  fi

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | cut -d' ' -f1
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | cut -d' ' -f1
  else
    md5sum "$file" | cut -d' ' -f1
  fi
}

# Ensure nself directories exist
db_ensure_directories() {
  mkdir -p "$(db_migrations_dir)"
  mkdir -p "$(db_seeds_dir)/common"
  mkdir -p "$(db_seeds_dir)/local"
  mkdir -p "$(db_seeds_dir)/staging"
  mkdir -p "$(db_seeds_dir)/production"
  mkdir -p "$(db_backups_dir)"
  mkdir -p "nself/mock"
  mkdir -p "nself/tests"
  mkdir -p ".nself"
}
