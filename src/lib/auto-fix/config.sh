#!/usr/bin/env bash

# config.sh - Fix configuration issues

fix_missing_config() {

set -euo pipefail

  local config_file="$1"

  if [[ "$config_file" == ".env.local" ]]; then
    log_info "Creating default configuration..."
    bash "$SCRIPT_DIR/../../cli/init.sh"
    return 0
  fi

  # Remove inline comments from env files
  if [[ "$config_file" =~ \.env ]]; then
    log_info "Cleaning inline comments from $config_file..."
    sed -i.bak 's/\s*#.*$//' "$config_file"
    return 0
  fi

  return 1
}

export -f fix_missing_config
