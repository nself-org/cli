#!/usr/bin/env bash
set -euo pipefail

# down.sh - Alias for stop.sh (backward compatibility)

# Get script directory
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"

# Source and call stop command
source "$SCRIPT_DIR/stop.sh"

# Export for use as library
export -f cmd_stop

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_stop "$@"
fi
