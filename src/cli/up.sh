#!/usr/bin/env bash
set -euo pipefail

# up.sh - Alias for start.sh (backward compatibility)

# Get script directory
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"

# Source and call start command
source "$SCRIPT_DIR/start.sh"

# Export for use as library
export -f cmd_start

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_start "$@"
fi
