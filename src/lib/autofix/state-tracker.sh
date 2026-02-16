#!/usr/bin/env bash


# State tracking for autofix attempts
# Tracks what fixes have been tried for each service to avoid repeating the same fix

AUTOFIX_STATE_DIR="/tmp/nself-autofix-state"

set -euo pipefail


init_autofix_state() {
  mkdir -p "$AUTOFIX_STATE_DIR"
}

get_service_attempts() {
  local service_name="$1"
  local state_file="$AUTOFIX_STATE_DIR/${service_name}.attempts"

  if [[ -f "$state_file" ]]; then
    cat "$state_file"
  else
    echo "0"
  fi
}

get_last_fix_strategy() {
  local service_name="$1"
  local strategy_file="$AUTOFIX_STATE_DIR/${service_name}.strategy"

  if [[ -f "$strategy_file" ]]; then
    cat "$strategy_file"
  else
    echo "none"
  fi
}

record_fix_attempt() {
  local service_name="$1"
  local strategy="$2"

  local state_file="$AUTOFIX_STATE_DIR/${service_name}.attempts"
  local strategy_file="$AUTOFIX_STATE_DIR/${service_name}.strategy"
  local history_file="$AUTOFIX_STATE_DIR/${service_name}.history"

  # Increment attempt counter
  local attempts=$(get_service_attempts "$service_name")
  echo $((attempts + 1)) >"$state_file"

  # Record the strategy used
  echo "$strategy" >"$strategy_file"

  # Add to history
  echo "$(date '+%Y-%m-%d %H:%M:%S') - Attempt $((attempts + 1)): $strategy" >>"$history_file"
}

reset_service_state() {
  local service_name="$1"
  rm -f "$AUTOFIX_STATE_DIR/${service_name}."* 2>/dev/null
}

reset_all_states() {
  rm -rf "$AUTOFIX_STATE_DIR"
  mkdir -p "$AUTOFIX_STATE_DIR"
}

get_service_error_hash() {
  local service_logs="$1"
  # Create a hash of the error to detect if it's the same error repeating
  echo "$service_logs" | grep -E "error|Error|failed|Failed|refused" | md5sum | cut -d' ' -f1
}

is_same_error() {
  local service_name="$1"
  local service_logs="$2"

  local error_hash_file="$AUTOFIX_STATE_DIR/${service_name}.error_hash"
  local current_hash=$(get_service_error_hash "$service_logs")

  if [[ -f "$error_hash_file" ]]; then
    local previous_hash=$(cat "$error_hash_file")
    if [[ "$current_hash" == "$previous_hash" ]]; then
      return 0 # Same error
    fi
  fi

  # Store the new hash
  echo "$current_hash" >"$error_hash_file"
  return 1 # Different error
}
