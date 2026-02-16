#!/usr/bin/env bash

# core.sh - Auto-fix decision engine

# List of fixable errors (limited scope)
readonly AUTO_FIXABLE_ERRORS=(

set -euo pipefail

  "docker_build"
  "port_conflict_self"
  "dependency_missing"
  "config_missing"
)

# Main auto-fix decision function
auto_fix_decision() {
  local error_type="$1"
  local error_details="$2"

  # Check if error is auto-fixable
  for fixable in "${AUTO_FIXABLE_ERRORS[@]}"; do
    if [[ "$error_type" == "$fixable" ]]; then
      return 0 # Can auto-fix
    fi
  done

  return 1 # Cannot auto-fix
}

# Attempt to fix an error
attempt_auto_fix() {
  local error_type="$1"
  local error_details="$2"

  # Get absolute path to auto-fix directory
  local AF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  case "$error_type" in
    docker_build)
      source "$AF_DIR/docker.sh"
      fix_docker_build "$error_details"
      ;;
    port_conflict_self)
      source "$AF_DIR/ports.sh"
      fix_port_conflict "$error_details"
      ;;
    dependency_missing)
      source "$AF_DIR/dependencies.sh"
      fix_missing_dependency "$error_details"
      ;;
    config_missing)
      source "$AF_DIR/config.sh"
      fix_missing_config "$error_details"
      ;;
    *)
      return 1
      ;;
  esac
}

export -f auto_fix_decision
export -f attempt_auto_fix
