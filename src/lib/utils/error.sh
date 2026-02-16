#!/usr/bin/env bash

# error.sh - Simplified error handling and classification

# Source display utilities
UTILS_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

source "$UTILS_DIR/display.sh" 2>/dev/null || true

# Error types
readonly ERROR_TYPE_DOCKER="docker"
readonly ERROR_TYPE_PORT="port_conflict"
readonly ERROR_TYPE_DEPENDENCY="dependency"
readonly ERROR_TYPE_CONFIG="config"
readonly ERROR_TYPE_PERMISSION="permission"
readonly ERROR_TYPE_NETWORK="network"
readonly ERROR_TYPE_RESOURCE="resource"

# Simple error handler
handle_error_simple() {
  local exit_code="$1"
  local command="$2"
  local context="${3:-}"

  # Log the error
  log_error "Command failed: $command (exit code: $exit_code)"
  [[ -n "$context" ]] && log_debug "Context: $context"

  # Classify the error
  local error_type=$(classify_error "$exit_code" "$command" "$context")

  # Show user-friendly help
  show_error_help "$error_type" "$command"

  return "$exit_code"
}

# Classify error based on command and context
classify_error() {
  local exit_code="$1"
  local command="$2"
  local context="$3"

  # Docker-related errors
  if [[ "$command" == *"docker"* ]] || [[ "$command" == *"compose"* ]]; then
    if [[ "$context" == *"port"* ]] || [[ "$context" == *"bind"* ]]; then
      echo "port_conflict_self"
    elif [[ "$context" == *"build"* ]]; then
      echo "docker_build"
    elif [[ "$context" == *"not found"* ]] || [[ "$context" == *"Cannot connect"* ]]; then
      echo "docker_not_running"
    else
      echo "docker_general"
    fi
    return
  fi

  # Dependency-related errors
  if [[ "$command" == *"npm"* ]] || [[ "$command" == *"yarn"* ]]; then
    echo "dependency_missing_ts"
    return
  fi

  if [[ "$command" == *"go mod"* ]] || [[ "$command" == *"go build"* ]]; then
    echo "dependency_missing_go"
    return
  fi

  if [[ "$command" == *"pip"* ]] || [[ "$command" == *"python"* ]]; then
    echo "dependency_missing_py"
    return
  fi

  # Configuration errors
  if [[ "$context" == *"env"* ]] || [[ "$context" == *"config"* ]]; then
    echo "config_missing"
    return
  fi

  # Permission errors
  if [[ "$exit_code" -eq 126 ]] || [[ "$context" == *"permission"* ]]; then
    echo "permission_denied"
    return
  fi

  # Default
  echo "unknown"
}

# Show user-friendly error help
show_error_help() {
  local error_type="$1"
  local command="$2"

  echo
  draw_box "ERROR HELP" "error"

  case "$error_type" in
    docker_not_running)
      echo "Docker is not running. Please start Docker:"
      echo "  • macOS: Start Docker Desktop"
      echo "  • Linux: sudo systemctl start docker"
      ;;

    port_conflict_self)
      echo "Port conflict detected with our own containers."
      echo "Try: nself stop && nself start"
      ;;

    port_conflict_external)
      echo "Port is in use by another process."
      echo "Options:"
      echo "  • Stop the other process"
      echo "  • Change the port in .env.local"
      ;;

    dependency_missing_ts)
      echo "TypeScript/Node.js dependencies missing."
      echo "Try: npm install"
      ;;

    dependency_missing_go)
      echo "Go dependencies missing."
      echo "Try: go mod download"
      ;;

    dependency_missing_py)
      echo "Python dependencies missing."
      echo "Try: pip install -r requirements.txt"
      ;;

    config_missing)
      echo "Configuration issue detected."
      echo "Try: nself validate-env --apply-fixes"
      ;;

    permission_denied)
      echo "Permission denied."
      echo "Check:"
      echo "  • File permissions"
      echo "  • Docker group membership"
      echo "  • Directory ownership"
      ;;

    docker_build)
      echo "Docker build failed."
      echo "Check:"
      echo "  • Dockerfile syntax"
      echo "  • Build context"
      echo "  • Available disk space"
      ;;

    *)
      echo "An error occurred. Check the logs above for details."
      ;;
  esac

  echo
}

# Trap and handle errors
setup_error_trap() {
  set -eE # Exit on error, inherit ERR trap
  trap 'handle_error_trap $? "$BASH_COMMAND" "$LINENO"' ERR
}

# Error trap handler
handle_error_trap() {
  local exit_code=$1
  local command="$2"
  local line_number=$3

  log_error "Error on line $line_number: $command (exit: $exit_code)"

  # Cleanup if needed
  if declare -f cleanup_on_error >/dev/null; then
    cleanup_on_error
  fi

  exit "$exit_code"
}

# Cleanup on error (can be overridden)
cleanup_on_error() {
  # Remove temporary files
  rm -f /tmp/nself_*.tmp 2>/dev/null || true

  # Show cursor if hidden
  tput cnorm 2>/dev/null || true
}

# Retry with exponential backoff
retry_with_backoff() {
  local max_attempts="${1:-3}"
  local delay="${2:-1}"
  shift 2

  local attempt=1
  while [[ $attempt -le $max_attempts ]]; do
    if "$@"; then
      return 0
    fi

    if [[ $attempt -lt $max_attempts ]]; then
      log_warning "Attempt $attempt failed, retrying in ${delay}s..."
      sleep "$delay"
      delay=$((delay * 2))
    fi

    ((attempt++))
  done

  log_error "All $max_attempts attempts failed"
  return 1
}

# Export functions
export -f handle_error_simple classify_error show_error_help
export -f setup_error_trap handle_error_trap cleanup_on_error
export -f retry_with_backoff
