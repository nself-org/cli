#!/usr/bin/env bash
# vault.sh - DEPRECATED: Redirects to nself config vault
# This file maintained for backward compatibility only
# Use: nself config vault instead

set -euo pipefail

# Get script directory
CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Show deprecation warning before sourcing (config.sh may exit early on 'help')
printf "\033[0;33m⚠\033[0m \033[2mDeprecation Notice:\033[0m 'nself vault' is deprecated\n" >&2
printf "  \033[2mUse:\033[0m \033[0;36mnself config vault\033[0m instead\n\n" >&2

# Source the consolidated config command
source "$CLI_SCRIPT_DIR/config.sh"

# Redirect to config vault
cmd_config "vault" "$@"
