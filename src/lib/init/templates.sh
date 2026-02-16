#!/usr/bin/env bash

# templates.sh - Template management functions for nself init command
#
# This module handles locating and copying template files for project
# initialization.

# Source dependencies if not already loaded
if [[ -z "${INIT_E_SUCCESS:-}" ]]; then

set -euo pipefail

  source "$(dirname "${BASH_SOURCE[0]}")/config.sh"
fi

if ! type -t atomic_copy >/dev/null 2>&1; then
  source "$(dirname "${BASH_SOURCE[0]}")/atomic-ops.sh"
fi

# Ensure safe_echo is available
if ! type -t safe_echo >/dev/null 2>&1; then
  source "$(dirname "${BASH_SOURCE[0]}")/platform.sh"
fi

# Find the templates directory
# Inputs: $1 - script directory (optional, defaults to $SCRIPT_DIR)
# Outputs: Path to templates directory
# Returns: 0 on success, error code on failure
find_templates_dir() {
  local script_dir="${1:-$SCRIPT_DIR}"
  local templates_dir=""

  # SECURITY: Safe path expansion helper - expand known vars without eval
  _safe_expand_path() {
    local _p="$1"
    _p="${_p/#\~/$HOME}"
    _p="${_p/\$HOME/$HOME}"
    _p="${_p/\${HOME\}/$HOME}"
    _p="${_p/\$NSELF_ROOT/${NSELF_ROOT:-}}"
    _p="${_p/\${NSELF_ROOT\}/${NSELF_ROOT:-}}"
    _p="${_p/\$NSELF_DIR/${NSELF_DIR:-}}"
    _p="${_p/\${NSELF_DIR\}/${NSELF_DIR:-}}"
    printf "%s" "$_p"
  }

  # Use configured search paths
  for relative_path in "${INIT_TEMPLATE_SEARCH_PATHS[@]}"; do
    local check_path

    # Handle relative paths (relative to script_dir)
    if [[ "$relative_path" == ../* ]] || [[ "$relative_path" == ./* ]]; then
      check_path="$script_dir/$relative_path"
    else
      # Expand known environment variables in path safely (no eval)
      check_path=$(_safe_expand_path "$relative_path")
    fi

    if [[ -d "$check_path" ]]; then
      templates_dir="$check_path"
      break
    fi
  done

  if [[ -z "$templates_dir" ]]; then
    log_error "Cannot find templates directory"
    echo "Searched in:" >&2
    for path in "${INIT_TEMPLATE_SEARCH_PATHS[@]}"; do
      local expanded_path
      if [[ "$path" == ../* ]] || [[ "$path" == ./* ]]; then
        expanded_path="$script_dir/$path"
      else
        expanded_path=$(_safe_expand_path "$path")
      fi
      echo "  - $expanded_path" >&2
    done
    echo "Please ensure nself is properly installed" >&2
    return $INIT_E_CONFIG
  fi

  echo "$templates_dir"
  return $INIT_E_SUCCESS
}

# Verify template files exist
# Inputs: $1 - templates directory, $2+ - template file names
# Outputs: None
# Returns: 0 if all exist, error code on failure
verify_template_files() {
  local templates_dir="$1"
  shift

  local missing_files=()
  for file in "$@"; do
    if [[ ! -f "$templates_dir/$file" ]]; then
      missing_files+=("$file")
    fi
  done

  if [[ ${#missing_files[@]} -gt 0 ]]; then
    # Use echo to stderr if log_error is not available
    if declare -f log_error >/dev/null 2>&1; then
      log_error "Template files not found in $templates_dir:"
    else
      echo "Error: Template files not found in $templates_dir:" >&2
    fi
    for file in "${missing_files[@]}"; do
      echo "  - $file" >&2
    done
    echo "Please ensure nself is properly installed" >&2
    return $INIT_E_CONFIG
  fi

  return $INIT_E_SUCCESS
}

# Copy basic template files
# Inputs: $1 - templates directory, $2 - quiet mode flag (optional)
# Outputs: Copies basic template files
# Returns: 0 on success, error code on failure
copy_basic_templates() {
  local templates_dir="$1"
  local quiet_mode="${2:-false}"

  # Verify templates exist
  verify_template_files "$templates_dir" "${INIT_TEMPLATES_BASIC[@]}" || return $?

  # Copy each basic template
  for template in "${INIT_TEMPLATES_BASIC[@]}"; do
    # Get target filename (remove envs/ prefix if present)
    local target_file="${template#envs/}"

    # Determine permissions based on file
    local perms="$INIT_PERM_PUBLIC"
    if [[ "$target_file" == ".env" ]] || [[ "$target_file" == ".env.secrets" ]]; then
      perms="$INIT_PERM_PRIVATE"
    fi

    # Copy atomically
    atomic_copy "$templates_dir/$template" "$target_file" "$perms" || return $?

    # Show success message unless quiet
    if [[ "$quiet_mode" != true ]]; then
      case "$target_file" in
        .env)
          safe_echo "${COLOR_GREEN:-}${CHECK_MARK:-✓}${COLOR_RESET:-} Created .env (your dev config)"
          ;;
        .env.example)
          safe_echo "${COLOR_GREEN:-}${CHECK_MARK:-✓}${COLOR_RESET:-} Created .env.example (reference docs)"
          ;;
        *)
          safe_echo "${COLOR_GREEN:-}${CHECK_MARK:-✓}${COLOR_RESET:-} Created $target_file"
          ;;
      esac
    fi
  done

  return $INIT_E_SUCCESS
}

# Copy full template files (includes basic + additional)
# Inputs: $1 - templates directory, $2 - quiet mode flag (optional)
# Outputs: Copies all template files
# Returns: 0 on success, error code on failure
copy_full_templates() {
  local templates_dir="$1"
  local quiet_mode="${2:-false}"

  # First copy basic templates
  copy_basic_templates "$templates_dir" "$quiet_mode" || return $?

  # Verify full templates exist
  verify_template_files "$templates_dir" "${INIT_TEMPLATES_FULL[@]}" || return $?

  # Copy each additional template
  for template in "${INIT_TEMPLATES_FULL[@]}"; do
    # Skip if file doesn't exist (optional templates like schema.dbml)
    if [[ ! -f "$templates_dir/$template" ]]; then
      continue
    fi

    # Get target filename (remove envs/ prefix if present)
    local target_file="${template#envs/}"

    # Determine permissions based on file
    local perms="$INIT_PERM_PUBLIC"
    if [[ "$target_file" == ".env.secrets" ]]; then
      perms="$INIT_PERM_PRIVATE"
    fi

    # Copy atomically
    atomic_copy "$templates_dir/$template" "$target_file" "$perms" || return $?

    # Show success message unless quiet
    if [[ "$quiet_mode" != true ]]; then
      case "$target_file" in
        .env.dev)
          safe_echo "${COLOR_GREEN:-}${CHECK_MARK:-✓}${COLOR_RESET:-} Created .env.dev (team dev defaults)"
          ;;
        .env.staging)
          safe_echo "${COLOR_GREEN:-}${CHECK_MARK:-✓}${COLOR_RESET:-} Created .env.staging (staging config)"
          ;;
        .env.prod)
          safe_echo "${COLOR_GREEN:-}${CHECK_MARK:-✓}${COLOR_RESET:-} Created .env.prod (production config)"
          ;;
        .env.secrets)
          safe_echo "${COLOR_GREEN:-}${CHECK_MARK:-✓}${COLOR_RESET:-} Created .env.secrets (permissions: 600)"
          safe_echo "  ${COLOR_YELLOW:-}⚠${COLOR_RESET:-} Keep this file secure - never commit to git!"
          ;;
        schema.dbml)
          safe_echo "${COLOR_GREEN:-}${CHECK_MARK:-✓}${COLOR_RESET:-} Created schema.dbml (database schema)"
          ;;
        *)
          safe_echo "${COLOR_GREEN:-}${CHECK_MARK:-✓}${COLOR_RESET:-} Created $target_file"
          ;;
      esac
    fi
  done

  return $INIT_E_SUCCESS
}

# Copy a single template file with custom destination
# Inputs: $1 - templates dir, $2 - template name, $3 - destination, $4 - perms
# Outputs: Copies template file
# Returns: 0 on success, error code on failure
copy_single_template() {
  local templates_dir="$1"
  local template_name="$2"
  local destination="${3:-$template_name}"
  local permissions="${4:-$INIT_PERM_PUBLIC}"

  # Verify template exists
  if [[ ! -f "$templates_dir/$template_name" ]]; then
    log_error "Template not found: $template_name"
    return $INIT_E_CONFIG
  fi

  # Copy atomically
  atomic_copy "$templates_dir/$template_name" "$destination" "$permissions"
  return $?
}

# List available templates
# Inputs: $1 - templates directory
# Outputs: List of available template files
# Returns: 0 on success, error code on failure
list_available_templates() {
  local templates_dir="$1"

  if [[ ! -d "$templates_dir" ]]; then
    log_error "Templates directory not found: $templates_dir"
    return $INIT_E_CONFIG
  fi

  echo "Available templates:"
  for file in "$templates_dir"/*; do
    if [[ -f "$file" ]]; then
      echo "  - $(basename "$file")"
    fi
  done

  return $INIT_E_SUCCESS
}

# Export functions for use in other scripts
export -f find_templates_dir verify_template_files copy_basic_templates
export -f copy_full_templates copy_single_template list_available_templates
