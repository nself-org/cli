#!/usr/bin/env bash

# atomic-ops.sh - Atomic file operations for nself init command
#
# This module provides atomic file operations with rollback capability
# to ensure safe file creation and modification.

# Source dependencies if not already loaded
if [[ -z "${INIT_E_SUCCESS:-}" ]]; then

set -euo pipefail

  source "$(dirname "${BASH_SOURCE[0]}")/config.sh"
fi
if [[ -z "${SUPPORTS_COLOR:-}" ]]; then
  source "$(dirname "${BASH_SOURCE[0]}")/platform.sh"
fi

# Global variables for state tracking
TEMP_DIR=""
CREATED_FILES=()
MODIFIED_FILES=()
BACKUP_SUFFIX=".nself-backup"

# Initialize temp directory for atomic operations
# Inputs: None
# Outputs: Creates temp directory, sets TEMP_DIR variable
# Returns: 0 on success, error code on failure
init_temp_dir() {
  if [[ -z "$TEMP_DIR" ]] || [[ ! -d "$TEMP_DIR" ]]; then
    TEMP_DIR=$(mktemp -d 2>/dev/null) || {
      # Fallback for systems without mktemp
      TEMP_DIR="/tmp/nself-init-$$-$(date +%s)"
      mkdir -p "$TEMP_DIR" || {
        log_error "Failed to create temp directory"
        return $INIT_E_CANTCREAT
      }
    }
  fi

  export TEMP_DIR
  return $INIT_E_SUCCESS
}

# Clean up temp directory
# Inputs: None
# Outputs: Removes temp directory
# Returns: 0
cleanup_temp_dir() {
  if [[ -n "$TEMP_DIR" ]] && [[ -d "$TEMP_DIR" ]]; then
    rm -rf "$TEMP_DIR" 2>/dev/null || true
  fi
  TEMP_DIR=""
  return 0
}

# Atomically copy a file with proper permissions
# Inputs: $1 - source file, $2 - destination file, $3 - permissions (optional)
# Outputs: Copies file atomically, updates tracking arrays
# Returns: 0 on success, error code on failure
atomic_copy() {
  local src="$1"
  local dest="$2"
  local perms="${3:-$INIT_PERM_PUBLIC}"

  # Validate source
  if [[ ! -f "$src" ]]; then
    log_error "Source template file not found: $src"
    return $INIT_E_IOERR
  fi

  # Initialize temp directory if needed
  init_temp_dir || return $?

  # Create temp file
  local temp_file
  temp_file="$TEMP_DIR/$(basename "$dest").tmp"

  # Copy to temp file
  if cp "$src" "$temp_file" 2>/dev/null; then
    # Set permissions
    chmod "$perms" "$temp_file" 2>/dev/null || true

    # Backup existing if present
    if [[ -f "$dest" ]]; then
      backup_file "$dest" || return $?
      MODIFIED_FILES+=("$dest")
    else
      CREATED_FILES+=("$dest")
    fi

    # Move atomically
    if mv -f "$temp_file" "$dest" 2>/dev/null; then
      return $INIT_E_SUCCESS
    else
      rm -f "$temp_file" 2>/dev/null
      log_error "Failed to create $dest"
      return $INIT_E_IOERR
    fi
  else
    log_error "Failed to copy template for $dest"
    return $INIT_E_IOERR
  fi
}

# Create a backup of an existing file
# Inputs: $1 - file to backup
# Outputs: Creates backup file
# Returns: 0 on success, error code on failure
backup_file() {
  local file="$1"
  local backup="${file}${BACKUP_SUFFIX}"

  if [[ -f "$file" ]]; then
    cp "$file" "$backup" 2>/dev/null || {
      log_warning "Could not create backup of $file"
      return $INIT_E_IOERR
    }
  fi

  return $INIT_E_SUCCESS
}

# Restore a file from backup
# Inputs: $1 - file to restore
# Outputs: Restores file from backup
# Returns: 0 on success, 1 if no backup exists
restore_file() {
  local file="$1"
  local backup="${file}${BACKUP_SUFFIX}"

  if [[ -f "$backup" ]]; then
    mv -f "$backup" "$file" 2>/dev/null || {
      log_error "Could not restore $file from backup"
      return 1
    }
    return 0
  else
    return 1 # No backup exists
  fi
}

