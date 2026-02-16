#!/usr/bin/env bash
# test-helpers.sh - Mock and stub helpers for testing
#
# This module provides mock commands and stub functions for testing
# without requiring actual system dependencies

set -euo pipefail

# ============================================================================
# Mock Command Framework
# ============================================================================

# Store original commands (initialize as empty arrays/strings)
MOCKED_COMMANDS=()
ORIGINAL_COMMANDS_LIST=""

# Try to use associative array if available (Bash 4+)
HAS_ASSOC_ARRAYS=false
if declare -A _test_assoc 2>/dev/null; then
  declare -A ORIGINAL_COMMANDS
  unset _test_assoc
  HAS_ASSOC_ARRAYS=true
fi

# Mock a command with a custom function
# Usage: mock_command "git" 'echo "mock git $@"'
mock_command() {
  local cmd="$1"
  local mock_impl="$2"

  # Save original if it exists
  if command -v "$cmd" >/dev/null 2>&1; then
    if [[ "$HAS_ASSOC_ARRAYS" == "true" ]]; then
      ORIGINAL_COMMANDS["$cmd"]=$(command -v "$cmd")
    else
      ORIGINAL_COMMANDS_LIST="$ORIGINAL_COMMANDS_LIST|$cmd:$(command -v "$cmd")"
    fi
  fi

  # Create mock function
  eval "function $cmd() { $mock_impl; }"
  export -f "$cmd"

  # Track mocked command
  MOCKED_COMMANDS+=("$cmd")
}

# Restore all mocked commands
restore_mocks() {
  if [[ ${#MOCKED_COMMANDS[@]} -gt 0 ]]; then
    for cmd in "${MOCKED_COMMANDS[@]}"; do
      unset -f "$cmd" 2>/dev/null || true
    done
  fi
  MOCKED_COMMANDS=()
}

# ============================================================================
# Stub Functions
# ============================================================================

# Stub for Docker
stub_docker() {
  case "$1" in
    ps)
      if [[ "${2:-}" == "--filter" ]]; then
        # Return empty list
        return 0
      fi
      ;;
    compose)
      echo "mock: Docker Compose version 2.0.0"
      return 0
      ;;
    *)
      echo "mock: docker $*"
      return 0
      ;;
  esac
}

# Stub for git
stub_git() {
  case "$1" in
    rev-parse)
      if [[ "${2:-}" == "--git-dir" ]]; then
        echo ".git"
        return 0
      elif [[ "${2:-}" == "--show-toplevel" ]]; then
        pwd
        return 0
      fi
      ;;
    status)
      echo "On branch main"
      echo "nothing to commit, working tree clean"
      return 0
      ;;
    *)
      echo "mock: git $*"
      return 0
      ;;
  esac
}

# Stub for stat command (cross-platform)
stub_stat() {
  local file="${2:-$1}"

  # Return mock permissions
  case "$file" in
    *.env)
      echo "600"
      ;;
    *.example)
      echo "644"
      ;;
    *)
      echo "644"
      ;;
  esac
  return 0
}

# ============================================================================
# File System Mocks
# ============================================================================

# Create a mock project structure
create_mock_project() {
  local base_dir="${1:-mock-project}"

  mkdir -p "$base_dir"/{src,tests,docs}

  # Create mock files
  touch "$base_dir/README.md"
  echo "# Mock project" >"$base_dir/README.md"

  # Create mock git repo
  mkdir -p "$base_dir/.git"
  echo "ref: refs/heads/main" >"$base_dir/.git/HEAD"

  # Create mock package.json
  cat >"$base_dir/package.json" <<'EOF'
{
  "name": "mock-project",
  "version": "1.0.0",
  "scripts": {
    "start": "node index.js",
    "test": "jest"
  }
}
EOF

  echo "$base_dir"
}

# ============================================================================
# Environment Mocks
# ============================================================================

# Mock terminal environment
mock_terminal_env() {
  export TERM="xterm-256color"
  export LANG="en_US.UTF-8"
  export COLUMNS=80
  export LINES=24
  export NO_COLOR=""
  export FORCE_COLOR="1"
}

# Mock CI environment
mock_ci_env() {
  export CI="true"
  export GITHUB_ACTIONS="true"
  export RUNNER_OS="Linux"
  export NO_COLOR="1"
}

