#!/usr/bin/env bash

# registry.sh - Nginx project registry for multi-project port management
# Tracks registered projects, their port ranges, and base domains.
# Registry file: ~/.nself/nginx/registry.json

# Prevent double-sourcing
[[ "${NGINX_REGISTRY_SOURCED:-}" == "1" ]] && return 0

set -euo pipefail

export NGINX_REGISTRY_SOURCED=1

# Source display utilities for log_info, log_error, log_success
NGINX_REGISTRY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$NGINX_REGISTRY_DIR/../utils/display.sh" ]]; then
  source "$NGINX_REGISTRY_DIR/../utils/display.sh"
fi

# Registry file path
NSELF_REGISTRY_DIR="${HOME}/.nself/nginx"
NSELF_REGISTRY_FILE="${NSELF_REGISTRY_DIR}/registry.json"

# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------

# Check that jq is available
_registry::require_jq() {
  if ! command -v jq >/dev/null 2>&1; then
    log_error "jq is required but not installed"
    return 1
  fi
  return 0
}

# Validate the registry file contains valid JSON
_registry::validate() {
  if [[ ! -f "$NSELF_REGISTRY_FILE" ]]; then
    return 1
  fi
  if ! jq empty "$NSELF_REGISTRY_FILE" 2>/dev/null; then
    log_error "Registry file is corrupt: $NSELF_REGISTRY_FILE"
    return 1
  fi
  return 0
}

# Ensure registry exists, creating it if needed. Returns 1 on failure.
_registry::ensure() {
  _registry::require_jq || return 1

  if [[ ! -f "$NSELF_REGISTRY_FILE" ]]; then
    registry::init || return 1
  fi

  _registry::validate || return 1
  return 0
}

# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------

# Initialize the registry directory and file.
# Creates ~/.nself/nginx/ and an empty registry.json if they do not exist.
registry::init() {
  _registry::require_jq || return 1

  if [[ ! -d "$NSELF_REGISTRY_DIR" ]]; then
    mkdir -p "$NSELF_REGISTRY_DIR"
  fi

  if [[ ! -f "$NSELF_REGISTRY_FILE" ]]; then
    printf '%s\n' '{"version":1,"portBase":10000,"projects":[]}' > "$NSELF_REGISTRY_FILE"
    log_info "Initialized nginx registry at $NSELF_REGISTRY_FILE"
  fi

  return 0
}

# Add a project to the registry.
# Args: name path base_domain port_start port_end
# Fails if a project with the same name already exists.
registry::add_project() {
  local name="${1:?registry::add_project requires name}"
  local path="${2:?registry::add_project requires path}"
  local base_domain="${3:?registry::add_project requires base_domain}"
  local port_start="${4:?registry::add_project requires port_start}"
  local port_end="${5:?registry::add_project requires port_end}"

  _registry::ensure || return 1

  # Check for duplicate name
  local existing
  existing=$(jq -r --arg n "$name" '.projects[] | select(.name == $n) | .name' "$NSELF_REGISTRY_FILE")
  if [[ -n "$existing" ]]; then
    log_error "Project '$name' is already registered"
    return 1
  fi

  # Build the ISO-8601 timestamp (portable: date -u works on both macOS and Linux)
  local timestamp
  timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

  # Add the project entry
  local tmp_file
  tmp_file=$(mktemp "${TMPDIR:-/tmp}/nself-registry.XXXXXX")

  if ! jq --arg n "$name" \
       --arg p "$path" \
       --arg bd "$base_domain" \
       --argjson ps "$port_start" \
       --argjson pe "$port_end" \
       --arg ts "$timestamp" \
       '.projects += [{"name":$n,"path":$p,"baseDomain":$bd,"portStart":$ps,"portEnd":$pe,"registeredAt":$ts}]' \
       "$NSELF_REGISTRY_FILE" > "$tmp_file"; then
    rm -f "$tmp_file"
    log_error "Failed to add project '$name' to registry"
    return 1
  fi

  mv "$tmp_file" "$NSELF_REGISTRY_FILE"
  log_success "Registered project '$name' (ports ${port_start}-${port_end})"
  return 0
}

# Remove a project from the registry by name.
# Returns 1 if the project is not found.
registry::remove_project() {
  local name="${1:?registry::remove_project requires name}"

  _registry::ensure || return 1

  # Verify the project exists
  local existing
  existing=$(jq -r --arg n "$name" '.projects[] | select(.name == $n) | .name' "$NSELF_REGISTRY_FILE")
  if [[ -z "$existing" ]]; then
    log_error "Project '$name' is not registered"
    return 1
  fi

  local tmp_file
  tmp_file=$(mktemp "${TMPDIR:-/tmp}/nself-registry.XXXXXX")

  if ! jq --arg n "$name" '.projects |= map(select(.name != $n))' \
       "$NSELF_REGISTRY_FILE" > "$tmp_file"; then
    rm -f "$tmp_file"
    log_error "Failed to remove project '$name' from registry"
    return 1
  fi

  mv "$tmp_file" "$NSELF_REGISTRY_FILE"
  log_success "Removed project '$name' from registry"
  return 0
}

# List all registered projects as a JSON array to stdout.
registry::list() {
  _registry::ensure || return 1

  jq '.projects' "$NSELF_REGISTRY_FILE"
  return 0
}

# Get a single project entry by name as JSON to stdout.
# Returns 1 if the project is not found.
registry::get_project() {
  local name="${1:?registry::get_project requires name}"

  _registry::ensure || return 1

  local result
  result=$(jq -e --arg n "$name" '.projects[] | select(.name == $n)' "$NSELF_REGISTRY_FILE" 2>/dev/null)
  if [[ -z "$result" ]]; then
    log_error "Project '$name' not found in registry"
    return 1
  fi

  printf '%s\n' "$result"
  return 0
}

# Check if a project path is already registered.
# Returns 0 if registered, 1 if not.
registry::is_registered() {
  local path="${1:?registry::is_registered requires path}"

  _registry::ensure || return 1

  local match
  match=$(jq -r --arg p "$path" '.projects[] | select(.path == $p) | .name' "$NSELF_REGISTRY_FILE")
  if [[ -n "$match" ]]; then
    return 0
  fi
  return 1
}

# Output the number of registered projects to stdout.
registry::project_count() {
  _registry::ensure || return 1

  jq '.projects | length' "$NSELF_REGISTRY_FILE"
  return 0
}
