#!/usr/bin/env bash


# scanner.sh - Comprehensive error scanning orchestrator

# Source all error handlers
ERROR_LIB_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

source "$ERROR_LIB_DIR/base.sh"
source "$ERROR_LIB_DIR/handlers/ports.sh"
source "$ERROR_LIB_DIR/handlers/docker.sh"

# Source display utilities if not already loaded
if ! declare -f log_info >/dev/null; then
  source "$ERROR_LIB_DIR/../utils/display.sh"
fi

# Comprehensive system scan
run_comprehensive_scan() {
  local context="${1:-startup}"
  local auto_fix="${2:-true}"

  log_header "System Error Scan"

  # Initialize error tracking
  init_error_handling

  # Phase 1: Critical checks (must pass)
  log_info "Phase 1: Critical System Checks"
  echo ""

  # Docker daemon must be running
  check_docker_daemon

  # docker-compose.yml must exist and be valid
  if [[ "$context" != "init" ]]; then
    check_docker_compose
  fi

  # Stop if critical errors found
  if [[ $CRITICAL_ERRORS -gt 0 ]]; then
    display_critical_errors

    if [[ "$auto_fix" == "true" ]]; then
      log_info "Attempting to fix critical errors..."
      run_auto_fixes "false" # Non-interactive in scan

      # Re-check critical items
      init_error_handling
      check_docker_daemon

      if [[ $CRITICAL_ERRORS -gt 0 ]]; then
        log_error "Critical errors remain after auto-fix attempts"
        show_error_summary
        return 1
      fi
    else
      show_error_summary
      return 1
    fi
  fi

  echo ""
  log_success "✓ Critical checks passed"

  # Phase 2: Service checks (can be fixed)
  echo ""
  log_info "Phase 2: Service Configuration Checks"
  echo ""

  # Port conflicts
  scan_port_conflicts

  # Docker resources
  check_docker_disk_space
  check_docker_images

  # Container health (if running)
  if docker compose ps --quiet 2>/dev/null | grep -q .; then
    check_container_health
  fi

  # Phase 3: Configuration checks
  echo ""
  log_info "Phase 3: Configuration Checks"
  echo ""

  check_configuration_files
  check_environment_variables

  # Show summary
  echo ""
  show_error_summary

  # Handle fixes
  if [[ $FIXABLE_ERRORS -gt 0 ]] && [[ "$auto_fix" == "true" ]]; then
    echo ""
    run_auto_fixes

    # If we fixed port conflicts, need to rebuild
    if [[ -f ".needs-rebuild" ]]; then
      log_info "Configuration changed, rebuild required"
      rm -f ".needs-rebuild"
      return 2 # Special code for needs rebuild
    fi
  fi

  # Determine if we can continue
  if should_continue; then
    return 0
  else
    return 1
  fi
}

# Display critical errors prominently
display_critical_errors() {
  echo ""
  log_error "═══════════════════════════════════════"
  log_error "     CRITICAL ERRORS DETECTED"
  log_error "═══════════════════════════════════════"
  echo ""

  for error_code in "${!ERROR_REGISTRY[@]}"; do
    if [[ "${ERROR_REGISTRY[${error_code}_severity]}" == "$ERROR_CRITICAL" ]]; then
      display_error "$error_code"
    fi
  done

  echo ""
}

# Check configuration files
check_configuration_files() {
  log_info "Checking configuration files..."

  local required_files=(
    ".env.local"
    "docker-compose.yml"
  )

  local missing=0
  for file in "${required_files[@]}"; do
    if [[ ! -f "$file" ]]; then
      log_warning "Missing: $file"
      missing=$((missing + 1))
    else
      log_debug "✓ Found: $file"
    fi
  done

  if [[ $missing -gt 0 ]]; then
    register_error "CONFIG_MISSING" \
      "$missing configuration file(s) missing" \
      $ERROR_MAJOR \
      "true" \
      "fix_missing_config"
  else
    log_success "All configuration files present"
  fi
}

# Fix missing configuration
fix_missing_config() {
  if [[ ! -f ".env.local" ]]; then
    log_info "Creating .env.local..."
    if bash "$SCRIPT_DIR/../cli/init.sh"; then
      log_success "Created .env.local"
    else
      return 1
    fi
  fi

  if [[ ! -f "docker-compose.yml" ]]; then
    log_info "Generating docker-compose.yml..."
    if bash "$SCRIPT_DIR/../cli/build.sh"; then
      log_success "Generated docker-compose.yml"
    else
      return 1
    fi
  fi

  return 0
}

# Check environment variables
check_environment_variables() {
  log_info "Checking environment variables..."

  if [[ ! -f ".env.local" ]]; then
    return 0 # Already handled
  fi

  # Source env file
  set -a
  source .env.local
  set +a

  # Check critical variables
  local required_vars=(
    "BASE_DOMAIN"
    "POSTGRES_PASSWORD"
    "JWT_SECRET"
    "HASURA_ADMIN_SECRET"
  )

  local missing=()
  for var in "${required_vars[@]}"; do
    if [[ -z "${!var}" ]]; then
      missing+=("$var")
    fi
  done

  if [[ ${#missing[@]} -gt 0 ]]; then
    register_error "ENV_INCOMPLETE" \
      "${#missing[@]} required environment variable(s) not set" \
      $ERROR_MAJOR \
      "false"

    for var in "${missing[@]}"; do
      log_warning "Missing: $var"
    done
  else
    log_success "All required environment variables set"
  fi
}

# Quick scan for specific context
quick_scan() {
  local scan_type="$1"

  init_error_handling

  case "$scan_type" in
    ports)
      scan_port_conflicts
      ;;
    docker)
      check_docker_daemon
      check_docker_disk_space
      ;;
    config)
      check_configuration_files
      check_environment_variables
      ;;
    health)
      check_container_health
      ;;
    *)
      log_error "Unknown scan type: $scan_type"
      return 1
      ;;
  esac

  show_error_summary
  return $?
}

# Export functions
export -f run_comprehensive_scan
export -f display_critical_errors
export -f check_configuration_files
export -f fix_missing_config
export -f check_environment_variables
export -f quick_scan
