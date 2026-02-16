#!/usr/bin/env bash
# providers.sh - DEPRECATED: Use 'nself infra provider' instead

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Show deprecation warning
printf "\033[0;33m⚠\033[0m  The 'nself providers' command is deprecated.\n"
printf "   Please use: \033[1mnself infra provider\033[0m\n\n"

# Delegate to new command
exec "${SCRIPT_DIR}/infra.sh" provider "$@"
