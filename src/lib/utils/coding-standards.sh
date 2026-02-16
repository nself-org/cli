#!/usr/bin/env bash

# coding-standards.sh - Coding standards and best practices utilities
# POSIX-compliant, enforces consistent coding style

# Function naming standard: snake_case
# Variables: lowercase with underscores
# Constants: UPPERCASE with underscores
# File names: kebab-case.sh

# Validate function name follows snake_case
validate_function_name() {

set -euo pipefail

  local name="$1"

  if [[ "$name" =~ ^[a-z][a-z0-9_]*$ ]]; then
    return 0
  else
    echo "Invalid function name: $name (should be snake_case)" >&2
    return 1
  fi
}

# Ensure all variables are local in functions
ensure_local_variables() {
  local function_name="$1"
  shift

  # All remaining arguments should be variable names
  for var in "$@"; do
    echo "local $var"
  done
}

# Standard error codes
readonly EXIT_SUCCESS=0
readonly EXIT_GENERAL_ERROR=1
readonly EXIT_MISUSE=2
readonly EXIT_CANTEXEC=126
readonly EXIT_NOTFOUND=127
readonly EXIT_INVALID_ARG=128

# Standard logging with proper scoping
log_debug() {
  local message="$1"
  [[ "${DEBUG:-false}" == "true" ]] && echo "[DEBUG] $message" >&2
}

log_info() {
  local message="$1"
  echo "[INFO] $message" >&2
}

log_warning() {
  local message="$1"
  echo "[WARN] $message" >&2
}

log_error() {
  local message="$1"
  echo "[ERROR] $message" >&2
}

log_fatal() {
  local message="$1"
  local exit_code="${2:-$EXIT_GENERAL_ERROR}"
  echo "[FATAL] $message" >&2
  exit "$exit_code"
}

# Ensure function has proper variable scoping
create_function_template() {
  local function_name="$1"
  local description="$2"

  cat <<EOF
# ${description}
${function_name}() {
  # Declare all variables as local
  local arg1="\${1:-}"
  local arg2="\${2:-}"
  local result=""

  # Function logic here

  # Return status
  return 0
}
EOF
}

# Check if variable is properly scoped
check_variable_scope() {
  local file="$1"
  local warnings=0

  # Look for variables that should be local
  while IFS= read -r line; do
    # Check for variable assignments without local
    if [[ "$line" =~ ^[[:space:]]*([a-z_][a-z0-9_]*)= ]] &&
      [[ ! "$line" =~ ^[[:space:]]*local ]] &&
      [[ ! "$line" =~ ^[[:space:]]*readonly ]] &&
      [[ ! "$line" =~ ^[[:space:]]*export ]]; then
      echo "Warning: Variable '${BASH_REMATCH[1]}' should be declared as local" >&2
      warnings=$((warnings + 1))
    fi
  done <"$file"

  return $warnings
}

# Standard function documentation template
document_function() {
  local function_name="$1"
  local description="$2"
  local params="$3"
  local returns="$4"

  cat <<EOF
# ${function_name} - ${description}
#
# Arguments:
#   ${params}
#
# Returns:
#   ${returns}
#
# Example:
#   ${function_name} "arg1" "arg2"
EOF
}

# Ensure consistent indentation (2 spaces)
fix_indentation() {
  local file="$1"
  local temp_file=$(mktemp)

  # Convert tabs to 2 spaces
  expand -t 2 "$file" >"$temp_file"
  mv "$temp_file" "$file"
}

# Check for common anti-patterns
check_anti_patterns() {
  local file="$1"
  local issues=0

  # Check for backticks instead of $()
  if grep -q '`' "$file"; then
    echo "Warning: Use \$() instead of backticks for command substitution" >&2
    issues=$((issues + 1))
  fi

  # Check for unquoted variables
  if grep -E '\$[a-zA-Z_][a-zA-Z0-9_]*[^"]' "$file" | grep -v '^\s*#'; then
    echo "Warning: Unquoted variables found (use \"\$var\" instead of \$var)" >&2
    issues=$((issues + 1))
  fi

  # Check for == in [ ] tests (should be =)
  if grep -E '\[\s+.*==.*\]' "$file"; then
    echo "Warning: Use = instead of == in [ ] tests" >&2
    issues=$((issues + 1))
  fi

  return $issues
}

# Standard cleanup function template
create_cleanup_function() {
  cat <<'EOF'
# Cleanup function to be called on exit
cleanup() {
  local exit_code="$?"

  # Remove temporary files
  [[ -n "${TEMP_FILE:-}" ]] && rm -f "$TEMP_FILE"
  [[ -n "${TEMP_DIR:-}" ]] && rm -rf "$TEMP_DIR"

  # Restore original state if needed

  exit "$exit_code"
}

# Set up trap for cleanup
trap cleanup EXIT INT TERM
EOF
}

# Export functions
export -f validate_function_name
export -f ensure_local_variables
export -f log_debug
export -f log_info
export -f log_warning
export -f log_error
export -f log_fatal
export -f create_function_template
export -f check_variable_scope
export -f document_function
export -f fix_indentation
export -f check_anti_patterns
export -f create_cleanup_function
