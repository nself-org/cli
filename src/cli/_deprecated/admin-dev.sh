#!/usr/bin/env bash
set -euo pipefail
# admin-dev.sh - DEPRECATED - Use 'nself service admin dev' instead
# This file maintained for backward compatibility only

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Show deprecation warning
printf "\033[0;33m[DEPRECATED]\033[0m 'nself admin-dev' is deprecated.\n"
printf "Use: \033[0;36mnself service admin dev\033[0m instead\n"
printf "\n"

# Map old commands to new structure
case "${1:-status}" in
  on|enable)
    shift
    exec bash "$SCRIPT_DIR/../service.sh" admin dev enable "$@"
    ;;
  off|disable)
    exec bash "$SCRIPT_DIR/../service.sh" admin dev disable
    ;;
  status|"")
    exec bash "$SCRIPT_DIR/../service.sh" admin dev status
    ;;
  *)
    # Forward as-is
    exec bash "$SCRIPT_DIR/../service.sh" admin dev "$@"
    ;;
esac