# Rollback all changes made during init
# Inputs: None (uses CREATED_FILES and MODIFIED_FILES arrays)
# Outputs: Removes created files, restores modified files
# Returns: 0
rollback_changes() {
  local rolled_back=false

  # Remove created files
  if [[ ${#CREATED_FILES[@]} -gt 0 ]]; then
    for file in "${CREATED_FILES[@]}"; do
      if [[ -f "$file" ]]; then
        rm -f "$file" 2>/dev/null || log_warning "Could not remove $file"
        rolled_back=true
      fi
    done
  fi

  # Restore modified files
  if [[ ${#MODIFIED_FILES[@]} -gt 0 ]]; then
    for file in "${MODIFIED_FILES[@]}"; do
      restore_file "$file"
      rolled_back=true
    done
  fi

  if [[ "$rolled_back" == true ]]; then
    # Log rollback success if log_info is available
    if type -t log_info >/dev/null 2>&1; then
      log_info "Changes rolled back successfully"
    fi
  fi

  # Clean up temp directory
  cleanup_temp_dir

  # Reset tracking arrays
  CREATED_FILES=()
  MODIFIED_FILES=()

  return 0
}

# Clean up backups after successful operation
# Inputs: None (uses MODIFIED_FILES array)
# Outputs: Removes backup files
# Returns: 0
cleanup_backups() {
  if [[ ${#MODIFIED_FILES[@]} -gt 0 ]]; then
    for file in "${MODIFIED_FILES[@]}"; do
      local backup="${file}${BACKUP_SUFFIX}"
      if [[ -f "$backup" ]]; then
        rm -f "$backup" 2>/dev/null || true
      fi
    done
  fi

  return 0
}

# Atomically write content to a file
# Inputs: $1 - destination file, $2 - content, $3 - permissions (optional)
# Outputs: Creates or updates file atomically
# Returns: 0 on success, error code on failure
atomic_write() {
  local dest="$1"
  local content="$2"
  local perms="${3:-$INIT_PERM_PUBLIC}"

  # Initialize temp directory if needed
  init_temp_dir || return $?

  # Create temp file
  local temp_file
  temp_file="$TEMP_DIR/$(basename "$dest").tmp"

  # Write content to temp file
  if echo "$content" >"$temp_file" 2>/dev/null; then
    # Set permissions
    chmod "$perms" "$temp_file" 2>/dev/null || true

    # Backup existing if present
    if [[ -f "$dest" ]]; then
      backup_file "$dest" || return $?
      MODIFIED_FILES+=("$dest")
    else
      CREATED_FILES+=("$dest")
    fi

    # Move atomically
    if mv -f "$temp_file" "$dest" 2>/dev/null; then
      return $INIT_E_SUCCESS
    else
      rm -f "$temp_file" 2>/dev/null
      log_error "Failed to write $dest"
      return $INIT_E_IOERR
    fi
  else
    log_error "Failed to write content for $dest"
    return $INIT_E_IOERR
  fi
}

# Check if operation can proceed safely
# Inputs: $1 - file path
# Outputs: None
# Returns: 0 if safe to proceed, error code otherwise
check_safe_to_proceed() {
  local file="$1"

  # Check if parent directory exists
  local parent_dir
  parent_dir=$(dirname "$file")
  if [[ ! -d "$parent_dir" ]]; then
    log_error "Parent directory does not exist: $parent_dir"
    return $INIT_E_CANTCREAT
  fi

  # Check if parent directory is writable
  if [[ ! -w "$parent_dir" ]]; then
    log_error "Parent directory is not writable: $parent_dir"
    return $INIT_E_NOPERM
  fi

  # Check if file exists and is writable (for updates)
  if [[ -f "$file" ]] && [[ ! -w "$file" ]]; then
    log_error "File exists but is not writable: $file"
    return $INIT_E_NOPERM
  fi

  return $INIT_E_SUCCESS
}

# Export functions for use in other scripts
export -f init_temp_dir cleanup_temp_dir atomic_copy backup_file
export -f restore_file rollback_changes cleanup_backups atomic_write
export -f check_safe_to_proceed

# Export variables
export TEMP_DIR CREATED_FILES MODIFIED_FILES BACKUP_SUFFIX
