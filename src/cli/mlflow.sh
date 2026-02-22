#!/usr/bin/env bash
# mlflow.sh - DEPRECATED: Use 'nself service mlflow' instead

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Intercept --help before delegating
if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  printf "DEPRECATION NOTICE\n\n"
  printf "  'nself mlflow' is deprecated and will be removed in v1.0.0.\n"
  printf "  Please use: nself service mlflow\n\n"
  printf "  Run 'nself service mlflow --help' for full usage information.\n"
  exit 0
fi

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  The 'nself mlflow' command is deprecated.\n"
printf "   Please use: \033[1mnself service mlflow\033[0m\n\n"

# Backward-compatibility stubs — functionality now lives in 'nself service mlflow'
# Uses safe_sed_inline for cross-platform config file editing
cmd_mlflow() {
  exec "${SCRIPT_DIR}/service.sh" mlflow "$@"
}

mlflow_enable() {
  exec "${SCRIPT_DIR}/service.sh" mlflow enable "$@"
}

mlflow_status() {
  exec "${SCRIPT_DIR}/service.sh" mlflow status "$@"
}

mlflow_test() {
  exec "${SCRIPT_DIR}/service.sh" mlflow test "$@"
}

# Experiment management: experiments create / experiments delete
mlflow_experiments() {
  exec "${SCRIPT_DIR}/service.sh" mlflow experiments "$@"
}

mlflow_runs() {
  exec "${SCRIPT_DIR}/service.sh" mlflow runs "$@"
}

# Delegate to new command
cmd_mlflow "$@"
