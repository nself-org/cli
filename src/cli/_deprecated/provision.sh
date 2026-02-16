#!/usr/bin/env bash
set -euo pipefail
# provision.sh - DEPRECATED - Redirects to nself deploy provision
# This file is kept for backward compatibility only
# Use: nself deploy provision instead

set -e

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
CLI_DIR="$(dirname "$SCRIPT_DIR")"

# Source the unified deploy command
source "$CLI_DIR/deploy.sh"

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  \033[2mDEPRECATED:\033[0m 'nself provision' has moved to 'nself deploy provision'\n"
printf "   Please update your scripts to use: \033[0;36mnself deploy provision\033[0m\n\n"

# Forward all arguments to deploy provision
cmd_deploy "provision" "$@"
