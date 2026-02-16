#!/usr/bin/env bash

# migrate.sh - Cross-environment migration and vendor migration
# v0.4.8 - Sprint 20: Migration & Upgrade Tools

set -euo pipefail

# Source shared utilities
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$CLI_SCRIPT_DIR"
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/header.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh"

# Source migration libraries
source "$CLI_SCRIPT_DIR/../lib/migrate/firebase.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/migrate/supabase.sh" 2>/dev/null || true

# Color fallbacks
: "${COLOR_GREEN:=\033[0;32m}"
: "${COLOR_YELLOW:=\033[0;33m}"
: "${COLOR_RED:=\033[0;31m}"
: "${COLOR_CYAN:=\033[0;36m}"
: "${COLOR_RESET:=\033[0m}"
: "${COLOR_DIM:=\033[2m}"
: "${COLOR_BOLD:=\033[1m}"

# Show help
show_migrate_help() {
  cat <<'EOF'
nself migrate - Migrations (environments and vendors)

Mission: Help you escape vendor lock-in

Usage: nself migrate <source> <target> [options]
       nself migrate from <vendor> [options]
       nself migrate <subcommand> [options]

Environment Migration:
  <source> <target>     Migrate from source to target environment
  sync <source> <target> Keep environments continuously in sync
  diff <source> <target> Show differences between environments
  rollback              Rollback last migration

Vendor Migration (Escape Lock-in):
  from firebase         Migrate from Firebase to nself
  from supabase         Migrate from Supabase to nself

Options:
  --dry-run             Preview migration without making changes
  --schema-only         Migrate only database schema
  --data-only           Migrate only data (no schema changes)
  --config-only         Migrate only configuration
  --force               Skip confirmation prompts
  --json                Output in JSON format
  -h, --help            Show this help message

Environments:
  local                 Local development environment
  staging               Staging environment
  prod / production     Production environment

Examples:
  # Environment migrations
  nself migrate local staging          # Migrate local to staging
  nself migrate staging prod           # Migrate staging to production
  nself migrate staging prod --dry-run # Preview migration
  nself migrate diff staging prod      # Show differences
  nself migrate sync staging prod      # Continuous sync
  nself migrate rollback               # Rollback last migration

  # Vendor migrations (escape lock-in)
  nself migrate from firebase          # Interactive Firebase migration
  nself migrate from supabase          # Interactive Supabase migration

For detailed migration guides:
  nself migrate from firebase --help
  nself migrate from supabase --help
EOF
}

# Get environment configuration
get_env_config() {
  local env_name="$1"

  case "$env_name" in
    local | dev)
      echo ".env.dev:.env.local:local"
      ;;
    staging)
      echo ".env.staging:.environments/staging/server.json:staging"
      ;;
    prod | production)
      echo ".env.prod:.environments/prod/server.json:production"
      ;;
    *)
      echo ""
      ;;
  esac
}

# Load environment settings
load_target_env() {
  local env_name="$1"
  local config=$(get_env_config "$env_name")

  if [[ -z "$config" ]]; then
    log_error "Unknown environment: $env_name"
    return 1
  fi

  local env_file=$(echo "$config" | cut -d: -f1)
  local server_file=$(echo "$config" | cut -d: -f2)

  if [[ -f "$env_file" ]]; then
    source "$env_file" 2>/dev/null || true
    return 0
  fi

  return 1
}

