#!/usr/bin/env bash
# migrate.sh - Database migration library for nself CLI
# Handles Hasura-style migrations with tracking

# Apply pending migrations
migrate_up() {

set -euo pipefail

  cli_header "nself db migrate up"
  cli_subheader "Apply pending database migrations"

  # Load environment
  load_env_with_priority true

  # Verify migrations directory exists
  local migrations_dir="hasura/migrations/default"
  if [[ ! -d "$migrations_dir" ]]; then
    cli_error "Migrations directory not found: $migrations_dir"
    cli_info "nSelf uses Hasura-format migrations in hasura/migrations/default/"
    cli_info "Each migration is a numbered directory with up.sql and down.sql"
    cli_info "Create with: mkdir -p hasura/migrations/default"
    cli_info "Then create a migration: nself db migrate create <name>"
    exit 1
  fi

  # Get database connection info
  local db_container="${PROJECT_NAME}_postgres"
  local db_user="${POSTGRES_USER:-postgres}"
  local db_name="${POSTGRES_DB:-${PROJECT_NAME}}"

  # Check if container is running
  if ! docker ps --format "{{.Names}}" | grep -q "^${db_container}$"; then
    cli_error "Database container not running: $db_container"
    cli_info "Start services with: nself start"
    exit 1
  fi

  # Ensure schema_migrations table exists
  printf "${COLOR_BLUE}→${COLOR_RESET} Initializing migration tracking...\n"
  docker exec -i "$db_container" psql -U "$db_user" -d "$db_name" <<'SQL' >/dev/null 2>&1 || true
CREATE TABLE IF NOT EXISTS schema_migrations (
  version BIGINT PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
SQL
  printf "${COLOR_GREEN}✓${COLOR_RESET} Migration tracking ready\n"

  # Get list of applied migrations
  local applied_migrations=$(docker exec "$db_container" psql -U "$db_user" -d "$db_name" -t -c "SELECT version FROM schema_migrations ORDER BY version" 2>/dev/null || echo "")

  # Find and apply pending migrations
  local pending_count=0
  local applied_count=0
  local failed_count=0

  printf "\n${COLOR_BLUE}→${COLOR_RESET} Checking for pending migrations...\n"

  # Apply migrations in timestamp order
  for migration_dir in $(find "$migrations_dir" -maxdepth 1 -type d -name '[0-9]*' | sort); do
    local migration_name=$(basename "$migration_dir")
    local version=$(echo "$migration_name" | cut -d'_' -f1)
    local up_sql="$migration_dir/up.sql"

    # Skip if already applied
    if echo "$applied_migrations" | grep -q "^\s*${version}\s*$"; then
      applied_count=$((applied_count + 1))
      continue
    fi

    # Check if up.sql exists
    if [[ ! -f "$up_sql" ]]; then
      cli_warning "Skipping $migration_name: up.sql not found"
      continue
    fi

    # Apply migration
    printf "  ${COLOR_BLUE}⠋${COLOR_RESET} Applying: $migration_name..."

    local start_time=$(date +%s)
    if docker exec -i "$db_container" psql -U "$db_user" -d "$db_name" < "$up_sql" >/dev/null 2>&1; then
      local end_time=$(date +%s)
      local duration=$((end_time - start_time))

      # Mark as applied
      docker exec -i "$db_container" psql -U "$db_user" -d "$db_name" <<SQL >/dev/null 2>&1
INSERT INTO schema_migrations (version, name) VALUES ($version, '$migration_name');
SQL

      printf "\r  ${COLOR_GREEN}✓${COLOR_RESET} $migration_name (${duration}s)\n"
      pending_count=$((pending_count + 1))
    else
      printf "\r  ${COLOR_RED}✗${COLOR_RESET} $migration_name (FAILED)\n"
      cli_error "Migration failed. Check SQL syntax and database logs."
      failed_count=$((failed_count + 1))
      break
    fi
  done

  # Summary
  printf "\n"
  if [[ $failed_count -gt 0 ]]; then
    cli_error "Migration failed"
    cli_info "Fix the SQL error and run 'nself db migrate up' again"
    exit 1
  elif [[ $pending_count -eq 0 ]]; then
    cli_success "Database is up to date"
    if [[ $applied_count -gt 0 ]]; then
      printf "  Total: $applied_count migration(s) applied\n"
    else
      printf "  No migrations found\n"
    fi
  else
    cli_success "All migrations applied successfully"
    printf "  Applied: $pending_count migration(s)\n"
    printf "  Total: $((applied_count + pending_count)) migration(s)\n"
  fi

  printf "\n"
  cli_info "Next: Run 'nself db seed' to populate data"
}

# Show migration status
migrate_status() {
  cli_header "nself db migrate status"
  cli_subheader "Database migration status"

  # Load environment
  load_env_with_priority true

  # Verify migrations directory
  local migrations_dir="hasura/migrations/default"
  if [[ ! -d "$migrations_dir" ]]; then
    cli_warning "No migrations directory found"
    cli_info "Create with: mkdir -p hasura/migrations/default"
    exit 0
  fi

  # Get database connection
  local db_container="${PROJECT_NAME}_postgres"
  local db_user="${POSTGRES_USER:-postgres}"
  local db_name="${POSTGRES_DB:-${PROJECT_NAME}}"

  # Check if container is running
  if ! docker ps --format "{{.Names}}" | grep -q "^${db_container}$"; then
    cli_error "Database container not running"
    exit 1
  fi

  # Get applied migrations
  local applied_migrations=$(docker exec "$db_container" psql -U "$db_user" -d "$db_name" -t -c "SELECT version FROM schema_migrations ORDER BY version" 2>/dev/null || echo "")

  printf "\n${COLOR_BLUE}→${COLOR_RESET} Migration Status:\n\n"

  local total=0
  local applied=0
  local pending=0

  # List all migrations
  for migration_dir in $(find "$migrations_dir" -maxdepth 1 -type d -name '[0-9]*' | sort); do
    local migration_name=$(basename "$migration_dir")
    local version=$(echo "$migration_name" | cut -d'_' -f1)
    total=$((total + 1))

    if echo "$applied_migrations" | grep -q "^\s*${version}\s"; then
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} $migration_name ${COLOR_DIM}(applied)${COLOR_RESET}\n"
      applied=$((applied + 1))
    else
      printf "  ${COLOR_YELLOW}○${COLOR_RESET} $migration_name ${COLOR_DIM}(pending)${COLOR_RESET}\n"
      pending=$((pending + 1))
    fi
  done

  printf "\n"
  printf "${COLOR_GREEN}✓${COLOR_RESET} $applied applied\n"
  if [[ $pending -gt 0 ]]; then
    printf "${COLOR_YELLOW}⏳${COLOR_RESET} $pending pending\n"
    printf "\n"
    cli_info "Run 'nself db migrate up' to apply"
  else
    printf "\n"
    cli_success "Database is up to date"
  fi
}