# ============================================================================
# Spy Functions
# ============================================================================

# Track function calls
FUNCTION_CALLS=()

# Create a spy for a function
# Usage: spy_function "my_func"
spy_function() {
  local func_name="$1"
  local original_func="original_$func_name"

  # Save original function
  if declare -f "$func_name" >/dev/null; then
    eval "$(declare -f "$func_name" | sed "1s/$func_name/$original_func/")"
  fi

  # Create spy wrapper
  eval "function $func_name() {
    FUNCTION_CALLS+=(\"$func_name:\$@\")
    if declare -f $original_func >/dev/null; then
      $original_func \"\$@\"
    fi
  }"
}

# Check if function was called
# Usage: was_called "my_func"
was_called() {
  local func_name="$1"
  local args="${2:-}"

  if [[ ${#FUNCTION_CALLS[@]} -eq 0 ]]; then
    return 1
  fi

  for call in "${FUNCTION_CALLS[@]}"; do
    if [[ "$args" ]]; then
      if [[ "$call" == "$func_name:$args" ]]; then
        return 0
      fi
    else
      if [[ "$call" == "$func_name:"* ]]; then
        return 0
      fi
    fi
  done

  return 1
}

# Get call count for a function
get_call_count() {
  local func_name="$1"
  local count=0

  if [[ ${#FUNCTION_CALLS[@]} -gt 0 ]]; then
    for call in "${FUNCTION_CALLS[@]}"; do
      if [[ "$call" == "$func_name:"* ]]; then
        ((count++))
      fi
    done
  fi

  echo "$count"
}

# Clear spy data
clear_spies() {
  FUNCTION_CALLS=()
}

# ============================================================================
# Input/Output Mocks
# ============================================================================

# Mock user input
# Usage: mock_input "y" "n" "test"
mock_input() {
  local inputs=("$@")
  local input_file="/tmp/mock_input_$$"

  # Write inputs to temp file
  for input in "${inputs[@]}"; do
    echo "$input"
  done >"$input_file"

  # Redirect stdin from file
  exec <"$input_file"

  # Clean up on exit
  trap "rm -f $input_file" EXIT
}

# Capture output
# Usage: output=$(capture_output my_function arg1 arg2)
capture_output() {
  local temp_file="/tmp/capture_output_$$"

  # Run command and capture output
  "$@" >"$temp_file" 2>&1
  local exit_code=$?

  # Read and output captured text
  cat "$temp_file"

  # Cleanup
  rm -f "$temp_file"

  return $exit_code
}

# ============================================================================
# Time Mocks
# ============================================================================

# Mock date command
stub_date() {
  case "${1:-}" in
    +%Y-%m-%d)
      echo "2024-01-15"
      ;;
    +%s)
      echo "1705334400" # 2024-01-15 00:00:00 UTC
      ;;
    *)
      echo "Mon Jan 15 00:00:00 UTC 2024"
      ;;
  esac
}

# ============================================================================
# Network Mocks
# ============================================================================

# Mock curl command
stub_curl() {
  local url="${@: -1}" # Last argument is usually URL

  case "$url" in
    *api.github.com*)
      echo '{"tag_name": "v1.0.0"}'
      ;;
    *localhost*)
      echo "OK"
      ;;
    *)
      echo "Mock response for $url"
      ;;
  esac
  return 0
}

# Mock wget command
stub_wget() {
  echo "mock: Downloaded $*"
  touch "${@: -1}" # Create empty file with last arg as name
  return 0
}

# ============================================================================
# Cleanup
# ============================================================================

# Clean up all test artifacts
cleanup_test_env() {
  restore_mocks
  clear_spies

  # Remove any temp files
  rm -f /tmp/mock_input_* /tmp/capture_output_* 2>/dev/null || true

  # Reset environment
  unset CI GITHUB_ACTIONS RUNNER_OS NO_COLOR FORCE_COLOR 2>/dev/null || true
}

# Set up trap for cleanup
trap cleanup_test_env EXIT INT TERM

# ============================================================================
# Export functions
# ============================================================================

export -f mock_command restore_mocks
export -f stub_docker stub_git stub_stat stub_date stub_curl stub_wget
export -f create_mock_project mock_terminal_env mock_ci_env
export -f spy_function was_called get_call_count clear_spies
export -f mock_input capture_output
export -f cleanup_test_env
