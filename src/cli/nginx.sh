#!/usr/bin/env bash

# nginx.sh - Multi-project shared nginx management
# Manages a shared nginx instance that aggregates routes from multiple nself projects

set -euo pipefail

# Source shared utilities
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$CLI_SCRIPT_DIR"
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/header.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh"

# Show help
show_nginx_help() {
  cat <<'EOF'
nself nginx - Multi-project shared nginx management

Usage: nself nginx <subcommand> [options]

Subcommands:
  register              Register this project with the shared nginx
  unregister            Remove this project from the shared nginx
  shared <action>       Manage the shared nginx container

Shared Actions:
  shared start          Start the shared nginx container
  shared stop           Stop the shared nginx container
  shared status         Show all registered projects and routes
  shared reload         Reload nginx configuration
  shared logs           Tail shared nginx logs

Options:
  --path PATH           Project path (default: current directory)
  --tail N              Number of log lines to show (default: 100)
  -h, --help            Show this help message

Examples:
  nself nginx register                       # Register current project
  nself nginx register --path /opt/backend   # Register specific project
  nself nginx unregister                     # Remove current project
  nself nginx shared start                   # Start shared nginx
  nself nginx shared status                  # Show registered projects
  nself nginx shared reload                  # Reload after config change
  nself nginx shared logs --tail 50          # View recent logs
EOF
}

# Show shared subcommand help
show_shared_help() {
  cat <<'EOF'
nself nginx shared - Manage the shared nginx container

Usage: nself nginx shared <action> [options]

Actions:
  start                 Start the shared nginx container
  stop                  Stop the shared nginx container
  status                Show all registered projects and routes
  reload                Reload nginx configuration (after nself build)
  logs                  Tail shared nginx logs

Options:
  --tail N              Number of log lines for 'logs' (default: 100)
  -h, --help            Show this help message

Examples:
  nself nginx shared start          # Launch shared nginx
  nself nginx shared status         # Show projects and routes
  nself nginx shared reload         # Apply config changes
  nself nginx shared logs --tail 20 # View last 20 lines
EOF
}

# Register current project with shared nginx
nginx_register() {
  local project_path="${PROJECT_PATH:-$(pwd)}"

  # Source required libraries (lazy load)
  source "$CLI_SCRIPT_DIR/../lib/nginx/registry.sh"
  source "$CLI_SCRIPT_DIR/../lib/nginx/port-allocator.sh"
  source "$CLI_SCRIPT_DIR/../lib/nginx/conflict-check.sh" 2>/dev/null || true

  show_command_header "nself nginx" "Registering project"

  # Validate project directory
  if [[ ! -d "$project_path" ]]; then
    log_error "Project path does not exist: $project_path"
    return 1
  fi

  if [[ ! -d "$project_path/nginx/sites" ]]; then
    log_error "No nginx/sites/ directory found at $project_path"
    log_info "Run 'nself build' first to generate nginx configuration"
    return 1
  fi

  if [[ ! -f "$project_path/.env" ]]; then
    log_error "No .env file found at $project_path"
    return 1
  fi

  # Extract project name and base domain from .env
  local project_name=""
  local base_domain=""
  if [[ -f "$project_path/.env" ]]; then
    project_name=$(grep -E '^PROJECT_NAME=' "$project_path/.env" | cut -d'=' -f2 | tr -d '"' | tr -d "'")
    base_domain=$(grep -E '^BASE_DOMAIN=' "$project_path/.env" | cut -d'=' -f2 | tr -d '"' | tr -d "'")
  fi

  if [[ -z "$project_name" ]]; then
    project_name=$(basename "$project_path")
  fi
  if [[ -z "$base_domain" ]]; then
    base_domain="localhost"
  fi

  # Check if already registered
  registry::init
  if registry::is_registered "$project_path"; then
    log_error "Project at $project_path is already registered"
    log_info "Run 'nself nginx unregister' first to re-register"
    return 1
  fi

  # Allocate port range
  local registry_file="$HOME/.nself/nginx/registry.json"
  local range
  range=$(ports::allocate_range "$registry_file")
  local port_start
  local port_end
  port_start=$(printf '%s' "$range" | cut -d':' -f1)
  port_end=$(printf '%s' "$range" | cut -d':' -f2)

  # Check for subdomain conflicts
  if type conflicts::check_new_project >/dev/null 2>&1; then
    if ! conflicts::check_new_project "$project_path"; then
      log_error "Registration aborted due to subdomain conflicts"
      return 1
    fi
  fi

  # Add to registry
  registry::add_project "$project_name" "$project_path" "$base_domain" "$port_start" "$port_end"

  log_success "Project registered with shared nginx"
  printf "  Name:        %s\n" "$project_name"
  printf "  Path:        %s\n" "$project_path"
  printf "  Base domain: %s\n" "$base_domain"
  printf "  Port range:  %s-%s\n" "$port_start" "$port_end"
  printf "\n"
  log_info "Run 'nself nginx shared start' or 'nself nginx shared reload' to apply"
}

