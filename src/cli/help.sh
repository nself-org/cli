#!/usr/bin/env bash
set -euo pipefail

# help.sh - Show help information

# Get script directory with absolute path
CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_DIR="$CLI_SCRIPT_DIR"

# Source shared utilities
[[ -z "${DISPLAY_SOURCED:-}" ]] && source "$CLI_SCRIPT_DIR/../lib/utils/display.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh"
[[ -z "${CONSTANTS_SOURCED:-}" ]] && source "$CLI_SCRIPT_DIR/../lib/config/constants.sh"

# Command function
cmd_help() {
  local command="${1:-}"

  # Handle --help/-h on the help command itself
  if [[ "$command" == "--help" ]] || [[ "$command" == "-h" ]] || [[ -z "$command" ]]; then
    show_general_help
  else
    # Show help for specific command
    show_command_help "$command"
  fi
}

# Show general help
show_general_help() {
  # Get version from VERSION file
  local version="unknown"
  if [[ -f "$SCRIPT_DIR/../VERSION" ]]; then
    version=$(cat "$SCRIPT_DIR/../VERSION" 2>/dev/null || echo "unknown")
  fi

  show_command_header "ɳSelf v${version}" "Self-Hosted Infrastructure Manager"
  echo
  printf "${COLOR_DIM}USAGE${COLOR_RESET}\n"
  printf "  nself ${COLOR_BLUE}<command>${COLOR_RESET} [options]\n"

  show_section "CORE COMMANDS"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Initialize new project\n" "init"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Build project and Docker images\n" "build"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Start all services\n" "start"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Stop all services\n" "stop"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Restart all services\n" "restart"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Reset to clean state\n" "reset"

  show_section "STATUS & MONITORING"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Show service status\n" "status"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} View service logs\n" "logs"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Show all service URLs\n" "urls"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Run system diagnostics\n" "doctor"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Health check management\n" "health"

  show_section "DATABASE OPERATIONS"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Database tools\n" "db"
  printf "  ${COLOR_DIM}%16s migrate, seed, mock, backup, restore, schema, types${COLOR_RESET}\n" ""

  show_section "SERVICE MANAGEMENT"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Manage optional services\n" "service"
  printf "  ${COLOR_DIM}%16s list, enable, disable, status, restart, logs${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s email: test, inbox, config${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s search: index, query, stats${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s functions: deploy, invoke, list${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s mlflow: ui, experiments, runs, artifacts${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s storage: buckets, upload, download, presign${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s cache: stats, flush, keys${COLOR_RESET}\n" ""
  echo
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} SSL certificate management\n" "ssl"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Trust local SSL certificates\n" "trust"

  show_section "INFRASTRUCTURE & DEPLOYMENT"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Provider infrastructure (v0.9.6)\n" "provider"
  printf "  ${COLOR_DIM}%16s info: list, init, validate, show${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s server: create, destroy, list, ssh${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s cost: estimate, compare${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s deploy: quick, full${COLOR_RESET}\n" ""
  echo
  printf "  ${COLOR_DIM}%2s Providers: DigitalOcean, Linode, Vultr, Hetzner, OVH,${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%13s Scaleway, AWS, GCP, Azure, Oracle, IBM + 15 more${COLOR_RESET}\n" ""
  echo
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Deploy with advanced strategies\n" "deploy"
  printf "  ${COLOR_DIM}%16s staging, production, rollback${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s preview: ephemeral environments${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s canary: percentage-based rollout${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s blue-green: instant switching${COLOR_RESET}\n" ""
  echo
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Environment management (local/staging/prod)\n" "env"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Sync data between environments\n" "sync"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Production hardening\n" "prod"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Staging environment\n" "staging"

  show_section "KUBERNETES & HELM"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Kubernetes management\n" "k8s"
  printf "  ${COLOR_DIM}%16s init, convert, apply, deploy, scale, rollback${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s cluster: list, connect, info${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s namespace: list, create, delete, switch${COLOR_RESET}\n" ""
  echo
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Helm chart management\n" "helm"
  printf "  ${COLOR_DIM}%16s init, install, upgrade, rollback, uninstall${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s repo: add, remove, update, list${COLOR_RESET}\n" ""

  show_section "PERFORMANCE & SCALING"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Performance profiling and analysis\n" "perf"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Benchmarking and load testing\n" "bench"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Service scaling and autoscaling\n" "scale"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Cross-environment migration\n" "migrate"

  show_section "PLUGINS & INTEGRATIONS"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Plugin management and execution\n" "plugin"
  printf "  ${COLOR_DIM}%16s list, install, remove, update, status${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s Available: stripe, shopify, github${COLOR_RESET}\n" ""

  show_section "MULTI-TENANCY"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Multi-tenant management\n" "tenant"
  printf "  ${COLOR_DIM}%16s init, create, list, show, suspend, activate${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s billing: usage, invoices, quotas, plans${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s branding: logo, colors, themes, css${COLOR_RESET}\n" ""
  printf "  ${COLOR_DIM}%16s domains: custom domains, SSL, verification${COLOR_RESET}\n" ""

  show_section "OPERATIONS"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Execute commands in containers\n" "exec"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Clean up Docker resources\n" "clean"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Restore from backup\n" "restore"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Frontend app management\n" "frontend"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Configuration management\n" "config"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Operation audit trail\n" "history"

  show_section "UTILITIES"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} CI/CD workflow generation (GitHub/GitLab)\n" "ci"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Shell completion (bash/zsh/fish)\n" "completion"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Show version information\n" "version"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Update nself to latest\n" "update"
  printf "  ${COLOR_BLUE}%-14s${COLOR_RESET} Show this help message\n" "help"

  show_section "DEPRECATED"
  printf "  ${COLOR_YELLOW}%-14s${COLOR_RESET} ${COLOR_DIM}Use 'provider' instead (removed in v1.0.0)${COLOR_RESET}\n" "cloud"

  echo
  printf "${COLOR_DIM}EXAMPLES${COLOR_RESET}\n"
  printf "  nself init --demo              # Initialize with demo config\n"
  printf "  nself build && nself start     # Build and start services\n"
  printf "  nself status                   # Check service health\n"
  printf "  nself logs api                 # View API service logs\n"
  printf "  nself db migrate up            # Run database migrations\n"
  printf "  nself provider server create   # Create cloud server\n"
  echo
  printf "${COLOR_DIM}HELP${COLOR_RESET}\n"
  printf "  nself help <command>           # Command-specific help\n"
  printf "  nself <command> --help         # Alternative help syntax\n"
  echo
  printf "${COLOR_DIM}LEARN MORE${COLOR_RESET}\n"
  printf "  Docs:    https://nself.org/docs\n"
  printf "  GitHub:  https://github.com/nself-org/cli\n"
  printf "  Issues:  https://github.com/nself-org/cli/issues\n"
  echo
}

# Show command-specific help
show_command_help() {
  local command="$1"
  local command_file=""

  # Find command file (check multiple locations)
  if [[ -f "$SCRIPT_DIR/${command}.sh" ]]; then
    command_file="$SCRIPT_DIR/${command}.sh"
  elif [[ -f "$SCRIPT_DIR/../tools/dev/${command}.sh" ]]; then
    command_file="$SCRIPT_DIR/../tools/dev/${command}.sh"
  fi

  # Check if command exists
  if [[ -z "$command_file" ]] || [[ ! -f "$command_file" ]]; then
    log_error "Unknown command: $command"
    echo
    echo "Run 'nself help' to see available commands"
    return 1
  fi

  # Try to run the command with --help
  if bash "$command_file" --help 2>/dev/null; then
    return 0
  else
    # Fallback: show basic info
    echo "Help for: nself $command"
    echo
    echo "Run: nself $command --help"
    echo "Or check the documentation"
  fi
}

# Export for use as library
export -f cmd_help

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]] || [[ -z "$*" ]]; then
      cmd_help "$@"
      exit 0
    fi
  done
  pre_command "help" || exit $?
  cmd_help "$@"
  exit_code=$?
  post_command "help" $exit_code
  exit $exit_code
fi
