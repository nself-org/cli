#!/usr/bin/env bash
set -euo pipefail
# sync.sh - DEPRECATED - Redirects to nself deploy sync
# This file is kept for backward compatibility only
# Use: nself deploy sync instead

set -e

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
CLI_DIR="$(dirname "$SCRIPT_DIR")"

# Source the unified deploy command
source "$CLI_DIR/deploy.sh"

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  \033[2mDEPRECATED:\033[0m 'nself sync' has moved to 'nself deploy sync'\n"
printf "   Please update your scripts to use: \033[0;36mnself deploy sync\033[0m\n\n"

# Forward all arguments to deploy sync
cmd_deploy "sync" "$@"
