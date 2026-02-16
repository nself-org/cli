#!/usr/bin/env bash

# gitignore.sh - Gitignore management functions for nself init command
#
# This module handles all .gitignore file operations including creation,
# updates, and validation of required security entries.

# Source configuration if not already loaded
if [[ -z "${INIT_E_SUCCESS:-}" ]]; then

set -euo pipefail

  source "$(dirname "${BASH_SOURCE[0]}")/config.sh"
fi

# Ensure safe_echo is available
if ! type -t safe_echo >/dev/null 2>&1; then
  source "$(dirname "${BASH_SOURCE[0]}")/platform.sh"
fi

# Ensure .gitignore has all required entries for security
# Inputs: None (uses INIT_GITIGNORE_REQUIRED array from config)
# Outputs: Creates or updates .gitignore file
# Returns: 0 on success, error code on failure
# Side effects: Updates CREATED_FILES or MODIFIED_FILES arrays
ensure_gitignore() {
  local required_entries=("${INIT_GITIGNORE_REQUIRED[@]}")

  # Create .gitignore if it doesn't exist
  if [[ ! -f ".gitignore" ]]; then
    create_gitignore
    return $?
  else
    update_gitignore
    return $?
  fi
}

# Create a new .gitignore file with all required entries
# Inputs: None
# Outputs: Creates .gitignore file
# Returns: 0 on success, error code on failure
create_gitignore() {
  {
    echo "# Environment files (sensitive)"
    echo ".env"
    echo ".env.local"
    echo ".env.*.local"
    echo ".env.secrets"
    echo ""
    echo "# Backup folders from nself reset"
    echo "_backup*"
    echo ""
    echo "# Docker volumes"
    echo ".volumes/"
    echo ""
    echo "# Logs"
    echo "logs/"
    echo "*.log"
    echo ""
    echo "# Dependencies"
    echo "node_modules/"
    echo ""
    echo "# System files"
    echo ".DS_Store"
    echo ".idea/"
    echo ".vscode/"
    echo "*.swp"
    echo "*.swo"
    echo "*~"
    echo ""
    echo "# Build artifacts"
    echo "dist/"
    echo "build/"
    echo "*.pid"
  } >.gitignore

  if [[ $? -eq 0 ]]; then
    CREATED_FILES+=(".gitignore")
    safe_echo "${COLOR_GREEN:-}${CHECK_MARK:-✓}${COLOR_RESET:-} Created .gitignore with security rules"
    return $INIT_E_SUCCESS
  else
    log_error "Failed to create .gitignore"
    return $INIT_E_IOERR
  fi
}

# Update existing .gitignore with missing required entries
# Inputs: None (uses INIT_GITIGNORE_REQUIRED array from config)
# Outputs: Updates .gitignore file if needed
# Returns: 0 on success, error code on failure
update_gitignore() {
  local required_entries=("${INIT_GITIGNORE_REQUIRED[@]}")
  local added=false
  local needs_header=true

  for entry in "${required_entries[@]}"; do
    if ! gitignore_has_entry "$entry"; then
      add_gitignore_entry "$entry" "$needs_header"
      added=true

      # Only add header once for .env entries
      if [[ "$entry" == ".env"* ]]; then
        needs_header=false
      fi
    fi
  done

  if [[ "$added" == true ]]; then
    MODIFIED_FILES+=(".gitignore")
    safe_echo "${COLOR_GREEN:-}${CHECK_MARK:-✓}${COLOR_RESET:-} Updated .gitignore with required security rules"
  fi

  return $INIT_E_SUCCESS
}

# Check if gitignore already has an entry
# Inputs: $1 - entry to check for
# Outputs: None
# Returns: 0 if entry exists, 1 if not
gitignore_has_entry() {
  local entry="$1"

  # Escape the entry for grep
  local escaped_entry
  escaped_entry=$(printf '%s\n' "$entry" | sed 's/[[\.*^$()+?{|]/\\&/g')

  # Check if entry already exists (exact match or with comment)
  if grep -q "^${escaped_entry}$" .gitignore 2>/dev/null ||
    grep -q "^${escaped_entry}[[:space:]]*#" .gitignore 2>/dev/null; then
    return 0
  else
    return 1
  fi
}

# Add a single entry to .gitignore
# Inputs: $1 - entry to add, $2 - whether to add header (true/false)
# Outputs: Appends to .gitignore file
# Returns: 0 on success, error code on failure
add_gitignore_entry() {
  local entry="$1"
  local add_header="${2:-false}"

  # Add header for first .env entry if requested
  if [[ "$entry" == ".env"* ]] && [[ "$add_header" == true ]]; then
    echo "" >>.gitignore
    echo "# Environment files (sensitive)" >>.gitignore
  fi

  echo "$entry" >>.gitignore
  return $?
}

# Validate that .gitignore has all required entries
# Inputs: None
# Outputs: None
# Returns: 0 if valid, 1 if missing entries
validate_gitignore() {
  local required_entries=("${INIT_GITIGNORE_REQUIRED[@]}")
  local missing_entries=()

  for entry in "${required_entries[@]}"; do
    if ! gitignore_has_entry "$entry"; then
      missing_entries+=("$entry")
    fi
  done

  if [[ ${#missing_entries[@]} -gt 0 ]]; then
    log_warning "Gitignore missing required entries: ${missing_entries[*]}"
    return 1
  fi

  return 0
}

# Export functions for use in other scripts
export -f ensure_gitignore create_gitignore update_gitignore
export -f gitignore_has_entry add_gitignore_entry validate_gitignore