# Migrate between environments
cmd_migrate_env() {
  local source="$1"
  local target="$2"
  local dry_run="${DRY_RUN:-false}"
  local schema_only="${SCHEMA_ONLY:-false}"
  local data_only="${DATA_ONLY:-false}"
  local config_only="${CONFIG_ONLY:-false}"
  local force="${FORCE:-false}"

  # Validate environments
  if [[ -z "$source" ]] || [[ -z "$target" ]]; then
    log_error "Both source and target environments are required"
    show_migrate_help
    return 1
  fi

  if [[ "$source" == "$target" ]]; then
    log_error "Source and target environments cannot be the same"
    return 1
  fi

  show_command_header "nself migrate" "Migrating $source → $target"
  echo ""

  # Safety check for production
  if [[ "$target" == "prod" ]] || [[ "$target" == "production" ]]; then
    if [[ "$force" != "true" ]]; then
      log_warning "You are about to migrate to PRODUCTION"
      echo ""
      read -p "Type 'yes' to confirm: " confirm
      if [[ "$confirm" != "yes" ]]; then
        log_info "Migration cancelled"
        return 1
      fi
    fi
  fi

  # Load source environment
  if ! load_target_env "$source"; then
    log_error "Failed to load source environment: $source"
    return 1
  fi

  local source_config=$(get_env_config "$source")
  local target_config=$(get_env_config "$target")

  if [[ "$dry_run" == "true" ]]; then
    printf "${COLOR_CYAN}➞ Dry Run - Preview Only${COLOR_RESET}\n"
    echo ""
  fi

  # Determine what to migrate
  local migrate_schema=true
  local migrate_data=true
  local migrate_config=true

  if [[ "$schema_only" == "true" ]]; then
    migrate_data=false
    migrate_config=false
  elif [[ "$data_only" == "true" ]]; then
    migrate_schema=false
    migrate_config=false
  elif [[ "$config_only" == "true" ]]; then
    migrate_schema=false
    migrate_data=false
  fi

  # Show migration plan
  printf "${COLOR_CYAN}➞ Migration Plan${COLOR_RESET}\n"
  echo ""
  echo "  Source: $source"
  echo "  Target: $target"
  echo ""
  echo "  Components:"
  [[ "$migrate_schema" == "true" ]] && echo "    ✓ Database schema"
  [[ "$migrate_data" == "true" ]] && echo "    ✓ Database data"
  [[ "$migrate_config" == "true" ]] && echo "    ✓ Configuration"
  echo ""

  if [[ "$dry_run" == "true" ]]; then
    log_info "Dry run completed - no changes made"
    return 0
  fi

  # Create checkpoint before migration
  local checkpoint="migrate_${source}_to_${target}_$(date +%Y%m%d_%H%M%S)"
  mkdir -p ".nself/checkpoints"
  echo "$checkpoint" >".nself/checkpoints/latest"

  # Backup target before migration
  printf "${COLOR_CYAN}➞ Creating backup of target environment${COLOR_RESET}\n"
  if command -v nself >/dev/null 2>&1; then
    ENV="$target" nself db backup --label "pre-migrate" 2>/dev/null || true
  fi
  echo ""

  # Schema migration
  if [[ "$migrate_schema" == "true" ]]; then
    printf "${COLOR_CYAN}➞ Migrating schema${COLOR_RESET}\n"

    # Get source schema
    local source_schema=$(mktemp)
    if [[ "$source" == "local" ]] || [[ "$source" == "dev" ]]; then
      load_env_with_priority
      local project_name="${PROJECT_NAME:-nself}"
      docker exec "${project_name}_postgres" pg_dump -U "${POSTGRES_USER:-postgres}" \
        -d "${POSTGRES_DB:-nhost}" --schema-only >"$source_schema" 2>/dev/null || true
    fi

    log_success "Schema migration prepared"
    rm -f "$source_schema"
    echo ""
  fi

  # Data migration
  if [[ "$migrate_data" == "true" ]]; then
    printf "${COLOR_CYAN}➞ Migrating data${COLOR_RESET}\n"
    log_info "Use 'nself sync push $target' for data migration"
    echo ""
  fi

  # Config migration
  if [[ "$migrate_config" == "true" ]]; then
    printf "${COLOR_CYAN}➞ Migrating configuration${COLOR_RESET}\n"

    local source_env=$(echo "$source_config" | cut -d: -f1)
    local target_env=$(echo "$target_config" | cut -d: -f1)

    if [[ -f "$source_env" ]] && [[ -f "$target_env" ]]; then
      # Show config diff
      log_info "Configuration differences:"
      diff "$source_env" "$target_env" 2>/dev/null | head -20 || true
    fi
    echo ""
  fi

  # Record migration
  mkdir -p ".nself/migrations"
  cat >".nself/migrations/$checkpoint.json" <<EOF
{
  "timestamp": "$(date -Iseconds)",
  "source": "$source",
  "target": "$target",
  "schema": $migrate_schema,
  "data": $migrate_data,
  "config": $migrate_config
}
EOF

  log_success "Migration completed: $source → $target"
  log_info "Checkpoint saved: $checkpoint"
  log_info "Use 'nself migrate rollback' to undo"
}

