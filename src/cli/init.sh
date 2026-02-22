#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

# init.sh - Thin wrapper for nself init command
#
# This script serves as the entry point for the init command and delegates
# all functionality to modular components in src/lib/init/

# Get script directory (cross-platform)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Determine the lib directory location
LIB_DIR=""
if [[ -d "$SCRIPT_DIR/../lib/init" ]]; then
  LIB_DIR="$SCRIPT_DIR/../lib/init"
elif [[ -d "$SCRIPT_DIR/../../src/lib/init" ]]; then
  LIB_DIR="$SCRIPT_DIR/../../src/lib/init"
elif [[ -d "/usr/share/nself/src/lib/init" ]]; then
  LIB_DIR="/usr/share/nself/src/lib/init"
elif [[ -d "$HOME/.nself/src/lib/init" ]]; then
  LIB_DIR="$HOME/.nself/src/lib/init"
else
  echo "Error: Cannot find init module directory" >&2
  echo "Searched in:" >&2
  echo "  - $SCRIPT_DIR/../lib/init" >&2
  echo "  - $SCRIPT_DIR/../../src/lib/init" >&2
  echo "  - /usr/share/nself/src/lib/init" >&2
  echo "  - $HOME/.nself/src/lib/init" >&2
  exit 78
fi

# Source the core module (which sources all other required modules)
if [[ -f "$LIB_DIR/core.sh" ]]; then
  source "$LIB_DIR/core.sh"
else
  echo "Error: Core module not found at $LIB_DIR/core.sh" >&2
  exit 78
fi

# Set up error handling with cleanup
trap init_cleanup EXIT INT TERM

# Execute the main command function
cmd_init "$SCRIPT_DIR" "$@"
exit_code=$?

# Cleanup is handled by trap
exit $exit_code
