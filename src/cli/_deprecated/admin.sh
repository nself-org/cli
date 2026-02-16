#!/usr/bin/env bash
set -euo pipefail
# admin.sh - DEPRECATED - Use 'nself service admin' instead
# This file maintained for backward compatibility only

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Show deprecation warning
printf "\033[0;33m[DEPRECATED]\033[0m 'nself admin' is deprecated.\n"
printf "Use: \033[0;36mnself service admin\033[0m instead\n"
printf "\n"

# Forward to service command
exec bash "$SCRIPT_DIR/../service.sh" admin "$@"