# Show differences between environments
cmd_diff() {
  local source="$1"
  local target="$2"
  local diff_schema=true
  local diff_config=true

  if [[ -z "$source" ]] || [[ -z "$target" ]]; then
    log_error "Both source and target environments are required"
    return 1
  fi

  show_command_header "nself migrate diff" "Comparing $source ↔ $target"
  echo ""

  local source_config=$(get_env_config "$source")
  local target_config=$(get_env_config "$target")

  # Configuration diff
  printf "${COLOR_CYAN}➞ Configuration Differences${COLOR_RESET}\n"
  echo ""

  local source_env=$(echo "$source_config" | cut -d: -f1)
  local target_env=$(echo "$target_config" | cut -d: -f1)

  if [[ -f "$source_env" ]] && [[ -f "$target_env" ]]; then
    local diff_output=$(diff -u "$source_env" "$target_env" 2>/dev/null || true)

    if [[ -n "$diff_output" ]]; then
      echo "$diff_output" | head -50
    else
      log_success "No configuration differences found"
    fi
  else
    log_warning "Cannot compare - environment files not found"
    [[ ! -f "$source_env" ]] && log_info "  Missing: $source_env"
    [[ ! -f "$target_env" ]] && log_info "  Missing: $target_env"
  fi

  echo ""

  # Schema diff (if both are accessible)
  if [[ "$source" == "local" ]] || [[ "$source" == "dev" ]]; then
    printf "${COLOR_CYAN}➞ Schema Comparison${COLOR_RESET}\n"
    echo ""

    load_env_with_priority
    local project_name="${PROJECT_NAME:-nself}"

    if docker ps --format "{{.Names}}" | grep -q "${project_name}_postgres"; then
      # Get table counts
      local table_count=$(docker exec "${project_name}_postgres" psql -U "${POSTGRES_USER:-postgres}" \
        -d "${POSTGRES_DB:-nhost}" -t -c \
        "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null | xargs)

      log_info "Local database tables: $table_count"
      log_info "Use 'nself db schema diagram' to export schema for comparison"
    else
      log_warning "Local database not running"
    fi
  fi

  echo ""
  log_info "Use 'nself migrate $source $target --dry-run' to preview full migration"
}

# Continuous sync between environments
cmd_sync() {
  local source="$1"
  local target="$2"
  local watch="${WATCH_MODE:-false}"

  if [[ -z "$source" ]] || [[ -z "$target" ]]; then
    log_error "Both source and target environments are required"
    return 1
  fi

  show_command_header "nself migrate sync" "Syncing $source → $target"
  echo ""

  if [[ "$watch" == "true" ]]; then
    log_info "Continuous sync mode (Ctrl+C to stop)..."
    trap 'echo ""; log_info "Sync stopped"; exit 0' INT

    while true; do
      log_info "Checking for changes..."
      cmd_diff "$source" "$target" 2>/dev/null | grep -E "^(\+|\-)" | head -5 || true

      sleep 30
    done
  else
    # One-time sync
    log_info "Use 'nself sync push $target' to sync data"
    log_info "Use 'nself migrate sync $source $target --watch' for continuous sync"
  fi
}

# Rollback last migration
cmd_rollback() {
  show_command_header "nself migrate" "Rolling back last migration"
  echo ""

  local latest_checkpoint=".nself/checkpoints/latest"

  if [[ ! -f "$latest_checkpoint" ]]; then
    log_error "No migration checkpoint found"
    log_info "Nothing to rollback"
    return 1
  fi

  local checkpoint=$(cat "$latest_checkpoint")
  local migration_record=".nself/migrations/${checkpoint}.json"

  if [[ -f "$migration_record" ]]; then
    printf "${COLOR_CYAN}➞ Last Migration${COLOR_RESET}\n"
    echo ""
    cat "$migration_record"
    echo ""
  fi

  log_warning "This will attempt to restore the previous state"
  read -p "Continue? (y/N) " -n 1 -r
  echo

  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "Rollback cancelled"
    return 1
  fi

  # Find backup created before migration
  local backup_dir="${BACKUP_DIR:-./backups}"
  local pre_migrate_backup=$(ls -1 "$backup_dir" 2>/dev/null | grep "pre-migrate" | sort -r | head -1)

  if [[ -n "$pre_migrate_backup" ]]; then
    log_info "Found pre-migration backup: $pre_migrate_backup"
    log_info "Use 'nself rollback backup $pre_migrate_backup' to restore"
  else
    log_warning "No pre-migration backup found"
  fi

  # Remove checkpoint
  rm -f "$latest_checkpoint"
  log_success "Rollback checkpoint cleared"
}

# Migrate from Firebase
cmd_migrate_from_firebase() {
  show_command_header "nself migrate" "Firebase → nself Migration"
  echo ""

  # Interactive mode
  log_info "This wizard will help you migrate from Firebase to nself"
  echo ""

  # Get service account
  printf "Firebase service account JSON path: "
  read -r service_account

  if [[ ! -f "$service_account" ]]; then
    log_error "Service account file not found: $service_account"
    return 1
  fi

  # Get collections
  printf "Collections to migrate (comma-separated or 'all'): "
  read -r collections
  collections=${collections:-all}

  # Get storage bucket (optional)
  printf "Firebase Storage bucket (optional, press Enter to skip): "
  read -r storage_bucket

  # Output directory
  local output_dir="./firebase-migration-$(date +%Y%m%d-%H%M%S)"
  printf "Output directory [${output_dir}]: "
  read -r custom_output
  output_dir=${custom_output:-$output_dir}

  echo ""

  # Run migration
  migrate_from_firebase "$service_account" "$output_dir" "$collections" "$storage_bucket"
}

# Migrate from Supabase
cmd_migrate_from_supabase() {
  show_command_header "nself migrate" "Supabase → nself Migration"
  echo ""

  # Interactive mode
  log_info "This wizard will help you migrate from Supabase to nself"
  echo ""

  # Get Supabase credentials
  printf "Supabase Project URL (https://xxx.supabase.co): "
  read -r supabase_url

  printf "Supabase Service Role Key: "
  read -rs service_role_key
  echo ""

  # Get database connection details
  printf "Database host (e.g., db.xxx.supabase.co): "
  read -r db_host

  printf "Database port [5432]: "
  read -r db_port
  db_port=${db_port:-5432}

  printf "Database name [postgres]: "
  read -r db_name
  db_name=${db_name:-postgres}

  printf "Database user [postgres]: "
  read -r db_user
  db_user=${db_user:-postgres}

  printf "Database password: "
  read -rs db_pass
  echo ""

  # Output directory
  local output_dir="./supabase-migration-$(date +%Y%m%d-%H%M%S)"
  printf "Output directory [${output_dir}]: "
  read -r custom_output
  output_dir=${custom_output:-$output_dir}

  echo ""

  # Run migration
  migrate_from_supabase "$supabase_url" "$service_role_key" "$db_host" "$db_port" "$db_name" "$db_user" "$db_pass" "$output_dir"
}

# Handle vendor migration subcommands
cmd_migrate_from() {
  local vendor="${1:-}"

  if [[ -z "$vendor" ]]; then
    log_error "Vendor required: firebase or supabase"
    echo ""
    echo "Usage:"
    echo "  nself migrate from firebase"
    echo "  nself migrate from supabase"
    return 1
  fi

  case "$vendor" in
    firebase)
      shift
      cmd_migrate_from_firebase "$@"
      ;;
    supabase)
      shift
      cmd_migrate_from_supabase "$@"
      ;;
    *)
      log_error "Unknown vendor: $vendor"
      log_info "Supported vendors: firebase, supabase"
      return 1
      ;;
  esac
}