# Rollback last migration
migrate_down() {
  cli_header "nself db migrate down"
  cli_subheader "Rollback last migration"

  load_env_with_priority true

  local db_container="${PROJECT_NAME}_postgres"
  local db_user="${POSTGRES_USER:-postgres}"
  local db_name="${POSTGRES_DB:-${PROJECT_NAME}}"

  if ! docker ps --format "{{.Names}}" | grep -q "^${db_container}$"; then
    cli_error "Database container not running"
    exit 1
  fi

  # Get last migration
  local last_version=$(docker exec "$db_container" psql -U "$db_user" -d "$db_name" -t -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1" 2>/dev/null | tr -d ' ')

  if [[ -z "$last_version" ]]; then
    cli_warning "No migrations to rollback"
    exit 0
  fi

  # Find migration directory
  local migrations_dir="hasura/migrations/default"
  local migration_dir=$(find "$migrations_dir" -maxdepth 1 -type d -name "${last_version}_*" | head -1)

  if [[ -z "$migration_dir" ]]; then
    cli_error "Migration directory not found for version: $last_version"
    exit 1
  fi

  local migration_name=$(basename "$migration_dir")
  local down_sql="$migration_dir/down.sql"

  if [[ ! -f "$down_sql" ]]; then
    cli_error "Rollback not available: down.sql not found"
    exit 1
  fi

  # Confirm
  printf "${COLOR_YELLOW}⚠${COLOR_RESET}  Rollback: $migration_name\n"
  printf "Continue? (y/N): "
  read -r response

  if [[ ! "$response" =~ ^[Yy]$ ]]; then
    cli_info "Cancelled"
    exit 0
  fi

  # Execute rollback
  printf "\n${COLOR_BLUE}→${COLOR_RESET} Rolling back...\n"

  if docker exec -i "$db_container" psql -U "$db_user" -d "$db_name" < "$down_sql" >/dev/null 2>&1; then
    docker exec "$db_container" psql -U "$db_user" -d "$db_name" <<SQL >/dev/null 2>&1
DELETE FROM schema_migrations WHERE version = $last_version;
SQL
    printf "${COLOR_GREEN}✓${COLOR_RESET} Rolled back: $migration_name\n"
    cli_success "Rollback complete"
  else
    cli_error "Rollback failed"
    exit 1
  fi
}

# Create new migration
migrate_create() {
  local migration_name="$1"

  if [[ -z "$migration_name" ]]; then
    cli_error "Migration name required"
    printf "Usage: nself db migrate create <name>\n"
    exit 1
  fi

  cli_header "nself db migrate create"
  cli_subheader "Create new migration: $migration_name"

  # Ensure directory exists
  local migrations_dir="hasura/migrations/default"
  mkdir -p "$migrations_dir"

  # Generate version (timestamp in milliseconds)
  local version=$(date +%s)000
  local dir_name="${version}_${migration_name}"
  local migration_path="$migrations_dir/$dir_name"

  # Create migration directory
  mkdir -p "$migration_path"

  # Create up.sql
  cat > "$migration_path/up.sql" <<SQL
-- Migration: ${migration_name}
-- Created: $(date '+%Y-%m-%d %H:%M:%S')

-- Add your migration SQL here
-- Example:
-- CREATE TABLE example (
--   id SERIAL PRIMARY KEY,
--   name TEXT NOT NULL
-- );

SQL

  # Create down.sql
  cat > "$migration_path/down.sql" <<SQL
-- Rollback: ${migration_name}

-- Add your rollback SQL here
-- Example:
-- DROP TABLE IF EXISTS example;

SQL

  printf "${COLOR_GREEN}✓${COLOR_RESET} Migration created: $dir_name\n"
  printf "\n"
  printf "  Files:\n"
  printf "    ${COLOR_CYAN}$migration_path/up.sql${COLOR_RESET}\n"
  printf "    ${COLOR_CYAN}$migration_path/down.sql${COLOR_RESET}\n"
  printf "\n"
  cli_info "Edit SQL files, then run: nself db migrate up"
}

# Export functions
export -f migrate_up
export -f migrate_down
export -f migrate_status
export -f migrate_create
