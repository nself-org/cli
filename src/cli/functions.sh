#!/usr/bin/env bash
# functions.sh - DEPRECATED: Use 'nself service functions' instead

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Intercept --help before delegating
if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  printf "DEPRECATION NOTICE\n\n"
  printf "  'nself functions' is deprecated and will be removed in v1.0.0.\n"
  printf "  Please use: nself service functions\n\n"
  printf "  Run 'nself service functions --help' for full usage information.\n"
  exit 0
fi

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  The 'nself functions' command is deprecated.\n"
printf "   Please use: \033[1mnself service functions\033[0m\n\n"

# Delegate to new command
exec "${SCRIPT_DIR}/service.sh" functions "$@"
