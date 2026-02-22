#!/usr/bin/env bash
# provision.sh - DEPRECATED: Use 'nself deploy provision' instead

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Show deprecation warning
printf "\033[0;33m⚠\033[0m  The 'nself provision' command is deprecated.\n"
printf "   Please use: \033[1mnself deploy provision\033[0m\n\n"

# Delegate to new command
exec "${SCRIPT_DIR}/deploy.sh" provision "$@"
