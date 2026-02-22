#!/usr/bin/env bash

# env-quotes-fix.sh - Auto-fix unquoted environment values with spaces
# This prevents silent failures when sourcing env files with 'set -a'

# Define logging functions if not already defined
if ! command -v log_warning >/dev/null 2>&1; then

set -euo pipefail

  log_warning() { echo "⚠ $1" >&2; }
fi
if ! command -v log_info >/dev/null 2>&1; then
  log_info() { echo "ℹ $1" >&2; }
fi
if ! command -v log_success >/dev/null 2>&1; then
  log_success() { echo "✓ $1" >&2; }
fi
if ! command -v log_error >/dev/null 2>&1; then
  log_error() { echo "✗ $1" >&2; }
fi

# Function to check if a value needs quotes
needs_quotes() {
  local value="$1"

  # Skip if already quoted
  if [[ "$value" =~ ^\".*\"$ ]] || [[ "$value" =~ ^\'.*\'$ ]]; then
    return 1
  fi

  # Check if contains spaces or special characters that need quoting
  # Extended list to catch more problematic cases
  if [[ "$value" =~ [[:space:]] ]] ||
    [[ "$value" =~ [\*\?\[\]\{\}\(\)\!\|\&\;\<\>\`\$] ]] ||
    [[ "$value" =~ ^[0-9]+[[:space:]] ]]; then
    return 0
  fi

  return 1
}

# Function to fix unquoted values in an env file
fix_env_file_quotes() {
  local env_file="${1:-.env}"
  local fixed_count=0
  local temp_file=$(mktemp)

  if [[ ! -f "$env_file" ]]; then
    return 0
  fi

  # Process each line
  while IFS= read -r line || [[ -n "$line" ]]; do
    # Skip comments and empty lines
    if [[ "$line" =~ ^[[:space:]]*# ]] || [[ -z "${line// /}" ]]; then
      echo "$line" >>"$temp_file"
      continue
    fi

    # Check if it's a variable assignment
    if [[ "$line" =~ ^([A-Z_][A-Z0-9_]*)=(.*)$ ]]; then
      local var_name="${BASH_REMATCH[1]}"
      local var_value="${BASH_REMATCH[2]}"

      # Check if value needs quotes
      if needs_quotes "$var_value"; then
        # Add quotes around the value
        echo "${var_name}=\"${var_value}\"" >>"$temp_file"
        fixed_count=$((fixed_count + 1))
        # Log the fix if verbose
        if [[ "${VERBOSE:-false}" == "true" ]] || [[ "${DEBUG:-false}" == "true" ]]; then
          echo "  Fixed: ${var_name}=${var_value} → ${var_name}=\"${var_value}\"" >&2
        fi
      else
        echo "$line" >>"$temp_file"
      fi
    else
      echo "$line" >>"$temp_file"
    fi
  done <"$env_file"

  # Replace original file if fixes were made
  if [[ $fixed_count -gt 0 ]]; then
    cp "$temp_file" "$env_file"
    rm -f "$temp_file"
    return $fixed_count
  else
    rm -f "$temp_file"
    return 0
  fi
}

# Function to validate env file before sourcing
validate_env_file() {
  local env_file="${1:-.env}"
  local has_issues=false
  local line_num=0
  local issue_count=0

  if [[ ! -f "$env_file" ]]; then
    return 0
  fi

  while IFS= read -r line || [[ -n "$line" ]]; do
    line_num=$((line_num + 1))

    # Skip comments and empty lines
    if [[ "$line" =~ ^[[:space:]]*# ]] || [[ -z "${line// /}" ]]; then
      continue
    fi

    # Check for variable assignments with unquoted spaces
    if [[ "$line" =~ ^([A-Z_][A-Z0-9_]*)=(.*)$ ]]; then
      local var_name="${BASH_REMATCH[1]}"
      local var_value="${BASH_REMATCH[2]}"

      if needs_quotes "$var_value"; then
        issue_count=$((issue_count + 1))
        if [[ "$has_issues" == "false" ]]; then
          has_issues=true
        fi
        # Only show detailed output in verbose mode
        if [[ "${VERBOSE:-false}" == "true" ]] || [[ "${DEBUG:-false}" == "true" ]]; then
          if [[ "${AUTO_FIX:-true}" == "true" ]]; then
            log_success "  Fixed line $line_num: ${var_name}=\"${var_value}\""
          else
            log_info "  Line $line_num: $line"
          fi
        fi
      fi
    fi
  done <"$env_file"

  if [[ "$has_issues" == "true" ]]; then
    # Return the issue count
    return $issue_count
  fi

  return 0
}

# Main auto-fix function to be called from build.sh
auto_fix_env_quotes() {
  local auto_fix="${AUTO_FIX:-true}"
  local fixed_total=0
  local files_with_issues=()

  # Set AUTO_FIX for validate_env_file to use
  export AUTO_FIX="$auto_fix"

  # Check all env files
  for env_file in .env .env.dev .env.staging .env.prod .env.local; do
    if [[ -f "$env_file" ]]; then
      # Validate will return issue count if there are issues
      local issue_count
      validate_env_file "$env_file" >/dev/null 2>&1
      issue_count=$?

      if [[ $issue_count -gt 0 ]]; then
        files_with_issues+=("$env_file")
        if [[ "$auto_fix" == "true" ]]; then
          # Silently fix without duplicate messages
          local fixed_count
          fix_env_file_quotes "$env_file" >/dev/null 2>&1
          fixed_count=$?
          ((fixed_total += fixed_count))
        else
          log_info "Run with AUTO_FIX=true to fix automatically"
          return 1
        fi
      fi
    fi
  done

  # Show compact summary unless verbose mode
  if [[ $fixed_total -gt 0 ]]; then
    if [[ "${VERBOSE:-false}" == "true" ]] || [[ "${DEBUG:-false}" == "true" ]]; then
      log_success "Fixed $fixed_total unquoted value(s) across ${#files_with_issues[@]} file(s)"
    else
      log_info "Auto-fixing unquoted values in ${files_with_issues[*]}..."
    fi
  fi

  return 0
}

# Export functions
export -f needs_quotes
export -f fix_env_file_quotes
export -f validate_env_file
export -f auto_fix_env_quotes
