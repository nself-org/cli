#!/usr/bin/env bash
# time-mock.sh - Mock time control for deterministic testing
#
# Allows tests to control time, fast-forward through delays,
# and create deterministic timestamps without sleep().

set -euo pipefail

# ============================================================================
# Mock Time State
# ============================================================================

# Current mock time (Unix timestamp)
# shellcheck disable=SC2218  # date() is defined later in this file to mock it;
#   here we intentionally call the system 'date' binary before the mock is installed.
MOCK_CURRENT_TIME="${MOCK_CURRENT_TIME:-$(date +%s)}"

# Time multiplier for fast-forward (1.0 = real time, 10.0 = 10x speed)
MOCK_TIME_MULTIPLIER="${MOCK_TIME_MULTIPLIER:-1.0}"

# Whether time mocking is enabled
MOCK_TIME_ENABLED="${MOCK_TIME_ENABLED:-false}"

# ============================================================================
# Time Control Functions
# ============================================================================

# Enable time mocking
enable_time_mock() {
  MOCK_TIME_ENABLED=true
  MOCK_CURRENT_TIME=$(date +%s)
  export MOCK_TIME_ENABLED MOCK_CURRENT_TIME
}

# Disable time mocking (return to real time)
disable_time_mock() {
  MOCK_TIME_ENABLED=false
  export MOCK_TIME_ENABLED
}

# Set current mock time
# Usage: set_mock_time timestamp
set_mock_time() {
  local timestamp="$1"
  MOCK_CURRENT_TIME="$timestamp"
  export MOCK_CURRENT_TIME
}

# Set mock time to specific date
# Usage: set_mock_date "2024-01-15 10:30:00"
set_mock_date() {
  local date_str="$1"
  local timestamp
  timestamp=$(date -j -f "%Y-%m-%d %H:%M:%S" "$date_str" +%s 2>/dev/null || \
              date -d "$date_str" +%s 2>/dev/null)
  set_mock_time "$timestamp"
}

# Advance mock time by seconds
# Usage: advance_time_by 60  # Advance by 60 seconds
advance_time_by() {
  local seconds="$1"
  MOCK_CURRENT_TIME=$((MOCK_CURRENT_TIME + seconds))
  export MOCK_CURRENT_TIME
}

# Set time multiplier for fast-forward
# Usage: set_time_multiplier 10.0  # 10x speed
set_time_multiplier() {
  local multiplier="$1"
  MOCK_TIME_MULTIPLIER="$multiplier"
  export MOCK_TIME_MULTIPLIER
}

# ============================================================================
# Mock date Command
# ============================================================================

# Mock date command that respects mock time
mock_date() {
  if [[ "$MOCK_TIME_ENABLED" == true ]]; then
    # Parse date arguments
    local format="+%a %b %d %H:%M:%S %Z %Y"  # Default format

    while [[ $# -gt 0 ]]; do
      case "$1" in
        +*)
          format="$1"
          shift
          ;;
        -d|--date)
          # Ignore date argument for now, use current mock time
          shift 2
          ;;
        -u|--utc|--universal)
          # UTC flag (ignore for simplicity)
          shift
          ;;
        *)
          shift
          ;;
      esac
    done

    # Format current mock time
    if command -v gdate >/dev/null 2>&1; then
      # GNU date (macOS with coreutils)
      gdate -d "@$MOCK_CURRENT_TIME" "$format"
    else
      # Try system date
      date -r "$MOCK_CURRENT_TIME" "$format" 2>/dev/null || \
      date -d "@$MOCK_CURRENT_TIME" "$format" 2>/dev/null
    fi
  else
    # Use real date
    command date "$@"
  fi
}

# ============================================================================
# Mock sleep Command
# ============================================================================

# Mock sleep that uses time multiplier
mock_sleep() {
  local duration="$1"

  if [[ "$MOCK_TIME_ENABLED" == true ]]; then
    # Calculate actual sleep time with multiplier
    local actual_sleep
    actual_sleep=$(awk "BEGIN {print $duration / $MOCK_TIME_MULTIPLIER}")

    # Advance mock time
    advance_time_by "$duration"

    # Sleep for reduced time (or skip if multiplier is high)
    if (( $(awk "BEGIN {print ($actual_sleep > 0.01)}") )); then
      command sleep "$actual_sleep"
    fi
  else
    # Use real sleep
    command sleep "$duration"
  fi
}

# ============================================================================
# Instant Sleep (no actual waiting)
# ============================================================================

