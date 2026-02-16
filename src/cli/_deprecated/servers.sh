#!/usr/bin/env bash
set -euo pipefail
# servers.sh - DEPRECATED - Redirects to nself deploy server
# This file is kept for backward compatibility only
# Use: nself deploy server instead

set -e

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
CLI_DIR="$(dirname "$SCRIPT_DIR")"

# Source the unified deploy command
source "$CLI_DIR/deploy.sh"

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  \033[2mDEPRECATED:\033[0m 'nself servers' has moved to 'nself deploy server'\n"
printf "   Please update your scripts to use: \033[0;36mnself deploy server\033[0m\n\n"

# Forward all arguments to deploy server (servers and server are now merged)
cmd_deploy "server" "$@"
