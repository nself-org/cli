#!/usr/bin/env bash

#
# Output utility functions
# Simple wrappers around display.sh for backward compatibility
#

# Source display.sh if not already loaded (use local variable to avoid conflicts)
if [[ -z "${DISPLAY_SOURCED:-}" ]]; then

set -euo pipefail

  _OUTPUT_SH_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  source "$_OUTPUT_SH_DIR/display.sh"
  unset _OUTPUT_SH_DIR
fi

# Backward compatibility aliases
info() {
  log_info "$@"
}

success() {
  log_success "$@"
}

warning() {
  log_warning "$@"
}

error() {
  log_error "$@"
}

debug() {
  log_debug "$@"
}

header() {
  log_header "$@"
}
