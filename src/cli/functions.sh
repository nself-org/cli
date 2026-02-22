#!/usr/bin/env bash
# functions.sh - DEPRECATED: Use 'nself service functions' instead
# Supports templates: basic, webhook, api, scheduled (typescript flag supported)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Intercept --help before delegating
if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  printf "DEPRECATION NOTICE\n\n"
  printf "  'nself functions' is deprecated and will be removed in v1.0.0.\n"
  printf "  Please use: nself service functions\n\n"
  printf "  Run 'nself service functions --help' for full usage information.\n"
  exit 0
fi

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  The 'nself functions' command is deprecated.\n"
printf "   Please use: \033[1mnself service functions\033[0m\n\n"

# Backward-compatibility stubs — functionality now lives in 'nself service functions'
# Uses safe_sed_inline for cross-platform config file editing
cmd_functions() {
  exec "${SCRIPT_DIR}/service.sh" functions "$@"
}

functions_create() {
  exec "${SCRIPT_DIR}/service.sh" functions create "$@"
}

functions_deploy() {
  exec "${SCRIPT_DIR}/service.sh" functions deploy "$@"
}

deploy_functions_local() {
  exec "${SCRIPT_DIR}/service.sh" functions deploy local "$@"
}

deploy_functions_production() {
  exec "${SCRIPT_DIR}/service.sh" functions deploy production "$@"
}

validate_functions() {
  exec "${SCRIPT_DIR}/service.sh" functions validate "$@"
}

create_typescript_function() {
  exec "${SCRIPT_DIR}/service.sh" functions create --typescript "$@"
}

# Delegate to new command
cmd_functions "$@"
