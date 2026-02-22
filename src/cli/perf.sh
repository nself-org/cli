#!/usr/bin/env bash
# perf.sh - Performance & Optimization
# Consolidated command including: bench, scale, migrate subcommands

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source utilities (save SCRIPT_DIR before it gets redefined by sourced files)
CLI_DIR="$SCRIPT_DIR"
source "$CLI_DIR/../lib/utils/cli-output.sh"
source "$CLI_DIR/../lib/utils/env.sh"
source "$CLI_DIR/../lib/utils/docker.sh"
source "$CLI_DIR/../lib/utils/header.sh"
source "$CLI_DIR/../lib/hooks/pre-command.sh"
source "$CLI_DIR/../lib/hooks/post-command.sh"

# Source migration libraries
source "$CLI_DIR/../lib/migrate/firebase.sh" 2>/dev/null || true
source "$CLI_DIR/../lib/migrate/supabase.sh" 2>/dev/null || true

# =============================================================================
# HELP TEXT
# =============================================================================

show_help() {
  cat <<'EOF'
nself perf - Performance & Optimization

USAGE:
  nself perf <subcommand> [options]

SUBCOMMANDS:
  profile [service] [options]    Profile service performance
  bench <target> [options]       Benchmark performance
  scale <service> [options]      Scale service resources
  migrate <source> <target>      Migrate between environments/vendors
  optimize [--auto-fix]          Optimization suggestions

PROFILE OPTIONS:
  --duration N                   Profiling duration in seconds (default: 60)

BENCH COMMANDS:
  run [target]                   Run benchmark against target
  baseline                       Establish performance baseline
  compare [file]                 Compare current performance against baseline
  stress [target]                Run stress test (high load)
  report                         Generate benchmark report

BENCH OPTIONS:
  --requests N                   Number of requests (default: 1000)
  --concurrency N                Concurrent connections (default: 10)
  --duration N                   Test duration in seconds (default: 30)
  --output FILE                  Save results to file
  --json                         Output in JSON format

SCALE OPTIONS:
  --cpu <limit>                  Set CPU limit (e.g., 2, 1.5, 0.5)
  --memory <limit>               Set memory limit (e.g., 2G, 512M)
  --replicas <n>                 Set number of replicas
  --list                         List current resource allocations

MIGRATE COMMANDS:
  <source> <target>              Migrate from source to target environment
  from <vendor>                  Migrate from vendor (firebase, supabase)
  diff <source> <target>         Show differences between environments
  sync <source> <target>         Keep environments in sync
  rollback                       Rollback last migration

MIGRATE OPTIONS:
  --dry-run                      Preview migration without making changes
  --schema-only                  Migrate only database schema
  --data-only                    Migrate only data
  --force                        Skip confirmation prompts

EXAMPLES:
  # Profiling
  nself perf profile postgres --duration 120

  # Benchmarking
  nself perf bench run api
  nself perf bench baseline
  nself perf bench stress api --duration 60

  # Scaling
  nself perf scale postgres --memory 4G --cpu 2
  nself perf scale hasura --replicas 3
  nself perf scale --list

  # Migration
  nself perf migrate local staging
  nself perf migrate from firebase
  nself perf migrate diff staging prod

  # Optimization
  nself perf optimize --auto-fix

For more information: https://docs.nself.org/performance
EOF
}

# =============================================================================
# PROFILE SUBCOMMAND
# =============================================================================

cmd_profile() {
  local service="${1:-}"
  local duration="${PROFILE_DURATION:-60}"

  if [[ -z "$service" ]]; then
    cli_error "Service name required"
    printf "\n"
    printf "Usage: nself perf profile <service> [--duration N]\n"
    return 1
  fi

  cli_section "Profiling $service"
  cli_info "Duration: ${duration}s"
  printf "\n"

  # Check if service is running
  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  if ! docker ps --format "{{.Names}}" | grep -q "${project_name}_${service}"; then
    cli_error "Service not running: $service"
    return 1
  fi

  # Collect performance metrics
  cli_info "Collecting performance metrics..."
  docker stats --no-stream "${project_name}_${service}"

  printf "\n"
  cli_success "Profile complete"
}

# =============================================================================
# BENCH SUBCOMMAND (from bench.sh)
# =============================================================================

# Include bench.sh functionality
source "$CLI_DIR/bench.sh" 2>/dev/null || true

