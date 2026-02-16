#!/usr/bin/env bash
# mlflow.sh - DEPRECATED: Use 'nself service mlflow' instead

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Show deprecation warning
printf "\033[0;33m⚠\033[0m  The 'nself mlflow' command is deprecated.\n"
printf "   Please use: \033[1mnself service mlflow\033[0m\n\n"

# Delegate to new command
exec "${SCRIPT_DIR}/service.sh" mlflow "$@"
