#!/usr/bin/env bash
# email.sh - DEPRECATED: Use 'nself service email' instead

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Intercept --help before delegating
if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  printf "DEPRECATION NOTICE\n\n"
  printf "  'nself email' is deprecated and will be removed in v1.0.0.\n"
  printf "  Please use: nself service email\n\n"
  printf "  Run 'nself service email --help' for full usage information.\n"
  exit 0
fi

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  The 'nself email' command is deprecated.\n"
printf "   Please use: \033[1mnself service email\033[0m\n\n"

# Delegate to new command
exec "${SCRIPT_DIR}/service.sh" email "$@"