cmd_bench() {
  # Delegate to bench command
  if declare -f cmd_bench >/dev/null 2>&1; then
    command cmd_bench "$@"
  else
    cli_error "Bench functionality not available"
    return 1
  fi
}

# =============================================================================
# SCALE SUBCOMMAND (from scale.sh)
# =============================================================================

# Include scale.sh functionality
source "$CLI_DIR/scale.sh" 2>/dev/null || true

cmd_scale() {
  # Delegate to scale command
  if declare -f cmd_scale >/dev/null 2>&1; then
    command cmd_scale "$@"
  else
    cli_error "Scale functionality not available"
    return 1
  fi
}

# =============================================================================
# MIGRATE SUBCOMMAND (from migrate.sh)
# =============================================================================

# Include migrate.sh functionality
source "$CLI_DIR/migrate.sh" 2>/dev/null || true

cmd_migrate() {
  # Delegate to migrate command
  if declare -f cmd_migrate >/dev/null 2>&1; then
    command cmd_migrate "$@"
  else
    cli_error "Migrate functionality not available"
    return 1
  fi
}

# =============================================================================
# OPTIMIZE SUBCOMMAND
# =============================================================================

cmd_optimize() {
  local auto_fix="${AUTO_FIX:-false}"

  cli_section "Performance Optimization Analysis"
  printf "\n"

  cli_info "Analyzing database performance..."

  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  if docker ps --format "{{.Names}}" | grep -q "${project_name}_postgres"; then
    # Run ANALYZE and VACUUM
    if [[ "$auto_fix" == "true" ]]; then
      cli_info "Optimizing database..."
      docker exec "${project_name}_postgres" psql -U "${POSTGRES_USER:-postgres}" \
        -d "${POSTGRES_DB:-nhost}" -c "ANALYZE" >/dev/null 2>&1
      docker exec "${project_name}_postgres" psql -U "${POSTGRES_USER:-postgres}" \
        -d "${POSTGRES_DB:-nhost}" -c "VACUUM ANALYZE" >/dev/null 2>&1
      cli_success "Database optimized"
    else
      cli_info "Run with --auto-fix to apply optimizations"
    fi
  fi

  printf "\n"
  cli_success "Analysis complete"
}

# =============================================================================
# MAIN COMMAND ROUTER
# =============================================================================

main() {
  local subcommand="${1:-help}"

  # Check for help
  if [[ "$subcommand" == "-h" ]] || [[ "$subcommand" == "--help" ]] || [[ "$subcommand" == "help" ]]; then
    show_help
    return 0
  fi

  # Parse global options
  local args=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --duration)
        PROFILE_DURATION="$2"
        BENCH_DURATION="$2"
        shift 2
        ;;
      --auto-fix)
        AUTO_FIX=true
        shift
        ;;
      --requests)
        BENCH_REQUESTS="$2"
        shift 2
        ;;
      --concurrency)
        BENCH_CONCURRENCY="$2"
        shift 2
        ;;
      --output)
        OUTPUT_FILE="$2"
        shift 2
        ;;
      --json)
        JSON_OUTPUT=true
        shift
        ;;
      --cpu)
        CPU_LIMIT="$2"
        shift 2
        ;;
      --memory)
        MEMORY_LIMIT="$2"
        shift 2
        ;;
      --replicas)
        REPLICAS="$2"
        shift 2
        ;;
      --list)
        LIST_MODE=true
        shift
        ;;
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
      --force)
        FORCE=true
        shift
        ;;
      -h | --help)
        show_help
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
  subcommand="${1:-help}"

  # Route to subcommand
  case "$subcommand" in
    profile)
      shift
      cmd_profile "$@"
      ;;
    bench)
      shift
      cmd_bench "$@"
      ;;
    scale)
      shift
      cmd_scale "$@"
      ;;
    migrate)
      shift
      cmd_migrate "$@"
      ;;
    optimize)
      cmd_optimize
      ;;
    help | -h | --help)
      show_help
      ;;
    *)
      cli_error "Unknown subcommand: $subcommand"
      printf "\n"
      show_help
      return 1
      ;;
  esac
}

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_help
      exit 0
    fi
  done
  printf "\033[0;33m⚠\033[0m  WARNING: 'nself perf' is deprecated.\n" >&2
  printf "   Use: nself service bench|scale|profile|optimize  or  nself db migrate\n\n" >&2
  pre_command "perf" || exit $?
  main "$@"
  exit_code=$?
  post_command "perf" $exit_code
  exit $exit_code
fi
