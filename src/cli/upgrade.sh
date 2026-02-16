#!/usr/bin/env bash
# upgrade.sh - DEPRECATED: Use 'nself deploy upgrade' instead

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Intercept --help before delegating
if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  printf "DEPRECATION NOTICE\n\n"
  printf "  'nself upgrade' is deprecated and will be removed in v1.0.0.\n"
  printf "  Please use: nself deploy upgrade\n\n"
  printf "  Run 'nself deploy upgrade --help' for full usage information.\n"
  exit 0
fi

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  The 'nself upgrade' command is deprecated.\n"
printf "   Please use: \033[1mnself deploy upgrade\033[0m\n\n"

# Delegate to new command
exec "${SCRIPT_DIR}/deploy.sh" upgrade "$@"
