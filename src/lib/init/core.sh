#!/usr/bin/env bash


# Source platform compatibility for safe_sed_inline
# (use INIT_MODULE_DIR below for module sourcing; this is just for the compat fallback)
_INIT_CORE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$_INIT_CORE_DIR/../utils/platform-compat.sh" 2>/dev/null || source "$_INIT_CORE_DIR/../../lib/utils/platform-compat.sh" 2>/dev/null || {
  # Fallback definition
  safe_sed_inline() {
    local file="$1"
    shift
    if [[ "$OSTYPE" == "darwin"* ]]; then
      sed -i "" "$@" "$file"
    else
      sed -i "$@" "$file"
    fi
  }
}

# core.sh - Core orchestration logic for nself init command
#
# This module contains the main initialization logic that coordinates
# all other modules to perform the init operation.

# Get module directory
INIT_MODULE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source all required modules with error checking
source "$INIT_MODULE_DIR/config.sh" || {
  echo "Error: Failed to load config module" >&2
  exit 78
}
source "$INIT_MODULE_DIR/platform.sh" || {
  echo "Error: Failed to load platform module" >&2
  exit 78
}

# Try to source the nice display utilities if available
if [[ -f "$INIT_MODULE_DIR/../utils/display.sh" ]]; then
  source "$INIT_MODULE_DIR/../utils/display.sh" || true
fi

source "$INIT_MODULE_DIR/validation.sh" || {
  echo "Error: Failed to load validation module" >&2
  exit 78
}
source "$INIT_MODULE_DIR/atomic-ops.sh" || {
  echo "Error: Failed to load atomic-ops module" >&2
  exit 78
}
source "$INIT_MODULE_DIR/templates.sh" || {
  echo "Error: Failed to load templates module" >&2
  exit 78
}
source "$INIT_MODULE_DIR/gitignore.sh" || {
  echo "Error: Failed to load gitignore module" >&2
  exit 78
}
source "$INIT_MODULE_DIR/help.sh" || {
  echo "Error: Failed to load help module" >&2
  exit 78
}

# Demo module is optional - set flag based on availability
DEMO_MODE_AVAILABLE=false
if [[ -f "$INIT_MODULE_DIR/demo.sh" ]]; then
  source "$INIT_MODULE_DIR/demo.sh" && DEMO_MODE_AVAILABLE=true
fi

# State tracking
INIT_STATE="$INIT_STATE_IDLE"

# Validate created environment file has required variables
# Inputs: $1 - env file path (defaults to .env)
# Outputs: Error messages if validation fails
# Returns: 0 on success, 1 on failure
validate_env_config() {
  local env_file="${1:-.env}"
  local missing_critical=()
  local validation_passed=true

  # Only validate if file exists
  [[ -f "$env_file" ]] || return 0

  # Check Tier 1 critical variables (absolutely required)
  local critical_vars=(
    "PROJECT_NAME"
    "BASE_DOMAIN"
    "ENV"
  )

  for var in "${critical_vars[@]}"; do
    if ! grep -q "^${var}=" "$env_file" 2>/dev/null; then
      missing_critical+=("$var")
      validation_passed=false
    fi
  done

  # Report validation results
  if [[ "$validation_passed" == false ]]; then
    if [[ "${QUIET_MODE:-false}" != "true" ]]; then
      log_warning "Configuration needs these variables set in $env_file:"
      for var in "${missing_critical[@]}"; do
        echo "  - $var"
      done
      echo ""
      echo "Edit $env_file and uncomment/set these values before running 'nself build'"
    fi
    return 1
  fi

  return 0
}