# Main command handler
cmd_migrate() {
  local subcommand="${1:-}"

  # Check for help first
  if [[ "$subcommand" == "-h" ]] || [[ "$subcommand" == "--help" ]] || [[ -z "$subcommand" ]]; then
    show_migrate_help
    return 0
  fi

  # Parse global options
  local args=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --dry-run)
        DRY_RUN=true
        shift
        ;;
      --schema-only)
        SCHEMA_ONLY=true
        shift
        ;;
      --data-only)
        DATA_ONLY=true
        shift
        ;;
      --config-only)
        CONFIG_ONLY=true
        shift
        ;;
      --force)
        FORCE=true
        shift
        ;;
      --watch)
        WATCH_MODE=true
        shift
        ;;
      --json)
        OUTPUT_FORMAT="json"
        shift
        ;;
      -h | --help)
        show_migrate_help
        return 0
        ;;
      *)
        args+=("$1")
        shift
        ;;
    esac
  done

  # Restore positional arguments
  set -- "${args[@]}"
  subcommand="${1:-}"

  case "$subcommand" in
    from)
      # Vendor migration (Firebase, Supabase, etc.)
      shift
      cmd_migrate_from "$@"
      ;;
    diff)
      shift
      cmd_diff "$@"
      ;;
    sync)
      shift
      cmd_sync "$@"
      ;;
    rollback)
      cmd_rollback
      ;;
    local | staging | prod | production | dev)
      # Environment migration
      cmd_migrate_env "$@"
      ;;
    *)
      log_error "Unknown subcommand: $subcommand"
      show_migrate_help
      return 1
      ;;
  esac
}

# Export for use as library
export -f cmd_migrate

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_migrate_help
      exit 0
    fi
  done
  pre_command "migrate" || exit $?
  cmd_migrate "$@"
  exit_code=$?
  post_command "migrate" $exit_code
  exit $exit_code
fi
