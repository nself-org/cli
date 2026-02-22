#!/usr/bin/env bash
# prod.sh - DEPRECATED: Use 'nself deploy production' instead

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Intercept --help before delegating
if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  printf "DEPRECATION NOTICE\n\n"
  printf "  'nself prod' is deprecated and will be removed in v1.0.0.\n"
  printf "  Please use: nself deploy production\n\n"
  printf "  Run 'nself deploy production --help' for full usage information.\n"
  exit 0
fi

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  The 'nself prod' command is deprecated.\n"
printf "   Please use: \033[1mnself deploy production\033[0m\n\n"

# Delegate to new command
exec "${SCRIPT_DIR}/deploy.sh" production "$@"