# Ensure env file has working defaults
# Inputs: $1 - env file path
# Outputs: Updates file with critical defaults if missing
# Returns: 0 on success
ensure_working_defaults() {
  local env_file="${1:-.env}"

  # Only process if file exists
  [[ -f "$env_file" ]] || return 0

  # Check and add critical defaults if completely missing
  local needs_update=false

  # Check if PROJECT_NAME is set
  if ! grep -q "^PROJECT_NAME=" "$env_file" 2>/dev/null; then
    if grep -q "^# PROJECT_NAME=" "$env_file" 2>/dev/null; then
      # Uncomment existing commented line
      # Use platform-safe sed
      if [[ "$OSTYPE" == "darwin"* ]]; then
        safe_sed_inline 's/^# PROJECT_NAME=.*/PROJECT_NAME=myproject/' "$env_file"
      else
        safe_sed_inline 's/^# PROJECT_NAME=.*/PROJECT_NAME=myproject/' "$env_file"
      fi
      needs_update=true
    else
      # Add new line
      echo "PROJECT_NAME=myproject" >>"$env_file"
      needs_update=true
    fi
  fi

  # Check if BASE_DOMAIN is set
  if ! grep -q "^BASE_DOMAIN=" "$env_file" 2>/dev/null; then
    if grep -q "^# BASE_DOMAIN=" "$env_file" 2>/dev/null; then
      # Use platform-safe sed
      if [[ "$OSTYPE" == "darwin"* ]]; then
        safe_sed_inline 's/^# BASE_DOMAIN=.*/BASE_DOMAIN=local.nself.org/' "$env_file"
      else
        safe_sed_inline 's/^# BASE_DOMAIN=.*/BASE_DOMAIN=local.nself.org/' "$env_file"
      fi
      needs_update=true
    else
      echo "BASE_DOMAIN=local.nself.org" >>"$env_file"
      needs_update=true
    fi
  fi

  # Check if ENV is set
  if ! grep -q "^ENV=" "$env_file" 2>/dev/null; then
    if grep -q "^# ENV=" "$env_file" 2>/dev/null; then
      # Use platform-safe sed
      if [[ "$OSTYPE" == "darwin"* ]]; then
        safe_sed_inline 's/^# ENV=.*/ENV=dev/' "$env_file"
      else
        safe_sed_inline 's/^# ENV=.*/ENV=dev/' "$env_file"
      fi
      needs_update=true
    else
      echo "ENV=dev" >>"$env_file"
      needs_update=true
    fi
  fi

  # Clean up backup files
  rm -f "${env_file}.bak" 2>/dev/null || true

  if [[ "$needs_update" == true ]] && [[ "${QUIET_MODE:-false}" != "true" ]]; then
    log_info "Added critical defaults to $env_file"
  fi

  return 0
}

# Initialize project with basic setup
# Inputs: $1 - script directory
# Outputs: Creates basic project files
# Returns: 0 on success, error code on failure
init_basic() {
  local script_dir="$1"

  # Find templates directory
  local templates_dir
  templates_dir=$(find_templates_dir "$script_dir") || return $?

  # Copy basic templates
  copy_basic_templates "$templates_dir" "$QUIET_MODE" || return $?

  # Ensure gitignore is properly configured
  ensure_gitignore || return $?

  # Ensure working defaults are present
  ensure_working_defaults ".env"

  # Validate configuration
  validate_env_config ".env"

  return $INIT_E_SUCCESS
}

# Initialize project with full setup
# Inputs: $1 - script directory
# Outputs: Creates all project files
# Returns: 0 on success, error code on failure
init_full() {
  local script_dir="$1"

  # Find templates directory
  local templates_dir
  templates_dir=$(find_templates_dir "$script_dir") || return $?

  # Copy all templates
  copy_full_templates "$templates_dir" "$QUIET_MODE" || return $?

  # Ensure gitignore is properly configured
  ensure_gitignore || return $?

  # Ensure working defaults are present
  ensure_working_defaults ".env"
  ensure_working_defaults ".env.dev"

  # Validate configuration
  validate_env_config ".env"

  return $INIT_E_SUCCESS
}

# Run the configuration wizard
# Inputs: None (uses INIT_MODULE_DIR)
# Outputs: Runs interactive wizard
# Returns: Exit code from wizard
run_wizard() {
  local wizard_script="$INIT_MODULE_DIR/wizard/init-wizard.sh"

  if [[ -f "$wizard_script" ]]; then
    source "$wizard_script" || {
      log_error "Failed to load wizard"
      return $INIT_E_CONFIG
    }
    run_config_wizard
    return $?
  else
    log_error "Wizard not found at $wizard_script"
    return $INIT_E_CONFIG
  fi
}

# Setup minimal admin environment
# Inputs: $1 - script directory
# Outputs: Creates minimal admin configuration
# Returns: Exit code from admin setup
setup_admin() {
  local script_dir="$1"
  local admin_script="$script_dir/admin.sh"

  if [[ -f "$admin_script" ]]; then
    source "$admin_script" || {
      log_error "Failed to load admin module"
      return $INIT_E_CONFIG
    }
    admin_minimal_setup
    return $?
  else
    log_error "Admin module not found"
    return $INIT_E_CONFIG
  fi
}

