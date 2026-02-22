#!/usr/bin/env bash

# pre-command.sh - Pre-command validation and setup hooks

# Source utilities (namespaced to avoid clobbering caller globals)
_HOOKS_SHARED_DIR="$(dirname "${BASH_SOURCE[0]}")/.."

set -euo pipefail

source "$_HOOKS_SHARED_DIR/utils/display.sh" 2>/dev/null || true
source "$_HOOKS_SHARED_DIR/utils/docker.sh" 2>/dev/null || true
source "$_HOOKS_SHARED_DIR/utils/env.sh" 2>/dev/null || true

# Pre-command validation
pre_command() {
  local command="$1"

  log_debug "Running pre-command hooks for: $command"

  # Check if we're in the nself repository itself
  check_not_in_nself_repo "$command"

  # Commands that don't need project initialization
  local no_init_commands="init help version update reset checklist doctor completion"

  # Check if project is initialized (unless exempt)
  if [[ ! " $no_init_commands " =~ " $command " ]]; then
    # Monorepo auto-detection: if no .env files here but backend/ has them, cd into it
    local has_env_here=false
    if [[ -f ".env.dev" ]] || [[ -f ".env.local" ]] || [[ -f ".env" ]]; then
      has_env_here=true
    fi

    if [[ "$has_env_here" == "false" ]] && [[ -d "backend" ]]; then
      if [[ -f "backend/.env.dev" ]] || [[ -f "backend/.env.local" ]] || [[ -f "backend/.env" ]]; then
        log_info "Monorepo detected — switching to backend/ directory"
        cd backend
        has_env_here=true
      fi
    fi

    # Check if any .env file exists (project is initialized)
    if [[ "$has_env_here" == "false" ]]; then
      log_error "Project not initialized. Run 'nself init' first."
      return 1
    fi

    # Load environment
    load_env_with_priority || return 1
  fi

  # Commands that need Docker
  local docker_commands="up down stop restart status logs doctor db build"

  if [[ " $docker_commands " =~ " $command " ]]; then
    ensure_docker_running || return 1
  fi

  # Ensure required directories exist
  ensure_directories

  # Setup logging
  setup_command_logging "$command"

  return 0
}

# Check if we're in the nself repository itself
check_not_in_nself_repo() {
  local command="$1"

  # Commands that are safe to run in nself repo
  local safe_commands="help version update doctor"

  # Skip check for safe commands
  if [[ " $safe_commands " =~ " $command " ]]; then
    return 0
  fi

  # Check for telltale signs we're in the nself repo root (not subdirectories)
  if [[ -f "bin/nself" ]] && [[ -d "src/lib" ]] && [[ -d "src/cli" ]] && [[ -f "install.sh" ]]; then
    log_warning "You appear to be in the nself repository root!"
    log_error "nself commands should be run in your project directory, not the nself source root."
    echo ""
    log_info "To use nself:"
    log_info "  1. Create a project directory: mkdir ~/myproject && cd ~/myproject"
    log_info "  2. Initialize the project: nself init"
    log_info "  3. Build and run: nself build && nself start"
    return 1
  fi

  return 0
}

# Ensure required directories exist
ensure_directories() {
  # Skip if we're in nself repo (safety check)
  if [[ -f "bin/nself" ]] && [[ -d "src/lib" ]] && [[ -d "src/cli" ]]; then
    return 0
  fi

  local dirs=(
    "logs"
    "certs/local"
    "hasura/migrations"
    "hasura/metadata"
    "bin/dbsyncs"
  )

  for dir in "${dirs[@]}"; do
    [[ ! -d "$dir" ]] && mkdir -p "$dir"
  done
}

# Setup command logging
setup_command_logging() {
  local command="$1"
  local log_file="logs/nself.log"

  # Create log directory if needed
  mkdir -p "$(dirname "$log_file")"

  # Log command execution
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] Executing: nself $command $*" >>"$log_file"
}

# Check system prerequisites
check_prerequisites() {
  local errors=0

  # Check Docker
  if ! command -v docker >/dev/null 2>&1; then
    log_error "Docker is not installed"
    errors=$((errors + 1))
  fi

  # Check Docker Compose v2
  if ! docker compose version >/dev/null 2>&1; then
    log_error "Docker Compose v2 is not available"
    errors=$((errors + 1))
  fi

  # Check disk space (warn if < 1GB)
  local available_space=$(df . | awk 'NR==2 {print $4}')
  if [[ $available_space -lt 1048576 ]]; then
    log_warning "Low disk space: less than 1GB available"
  fi

  return $errors
}

# Export functions
export -f pre_command ensure_directories setup_command_logging check_prerequisites check_not_in_nself_repo
