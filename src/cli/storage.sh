#!/usr/bin/env bash
# storage.sh - DEPRECATED: Use 'nself service storage' instead

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Intercept --help before delegating
if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  printf "DEPRECATION NOTICE\n\n"
  printf "  'nself storage' is deprecated and will be removed in v1.0.0.\n"
  printf "  Please use: nself service storage\n\n"
  printf "  Run 'nself service storage --help' for full usage information.\n"
  exit 0
fi

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  The 'nself storage' command is deprecated.\n"
printf "   Please use: \033[1mnself service storage\033[0m\n\n"

# Delegate to new command
exec "${SCRIPT_DIR}/service.sh" storage "$@"
