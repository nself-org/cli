#!/usr/bin/env bash
# seed.sh - Database seeding library for nself CLI
# Environment-aware seed data management

# Apply seed data
seed_database() {

set -euo pipefail

  local env_override=""
  local reset=false

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --env=*)
        env_override="${1#*=}"
        shift
        ;;
      --reset)
        reset=true
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  cli_header "nself db seed"
  cli_subheader "Apply environment-specific seed data"

  # Load environment
  load_env_with_priority true

  # Determine environment
  local env="${env_override:-${ENV:-dev}}"

  # Verify seeds directory
  local seeds_dir="hasura/seeds/default"
  if [[ ! -d "$seeds_dir" ]]; then
    cli_error "Seeds directory not found: $seeds_dir"
    cli_info "Create with: mkdir -p hasura/seeds/default"
    exit 1
  fi

  # Get database connection
  local db_container="${PROJECT_NAME}_postgres"
  local db_user="${POSTGRES_USER:-postgres}"
  local db_name="${POSTGRES_DB:-${PROJECT_NAME}}"

  if ! docker ps --format "{{.Names}}" | grep -q "^${db_container}$"; then
    cli_error "Database container not running"
    cli_info "Start with: nself start"
    exit 1
  fi

  printf "\n${CLI_BLUE}→${CLI_RESET} Environment: ${CLI_CYAN}$env${CLI_RESET}\n"

  # Determine seed strategy based on environment
  local seed_pattern=""
  case "$env" in
    prod|production)
      # Production: Only critical system seeds (000-001)
      seed_pattern="^(000|001)_.*\\.sql$"
      printf "  Strategy: ${CLI_YELLOW}Production-safe seeds only${CLI_RESET}\n"
      ;;
    staging|stage)
      # Staging: System + basic demo (000-004)
      seed_pattern="^(000|001|002|003|004)_.*\\.sql$"
      printf "  Strategy: ${CLI_CYAN}System + basic demo data${CLI_RESET}\n"
      ;;
    dev|development|*)
      # Development: All seeds
      seed_pattern=".*\\.sql$"
      printf "  Strategy: ${CLI_GREEN}All seeds (full demo)${CLI_RESET}\n"
      ;;
  esac

  printf "\n${CLI_BLUE}→${CLI_RESET} Applying seeds:\n"

  local applied=0
  local skipped=0
  local failed=0

  # Apply seeds in order
  for seed_file in $(find "$seeds_dir" -maxdepth 1 -name "*.sql" -type f | sort); do
    local seed_name=$(basename "$seed_file")

    # Check if seed matches environment pattern
    if echo "$seed_name" | grep -qE "$seed_pattern"; then
      printf "  ${CLI_BLUE}⠋${CLI_RESET} $seed_name..."

      if docker exec -i "$db_container" psql -U "$db_user" -d "$db_name" < "$seed_file" >/dev/null 2>&1; then
        # Count affected rows
        local row_count=$(docker exec "$db_container" psql -U "$db_user" -d "$db_name" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'" 2>/dev/null | tr -d ' ')
        printf "\r  ${CLI_GREEN}✓${CLI_RESET} $seed_name\n"
        ((applied++))
      else
        printf "\r  ${CLI_RED}✗${CLI_RESET} $seed_name (FAILED)\n"
        ((failed++))
      fi
    else
      printf "  ${CLI_DIM}○${CLI_RESET} $seed_name ${CLI_DIM}(skipped for $env)${CLI_RESET}\n"
      ((skipped++))
    fi
  done

  # Summary
  printf "\n"
  if [[ $failed -gt 0 ]]; then
    cli_error "Seeding failed"
    printf "  Applied: $applied\n"
    printf "  Failed: $failed\n"
    exit 1
  elif [[ $applied -eq 0 ]]; then
    cli_warning "No seeds applied"
    if [[ $skipped -gt 0 ]]; then
      printf "  Skipped: $skipped (not for $env environment)\n"
    fi
  else
    cli_success "Seeding complete"
    printf "  Applied: $applied seed(s)\n"
    if [[ $skipped -gt 0 ]]; then
      printf "  Skipped: $skipped (environment-filtered)\n"
    fi
  fi

  printf "\n"
  cli_info "Database ready for: $env"
}

# Export function
export -f seed_database