# Unregister current project from shared nginx
nginx_unregister() {
  local project_path="${PROJECT_PATH:-$(pwd)}"

  # Source required libraries
  source "$CLI_SCRIPT_DIR/../lib/nginx/registry.sh"
  source "$CLI_SCRIPT_DIR/../lib/nginx/shared.sh" 2>/dev/null || true

  show_command_header "nself nginx" "Unregistering project"

  registry::init

  # Find project name by path
  local project_name=""
  local registry_file="$HOME/.nself/nginx/registry.json"
  if [[ -f "$registry_file" ]]; then
    project_name=$(jq -r --arg path "$project_path" '.projects[] | select(.path == $path) | .name' "$registry_file" 2>/dev/null)
  fi

  if [[ -z "$project_name" ]]; then
    log_error "Project at $project_path is not registered"
    return 1
  fi

  registry::remove_project "$project_name"

  log_success "Project '$project_name' unregistered from shared nginx"

  # Warn if shared nginx is running
  if type shared::is_running >/dev/null 2>&1 && shared::is_running; then
    log_info "Shared nginx is running. Run 'nself nginx shared reload' to apply changes"
  fi
}

# Handle shared subcommands
nginx_shared() {
  local action="${1:-status}"
  shift || true

  # Source required libraries
  source "$CLI_SCRIPT_DIR/../lib/nginx/registry.sh"
  source "$CLI_SCRIPT_DIR/../lib/nginx/shared.sh"

  local tail_lines=100

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --tail)
        tail_lines="$2"
        shift 2
        ;;
      -h|--help)
        show_shared_help
        return 0
        ;;
      *)
        shift
        ;;
    esac
  done

  case "$action" in
    start)
      show_command_header "nself nginx shared" "Starting shared nginx"
      registry::init
      local count
      count=$(registry::project_count)
      if [[ "$count" -eq 0 ]]; then
        log_error "No projects registered. Run 'nself nginx register' first"
        return 1
      fi
      local registry_file="$HOME/.nself/nginx/registry.json"
      shared::generate_main_conf
      shared::generate_compose "$registry_file"
      shared::start
      log_success "Shared nginx started with $count registered project(s)"
      ;;
    stop)
      show_command_header "nself nginx shared" "Stopping shared nginx"
      shared::stop
      log_success "Shared nginx stopped"
      ;;
    status)
      show_command_header "nself nginx shared" "Status"
      registry::init
      local count
      count=$(registry::project_count)
      printf "\n"

      if shared::is_running; then
        printf "  Container: %b%s%b\n" "\033[0;32m" "running" "\033[0m"
      else
        printf "  Container: %b%s%b\n" "\033[0;31m" "stopped" "\033[0m"
      fi
      printf "  Projects:  %s registered\n\n" "$count"

      if [[ "$count" -gt 0 ]]; then
        printf "  %-20s %-30s %-15s %s\n" "NAME" "PATH" "DOMAIN" "PORTS"
        printf "  %-20s %-30s %-15s %s\n" "----" "----" "------" "-----"
        local registry_file="$HOME/.nself/nginx/registry.json"
        jq -r '.projects[] | "  \(.name)|\(.path)|\(.baseDomain)|\(.portStart)-\(.portEnd)"' "$registry_file" 2>/dev/null | while IFS='|' read -r name path domain ports; do
          printf "  %-20s %-30s %-15s %s\n" "$name" "$path" "$domain" "$ports"
        done
      fi
      ;;
    reload)
      show_command_header "nself nginx shared" "Reloading configuration"
      if ! shared::is_running; then
        log_error "Shared nginx is not running. Start it with 'nself nginx shared start'"
        return 1
      fi
      local registry_file="$HOME/.nself/nginx/registry.json"
      shared::generate_main_conf
      shared::generate_compose "$registry_file"
      shared::reload
      log_success "Shared nginx configuration reloaded"
      ;;
    logs)
      shared::logs "$tail_lines"
      ;;
    -h|--help)
      show_shared_help
      ;;
    *)
      log_error "Unknown shared action: $action"
      show_shared_help
      return 1
      ;;
  esac
}

# Main command dispatcher
cmd_nginx() {
  local subcommand="${1:-help}"

  # Check for help first
  if [[ "$subcommand" == "-h" ]] || [[ "$subcommand" == "--help" ]]; then
    show_nginx_help
    return 0
  fi

  # Parse global options
  local args=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --path)
        PROJECT_PATH="$2"
        shift 2
        ;;
      -h|--help)
        show_nginx_help
        return 0
        ;;
      *)
        args+=("$1")
        shift
        ;;
    esac
  done

  # Restore positional arguments (Bash 3.2 safe — empty array check)
  set -- ${args[@]+"${args[@]}"}
  subcommand="${1:-help}"

  case "$subcommand" in
    register)
      shift
      nginx_register "$@"
      ;;
    unregister)
      shift
      nginx_unregister "$@"
      ;;
    shared)
      shift
      nginx_shared "$@"
      ;;
    help)
      show_nginx_help
      ;;
    *)
      log_error "Unknown subcommand: $subcommand"
      show_nginx_help
      return 1
      ;;
  esac
}

# Export for use as library
export -f cmd_nginx

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_nginx_help
      exit 0
    fi
  done
  pre_command "nginx" || exit $?
  cmd_nginx "$@"
  exit_code=$?
  post_command "nginx" $exit_code
  exit $exit_code
fi