# Display next steps after successful init
# Inputs: $1 - full setup flag
# Outputs: Next steps message
# Returns: 0
show_next_steps() {
  local full_setup="${1:-false}"

  echo ""
  safe_echo "${COLOR_CYAN:-}${ARROW:-➞} Next Steps${COLOR_RESET:-}"
  echo ""
  safe_echo "${COLOR_BLUE:-}1.${COLOR_RESET:-} Edit .env to customize (optional)"
  safe_echo "   ${COLOR_DIM:-}Set project name, defaults handle the rest${COLOR_RESET:-}"
  echo ""
  safe_echo "${COLOR_BLUE:-}2.${COLOR_RESET:-} nself build - Generate project files"
  safe_echo "   ${COLOR_DIM:-}Creates Docker configs and services${COLOR_RESET:-}"
  echo ""
  safe_echo "${COLOR_BLUE:-}3.${COLOR_RESET:-} nself start - Start your backend"
  safe_echo "   ${COLOR_DIM:-}Launches all configured services${COLOR_RESET:-}"
  echo ""

  # Add help line at bottom
  echo "For more help, use: nself help or nself help init"
  echo ""

  return 0
}

# Main init command function
# Inputs: Command line arguments
# Outputs: Initializes project based on arguments
# Returns: 0 on success, error code on failure
cmd_init() {
  local full_setup=false
  local force_init=false
  local quiet_mode=false
  local script_dir="$1"
  shift # Remove script_dir from arguments

  # Initialize platform and display
  init_platform

  # Set globals for other modules
  export QUIET_MODE="$quiet_mode"
  export FORCE_INIT="$force_init"

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case $1 in
      --full)
        full_setup=true
        shift
        ;;
      --wizard)
        INIT_STATE="$INIT_STATE_IN_PROGRESS"
        run_wizard
        local wizard_result=$?
        INIT_STATE="$INIT_STATE_COMPLETED"
        return $wizard_result
        ;;
      --demo)
        INIT_STATE="$INIT_STATE_IN_PROGRESS"
        setup_demo "$script_dir"
        local demo_result=$?
        INIT_STATE="$INIT_STATE_COMPLETED"
        return $demo_result
        ;;
      --admin)
        INIT_STATE="$INIT_STATE_IN_PROGRESS"
        setup_admin "$script_dir"
        local admin_result=$?
        INIT_STATE="$INIT_STATE_COMPLETED"
        return $admin_result
        ;;
      --force)
        force_init=true
        FORCE_INIT=true
        shift
        ;;
      --quiet | -q)
        quiet_mode=true
        QUIET_MODE=true
        shift
        ;;
      -h | --help)
        show_init_help
        return $INIT_E_SUCCESS
        ;;
      *)
        log_error "Unknown option: $1"
        echo "Use 'nself init --help' for usage information" >&2
        return $INIT_E_MISUSE
        ;;
    esac
  done

  # Perform validations first (checking existing config, etc)
  # This needs to happen before showing the header to avoid showing it on error
  # We'll do a quick check for existing config here
  if [[ -f ".env" ]] && [[ "$force_init" != true ]] && [[ "$quiet_mode" != true ]]; then
    # Show header for context
    show_command_header "nself init" "Initialize a new full-stack application"
    # The validation will show the error message
    perform_all_validations "$force_init" "$quiet_mode" || return $?
  else
    # Show header unless quiet (normal flow)
    if [[ "$quiet_mode" != true ]]; then
      show_command_header "nself init" "Initialize a new full-stack application"
      echo "" # Add blank line after header
    fi
    # Perform validations (will show single line unless quiet)
    perform_all_validations "$force_init" "$quiet_mode" || return $?
  fi

  # Set init state
  INIT_STATE="$INIT_STATE_IN_PROGRESS"

  # Perform initialization
  local init_result
  if [[ "$full_setup" == true ]]; then
    init_full "$script_dir"
    init_result=$?
  else
    init_basic "$script_dir"
    init_result=$?
  fi

  # Handle result
  if [[ $init_result -eq 0 ]]; then
    INIT_STATE="$INIT_STATE_COMPLETED"

    # Clean up backups on success
    cleanup_backups
    cleanup_temp_dir

    # Show next steps unless quiet
    if [[ "$quiet_mode" != true ]]; then
      show_next_steps "$full_setup"
    fi
  else
    INIT_STATE="$INIT_STATE_FAILED"
    return $init_result
  fi

  return $INIT_E_SUCCESS
}

# Cleanup function for error handling
# Inputs: None
# Outputs: Performs cleanup on error
# Returns: 0
init_cleanup() {
  local exit_code=$?

  if [[ $exit_code -ne 0 ]] && [[ "$INIT_STATE" == "$INIT_STATE_IN_PROGRESS" ]]; then
    # Only show rollback message if we were actually doing something
    if [[ ${#CREATED_FILES[@]} -gt 0 ]] || [[ ${#MODIFIED_FILES[@]} -gt 0 ]]; then
      echo "Init failed, rolling back changes..." >&2
      rollback_changes
      INIT_STATE="$INIT_STATE_ROLLED_BACK"
    fi
  fi

  # Always clean up temp directory
  cleanup_temp_dir

  return 0
}

# Export main function
export -f cmd_init init_cleanup