# Sleep that only advances mock time, no actual delay
# Usage: instant_sleep 60  # Instantly advance time by 60s
instant_sleep() {
  local duration="$1"

  if [[ "$MOCK_TIME_ENABLED" == true ]]; then
    advance_time_by "$duration"
  else
    # If mocking disabled, use fast sleep
    command sleep 0.01
  fi
}

# ============================================================================
# Timeout Simulation
# ============================================================================

# Simulate timeout testing without waiting
# Usage: simulate_timeout seconds command [args...]
simulate_timeout() {
  local timeout_seconds="$1"
  shift
  local command_to_run=("$@")

  if [[ "$MOCK_TIME_ENABLED" == true ]]; then
    # Advance time to timeout
    advance_time_by "$timeout_seconds"

    # Return timeout error
    printf "Simulated timeout after %ds\n" "$timeout_seconds" >&2
    return 124  # timeout command exit code
  else
    # Run with real timeout if available
    if command -v timeout >/dev/null 2>&1; then
      timeout "$timeout_seconds" "${command_to_run[@]}"
    elif command -v gtimeout >/dev/null 2>&1; then
      gtimeout "$timeout_seconds" "${command_to_run[@]}"
    else
      # No timeout available, just run command
      "${command_to_run[@]}"
    fi
  fi
}

# ============================================================================
# Time-based Test Helpers
# ============================================================================

# Get current mock time
get_mock_time() {
  if [[ "$MOCK_TIME_ENABLED" == true ]]; then
    printf "%s\n" "$MOCK_CURRENT_TIME"
  else
    date +%s
  fi
}

# Get formatted mock time
# Usage: get_mock_time_formatted "+%Y-%m-%d %H:%M:%S"
get_mock_time_formatted() {
  local format="${1:-+%Y-%m-%d %H:%M:%S}"
  mock_date "$format"
}

# Assert time has advanced
# Usage: assert_time_advanced initial_time expected_advance
assert_time_advanced() {
  local initial_time="$1"
  local expected_advance="$2"
  local actual_advance=$((MOCK_CURRENT_TIME - initial_time))

  if [[ $actual_advance -ge $expected_advance ]]; then
    return 0
  else
    printf "Time did not advance as expected\n" >&2
    printf "  Expected advance: %ds\n" "$expected_advance" >&2
    printf "  Actual advance: %ds\n" "$actual_advance" >&2
    return 1
  fi
}

# Freeze time at current value
freeze_time() {
  set_time_multiplier 0
}

# Unfreeze time (return to normal speed)
unfreeze_time() {
  set_time_multiplier 1.0
}

# ============================================================================
# Deterministic Timestamps
# ============================================================================

# Generate deterministic timestamp for testing
# Always returns same value for same seed
generate_deterministic_timestamp() {
  local seed="${1:-test}"
  local base_time=1704067200  # 2024-01-01 00:00:00 UTC

  # Simple hash of seed to offset
  local offset=0
  local char
  for ((i=0; i<${#seed}; i++)); do
    char="${seed:$i:1}"
    offset=$((offset + $(printf "%d" "'$char")))
  done

  printf "%d\n" $((base_time + offset))
}

# Create deterministic date string
# Usage: generate_deterministic_date seed [format]
generate_deterministic_date() {
  local seed="${1:-test}"
  local format="${2:-+%Y-%m-%d %H:%M:%S}"
  local timestamp
  timestamp=$(generate_deterministic_timestamp "$seed")

  if command -v gdate >/dev/null 2>&1; then
    gdate -d "@$timestamp" "$format"
  else
    date -r "$timestamp" "$format" 2>/dev/null || \
    date -d "@$timestamp" "$format" 2>/dev/null
  fi
}

# ============================================================================
# Install Mocks
# ============================================================================

# Override date (only when explicitly enabled)
date() {
  mock_date "$@"
}

# Override sleep (only when explicitly enabled)
sleep() {
  mock_sleep "$@"
}

# Export functions
export -f enable_time_mock
export -f disable_time_mock
export -f set_mock_time
export -f set_mock_date
export -f advance_time_by
export -f set_time_multiplier
export -f mock_date
export -f mock_sleep
export -f instant_sleep
export -f simulate_timeout
export -f get_mock_time
export -f get_mock_time_formatted
export -f assert_time_advanced
export -f freeze_time
export -f unfreeze_time
export -f generate_deterministic_timestamp
export -f generate_deterministic_date
export -f date
export -f sleep

# Note: Time mocking is NOT enabled by default
# Tests must explicitly call enable_time_mock
