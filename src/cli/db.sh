#!/usr/bin/env bash
# db.sh - Database management for nself CLI
# Handles migrations, seeding, backups, and database utilities
set -euo pipefail

# Source required libraries
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../lib/utils/cli-output.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../lib/utils/env.sh" 2>/dev/null || true

# Database command entry point
cmd_db() {
  local subcommand="${1:-help}"
  shift || true

  case "$subcommand" in
    migrate)
      db_migrate "$@"
      ;;
    seed)
      db_seed "$@"
      ;;
    shell)
      db_shell "$@"
      ;;
    backup)
      if [[ "${1:-}" == "list" ]]; then
        cli_header "nself db backup list"
        if ls backups/*.sql 2>/dev/null | head -1 > /dev/null 2>&1; then
          ls -lh backups/*.sql 2>/dev/null
        else
          printf "No database backups found in ./backups/\n"
        fi
      else
        db_backup "$@"
      fi
      ;;
    restore)
      db_restore "$@"
      ;;
    reset)
      db_reset "$@"
      ;;
    hasura)
      source "$(dirname "${BASH_SOURCE[0]}")/hasura.sh"
      cmd_hasura "$@"
      ;;
    help)
      db_help
      ;;
    *)
      cli_error "Unknown subcommand: $subcommand"
      db_help
      exit 1
      ;;
  esac
}

# Database migration sub-commands
db_migrate() {
  local action="${1:-help}"
  shift || true

  # Source migration library
  if [[ -f "$SCRIPT_DIR/../lib/database/migrate.sh" ]]; then
    source "$SCRIPT_DIR/../lib/database/migrate.sh"
  fi

  case "$action" in
    up)
      migrate_up "$@"
      ;;
    down)
      migrate_down "$@"
      ;;
    status)
      migrate_status "$@"
      ;;
    create)
      migrate_create "$@"
      ;;
    *)
      cli_error "Unknown migrate action: $action"
      printf "Usage: nself db migrate [up|down|status|create]\n"
      exit 1
      ;;
  esac
}

# Database seeding sub-commands
db_seed() {
  # Source seed library
  if [[ -f "$SCRIPT_DIR/../lib/database/seed.sh" ]]; then
    source "$SCRIPT_DIR/../lib/database/seed.sh"
  fi

  seed_database "$@"
}

# Open database shell
db_shell() {
  cli_header "nself db shell"
  cli_subheader "PostgreSQL interactive shell"

  # Load environment
  load_env_with_priority true

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

  printf "${CLI_GREEN}✓${CLI_RESET} Connected to: $db_name\n"
  printf "${CLI_DIM}Type \\\\q to exit${CLI_RESET}\n\n"

  # Open psql shell
  docker exec -it "$db_container" psql -U "$db_user" -d "$db_name"
}

# Backup database
db_backup() {
  local backup_file="$1"

  cli_header "nself db backup"
  cli_subheader "Create database backup"

  # Load environment
  load_env_with_priority true

  # Get database connection info
  local db_container="${PROJECT_NAME}_postgres"
  local db_user="${POSTGRES_USER:-postgres}"
  local db_name="${POSTGRES_DB:-${PROJECT_NAME}}"

  # Generate backup filename if not provided
  if [[ -z "$backup_file" ]]; then
    local timestamp=$(date +%Y%m%d_%H%M%S)
    backup_file="backup_${db_name}_${timestamp}.sql"
  fi

  # Ensure backups directory exists
  mkdir -p backups

  local backup_path="backups/$backup_file"

  # Check if container is running
  if ! docker ps --format "{{.Names}}" | grep -q "^${db_container}$"; then
    cli_error "Database container not running: $db_container"
    exit 1
  fi

  # Create backup
  printf "${CLI_BLUE}→${CLI_RESET} Creating backup: $backup_file...\n"

  if docker exec "$db_container" pg_dump -U "$db_user" "$db_name" > "$backup_path" 2>/dev/null; then
    local size=$(du -h "$backup_path" | cut -f1)
    printf "${CLI_GREEN}✓${CLI_RESET} Backup created: $backup_path ($size)\n"
    cli_success "Database backup complete"
  else
    cli_error "Backup failed"
    rm -f "$backup_path"
    exit 1
  fi
}

# Restore database from backup
db_restore() {
  local backup_file="$1"

  if [[ -z "$backup_file" ]]; then
    cli_error "Backup file required"
    printf "Usage: nself db restore <backup-file>\n"
    exit 1
  fi

  cli_header "nself db restore"
  cli_subheader "Restore database from backup"

  # Load environment
  load_env_with_priority true

  # Check environment
  local env="${ENV:-dev}"
  if [[ "$env" == "prod" ]] || [[ "$env" == "production" ]]; then
    printf "${CLI_RED}⚠ WARNING:${CLI_RESET} Restoring in production environment\n"
    printf "Continue? (type 'yes' to confirm): "
    read -r response
    if [[ "$response" != "yes" ]]; then
      cli_info "Restore cancelled"
      exit 0
    fi
  fi

  # Check if backup file exists
  if [[ ! -f "$backup_file" ]]; then
    cli_error "Backup file not found: $backup_file"
    exit 1
  fi

  # Get database connection info
  local db_container="${PROJECT_NAME}_postgres"
  local db_user="${POSTGRES_USER:-postgres}"
  local db_name="${POSTGRES_DB:-${PROJECT_NAME}}"

  # Check if container is running
  if ! docker ps --format "{{.Names}}" | grep -q "^${db_container}$"; then
    cli_error "Database container not running: $db_container"
    exit 1
  fi

  # Restore backup
  printf "${CLI_BLUE}→${CLI_RESET} Restoring from: $backup_file...\n"

  if docker exec -i "$db_container" psql -U "$db_user" -d "$db_name" < "$backup_file" >/dev/null 2>&1; then
    printf "${CLI_GREEN}✓${CLI_RESET} Database restored\n"
    cli_success "Restore complete"
  else
    cli_error "Restore failed"
    exit 1
  fi
}

# Reset database (dev only)
db_reset() {
  cli_header "nself db reset"
  cli_subheader "Reset database (DESTRUCTIVE)"

  # Load environment
  load_env_with_priority true

  # Check environment
  local env="${ENV:-dev}"
  if [[ "$env" == "prod" ]] || [[ "$env" == "production" ]]; then
    cli_error "Cannot reset production database"
    cli_info "This command is only available in dev environment"
    exit 1
  fi

  # Confirm reset
  printf "${CLI_RED}⚠ WARNING:${CLI_RESET} This will DROP ALL TABLES\n"
  printf "Environment: ${CLI_YELLOW}$env${CLI_RESET}\n"
  printf "Continue? (type 'yes' to confirm): "
  read -r response

  if [[ "$response" != "yes" ]]; then
    cli_info "Reset cancelled"
    exit 0
  fi

  # Get database connection info
  local db_container="${PROJECT_NAME}_postgres"
  local db_user="${POSTGRES_USER:-postgres}"
  local db_name="${POSTGRES_DB:-${PROJECT_NAME}}"

  # Drop all tables
  printf "\n${CLI_BLUE}→${CLI_RESET} Dropping all tables...\n"
  docker exec "$db_container" psql -U "$db_user" -d "$db_name" <<'SQL' >/dev/null 2>&1
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO postgres;
GRANT ALL ON SCHEMA public TO public;
SQL

  printf "${CLI_GREEN}✓${CLI_RESET} Database reset complete\n"
  printf "\n"
  cli_info "Run 'nself db migrate up' to re-apply migrations"
}

# Show help
db_help() {
  cat <<'HELP'
╔══════════════════════════════════════════════════════════╗
║ nself db                                                  ║
║ Database management and utilities                         ║
╚══════════════════════════════════════════════════════════╝

USAGE:
  nself db <command> [options]

COMMANDS:
  migrate <action>    Database migrations (up/down/status/create)
  seed [options]      Apply seed data
  shell               Open PostgreSQL shell
  backup [file]       Create database backup
  restore <file>      Restore from backup
  reset               Reset database (dev only)
  hasura <sub>        Hasura console and metadata (console, metadata apply/export/reload)
  help                Show this help message

MIGRATION COMMANDS:
  nself db migrate up         Apply pending migrations
  nself db migrate down       Rollback last migration
  nself db migrate status     Show migration status
  nself db migrate create <name>  Create new migration

EXAMPLES:
  # Check migration status
  nself db migrate status

  # Apply migrations
  nself db migrate up

  # Seed database
  nself db seed

  # Open database shell
  nself db shell

  # Create backup
  nself db backup

  # Restore from backup
  nself db restore backups/backup_20260211.sql

  # Reset database (dev only)
  nself db reset

  # Open Hasura Console
  nself db hasura console

  # Apply Hasura metadata
  nself db hasura metadata apply

NOTES:
  - Migrations are stored in hasura/migrations/default/
  - Seeds are stored in hasura/seeds/default/
  - Backups are saved to backups/ directory
  - Reset command only works in dev environment

HELP
}

# Export main function
export -f cmd_db

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_db "$@"
fi
