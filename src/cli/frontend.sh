#!/usr/bin/env bash

# frontend.sh - Frontend application management and deployment tracking
# v0.4.6 - Feedback implementation

set -euo pipefail

# Source shared utilities
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$CLI_SCRIPT_DIR"
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/header.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh"

# Color fallbacks
: "${COLOR_GREEN:=\033[0;32m}"
: "${COLOR_YELLOW:=\033[0;33m}"
: "${COLOR_RED:=\033[0;31m}"
: "${COLOR_CYAN:=\033[0;36m}"
: "${COLOR_RESET:=\033[0m}"
: "${COLOR_DIM:=\033[2m}"
: "${COLOR_BOLD:=\033[1m}"

# Show help
show_frontend_help() {
  cat <<'EOF'
nself frontend - Frontend application management

Usage: nself frontend <subcommand> [options]

Subcommands:
  status                Show frontend deployment status
  list                  List configured frontend apps
  add <name>            Add a new frontend application
  remove <name>         Remove a frontend application
  deploy <name>         Deploy frontend (Vercel/Netlify integration)
  logs <name>           View frontend build/deploy logs
  env <name>            Show environment variables for frontend

Options:
  --port N              Frontend port (default: 3000)
  --route PATH          Route prefix (default: app name)
  --provider NAME       Deployment provider (vercel, netlify, none)
  --env NAME            Target environment
  --json                Output in JSON format
  -h, --help            Show this help message

Examples:
  nself frontend status                    # Show all frontends
  nself frontend add webapp --port 3000    # Add frontend app
  nself frontend deploy webapp --env prod  # Deploy to production
  nself frontend logs webapp               # View deploy logs
  nself frontend env webapp                # Show environment vars
EOF
}

# Initialize frontend environment
init_frontend() {
  load_env_with_priority

  FRONTEND_DIR="${FRONTEND_DIR:-.nself/frontend}"
  mkdir -p "$FRONTEND_DIR"

  PROJECT_NAME="${PROJECT_NAME:-nself}"
  BASE_DOMAIN="${BASE_DOMAIN:-local.nself.org}"
}

# Get configured frontends from environment
get_frontends() {
  local frontends=()

  # Check FRONTEND_APP_N pattern
  for i in {1..20}; do
    local name_var="FRONTEND_APP_${i}_NAME"
    local port_var="FRONTEND_APP_${i}_PORT"
    local route_var="FRONTEND_APP_${i}_ROUTE"

    local name="${!name_var:-}"
    local port="${!port_var:-}"
    local route="${!route_var:-}"

    if [[ -n "$name" ]]; then
      frontends+=("${name}:${port}:${route}")
    fi
  done

  # Also check APP_N pattern (legacy)
  for i in {1..20}; do
    local name_var="APP_${i}_NAME"
    local port_var="APP_${i}_PORT"

    local name="${!name_var:-}"
    local port="${!port_var:-}"

    if [[ -n "$name" ]]; then
      # Check if already added
      local exists=false
      for f in "${frontends[@]}"; do
        [[ "${f%%:*}" == "$name" ]] && exists=true
      done
      [[ "$exists" == "false" ]] && frontends+=("${name}:${port}:${name}")
    fi
  done

  [[ "${#frontends[@]}" -gt 0 ]] && printf '%s\n' "${frontends[@]}"
}

# Check if frontend is running
check_frontend_status() {
  local name="$1"
  local port="$2"

  # Check if process is listening on port
  if command -v lsof >/dev/null 2>&1; then
    if lsof -i ":$port" >/dev/null 2>&1; then
      echo "running"
      return 0
    fi
  elif command -v netstat >/dev/null 2>&1; then
    if netstat -an 2>/dev/null | grep -q ":$port.*LISTEN"; then
      echo "running"
      return 0
    fi
  fi

  # Check if there's a PID file
  if [[ -f "${FRONTEND_DIR}/${name}.pid" ]]; then
    local pid=$(cat "${FRONTEND_DIR}/${name}.pid")
    if kill -0 "$pid" 2>/dev/null; then
      echo "running"
      return 0
    fi
  fi

  echo "stopped"
}

# Show frontend status
cmd_status() {
  local json_mode="${JSON_OUTPUT:-false}"

  init_frontend

  if [[ "$json_mode" != "true" ]]; then
    show_command_header "nself frontend" "Frontend Status"
    echo ""
  fi

  local frontends=($(get_frontends))

  if [[ ${#frontends[@]} -eq 0 ]]; then
    if [[ "$json_mode" == "true" ]]; then
      echo '{"frontends": [], "message": "No frontend applications configured"}'
    else
      log_info "No frontend applications configured"
      echo ""
      log_info "Add a frontend with: nself frontend add <name> --port <port>"
    fi
    return 0
  fi

  if [[ "$json_mode" != "true" ]]; then
    printf "${COLOR_CYAN}➞ Frontend Applications${COLOR_RESET}\n"
    echo ""
    printf "  %-20s %-8s %-15s %-12s %s\n" "Name" "Port" "Route" "Status" "URL"
    printf "  %-20s %-8s %-15s %-12s %s\n" "----" "----" "-----" "------" "---"
  fi

  local json_array="["
  local first=true

  for entry in "${frontends[@]}"; do
    local name="${entry%%:*}"
    local rest="${entry#*:}"
    local port="${rest%%:*}"
    local route="${rest#*:}"

    local status=$(check_frontend_status "$name" "$port")
    local url="https://${route}.${BASE_DOMAIN}"

    if [[ "$first" != "true" ]]; then
      json_array+=","
    fi
    first=false

    json_array+="{\"name\": \"$name\", \"port\": $port, \"route\": \"$route\", \"status\": \"$status\", \"url\": \"$url\"}"

    if [[ "$json_mode" != "true" ]]; then
      local status_color="$COLOR_RED"
      [[ "$status" == "running" ]] && status_color="$COLOR_GREEN"

      printf "  %-20s %-8s %-15s ${status_color}%-12s${COLOR_RESET} %s\n" \
        "$name" "$port" "$route" "$status" "$url"
    fi
  done

  json_array+="]"

  if [[ "$json_mode" == "true" ]]; then
    printf '{"frontends": %s}\n' "$json_array"
  else
    echo ""

    # Check for deployment integrations
    printf "${COLOR_CYAN}➞ Deployment Integrations${COLOR_RESET}\n"
    echo ""

    if [[ -f "vercel.json" ]] || [[ -d ".vercel" ]]; then
      printf "  ✓ Vercel configured\n"
    fi
    if [[ -f "netlify.toml" ]] || [[ -d ".netlify" ]]; then
      printf "  ✓ Netlify configured\n"
    fi
    if [[ ! -f "vercel.json" ]] && [[ ! -f "netlify.toml" ]]; then
      printf "  ${COLOR_DIM}No deployment providers configured${COLOR_RESET}\n"
    fi
  fi
}

# List frontends
cmd_list() {
  local json_mode="${JSON_OUTPUT:-false}"

  init_frontend

  local frontends
  frontends=($(get_frontends)) || true

  if [[ "${#frontends[@]}" -eq 0 ]]; then
    [[ "$json_mode" == "true" ]] && printf '{"frontends": []}\n' || true
    return 0
  fi

  if [[ "$json_mode" == "true" ]]; then
    printf '{"frontends": ['
    local first=true
    for entry in "${frontends[@]}"; do
      local name="${entry%%:*}"
      [[ "$first" != "true" ]] && printf ","
      first=false
      printf '"%s"' "$name"
    done
    printf ']}\n'
  else
    for entry in "${frontends[@]}"; do
      echo "${entry%%:*}"
    done
  fi
}

# Add frontend application
cmd_add() {
  local name="$1"
  local port="${FRONTEND_PORT:-3000}"
  local route="${FRONTEND_ROUTE:-$name}"

  if [[ -z "$name" ]]; then
    log_error "Frontend name required"
    return 1
  fi

  init_frontend

  show_command_header "nself frontend" "Adding $name"
  echo ""

  # Find next available slot
  local slot=0
  for i in {1..20}; do
    local name_var="FRONTEND_APP_${i}_NAME"
    if [[ -z "${!name_var:-}" ]]; then
      slot=$i
      break
    fi
  done

  if [[ "$slot" -eq 0 ]]; then
    log_error "Maximum frontend apps (20) reached"
    return 1
  fi

  printf "${COLOR_CYAN}➞ Configuration${COLOR_RESET}\n"
  printf "  Name: %s\n" "$name"
  printf "  Port: %s\n" "$port"
  printf "  Route: %s.%s\n" "$route" "$BASE_DOMAIN"
  printf "  Slot: FRONTEND_APP_%s\n" "$slot"
  echo ""

  # Add to .env
  local env_file=".env"
  if [[ -f "$env_file" ]]; then
    echo "" >>"$env_file"
    echo "# Frontend: $name" >>"$env_file"
    echo "FRONTEND_APP_${slot}_NAME=$name" >>"$env_file"
    echo "FRONTEND_APP_${slot}_PORT=$port" >>"$env_file"
    echo "FRONTEND_APP_${slot}_ROUTE=$route" >>"$env_file"

    log_success "Frontend added to .env"
    log_info "Run 'nself build && nself restart nginx' to apply"
  else
    log_warning ".env file not found"
    echo ""
    echo "Add these lines to your .env:"
    echo "  FRONTEND_APP_${slot}_NAME=$name"
    echo "  FRONTEND_APP_${slot}_PORT=$port"
    echo "  FRONTEND_APP_${slot}_ROUTE=$route"
  fi
}

# Remove frontend
cmd_remove() {
  local name="$1"

  if [[ -z "$name" ]]; then
    log_error "Frontend name required"
    return 1
  fi

  init_frontend

  show_command_header "nself frontend" "Removing $name"
  echo ""

  log_warning "This will remove the frontend configuration"
  log_info "The application files will not be deleted"
  echo ""

  read -p "Continue? (y/N) " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "Removal cancelled"
    return 1
  fi

  # Find and comment out in .env
  local env_file=".env"
  if [[ -f "$env_file" ]]; then
    # Create backup
    cp "$env_file" "${env_file}.bak"

    # Find the slot
    for i in {1..20}; do
      local name_var="FRONTEND_APP_${i}_NAME"
      if [[ "${!name_var:-}" == "$name" ]]; then
        # Comment out the lines
        if [[ "$OSTYPE" == "darwin"* ]]; then
          sed -i '' "s/^FRONTEND_APP_${i}_/#FRONTEND_APP_${i}_/" "$env_file"
        else
          sed -i "s/^FRONTEND_APP_${i}_/#FRONTEND_APP_${i}_/" "$env_file"
        fi
        break
      fi
    done

    log_success "Frontend configuration commented out"
    log_info "Backup saved to ${env_file}.bak"
    log_info "Run 'nself build && nself restart nginx' to apply"
  else
    log_error ".env file not found"
    return 1
  fi
}

# Deploy frontend
cmd_deploy() {
  local name="$1"
  local target_env="${TARGET_ENV:-production}"

  if [[ -z "$name" ]]; then
    log_error "Frontend name required"
    return 1
  fi

  init_frontend

  show_command_header "nself frontend" "Deploying $name"
  echo ""

  # Check for deployment providers
  if [[ -f "vercel.json" ]] || [[ -d ".vercel" ]]; then
    printf "${COLOR_CYAN}➞ Deploying with Vercel${COLOR_RESET}\n"
    echo ""

    if command -v vercel >/dev/null 2>&1; then
      if [[ "$target_env" == "production" ]]; then
        vercel --prod
      else
        vercel
      fi

      # Record deployment
      local timestamp=$(date -Iseconds)
      echo "{\"frontend\": \"$name\", \"provider\": \"vercel\", \"env\": \"$target_env\", \"timestamp\": \"$timestamp\"}" \
        >>"${FRONTEND_DIR}/deployments.jsonl"

      log_success "Deployment complete"
    else
      log_error "Vercel CLI not installed"
      log_info "Install with: npm i -g vercel"
      return 1
    fi

  elif [[ -f "netlify.toml" ]] || [[ -d ".netlify" ]]; then
    printf "${COLOR_CYAN}➞ Deploying with Netlify${COLOR_RESET}\n"
    echo ""

    if command -v netlify >/dev/null 2>&1; then
      if [[ "$target_env" == "production" ]]; then
        netlify deploy --prod
      else
        netlify deploy
      fi

      log_success "Deployment complete"
    else
      log_error "Netlify CLI not installed"
      log_info "Install with: npm i -g netlify-cli"
      return 1
    fi

  else
    log_warning "No deployment provider configured"
    echo ""
    log_info "Supported providers:"
    echo "  - Vercel: Create vercel.json"
    echo "  - Netlify: Create netlify.toml"
    echo ""
    log_info "Or deploy manually and update DNS to point to your frontend"
  fi
}

# View deploy logs
cmd_logs() {
  local name="$1"
  local limit="${LIMIT:-20}"
  local json_mode="${JSON_OUTPUT:-false}"

  init_frontend

  local log_file="${FRONTEND_DIR}/deployments.jsonl"

  if [[ ! -f "$log_file" ]]; then
    log_info "No deployment logs found"
    return 0
  fi

  if [[ "$json_mode" != "true" ]]; then
    show_command_header "nself frontend" "Deployment Logs"
    echo ""

    printf "  %-20s %-15s %-15s %s\n" "Frontend" "Provider" "Environment" "Timestamp"
    printf "  %-20s %-15s %-15s %s\n" "--------" "--------" "-----------" "---------"
  fi

  # SECURITY: Use direct grep instead of eval with user-influenced filter
  # Apply frontend name filter safely without eval
  _frontend_filter_log() {
    local _file="$1"
    local _name="$2"
    if [[ -n "$_name" ]]; then
      grep "\"frontend\": *\"$_name\"" "$_file" 2>/dev/null || true
    else
      cat "$_file"
    fi
  }

  if [[ "$json_mode" == "true" ]]; then
    printf '{"deployments": ['
    _frontend_filter_log "$log_file" "$name" | tail -n "$limit" | tr '\n' ',' | sed 's/,$//'
    printf ']}\n'
  else
    _frontend_filter_log "$log_file" "$name" | tail -n "$limit" | while read -r line; do
      local frontend=$(echo "$line" | grep -o '"frontend": *"[^"]*"' | sed 's/"frontend": *"\([^"]*\)"/\1/')
      local provider=$(echo "$line" | grep -o '"provider": *"[^"]*"' | sed 's/"provider": *"\([^"]*\)"/\1/')
      local env=$(echo "$line" | grep -o '"env": *"[^"]*"' | sed 's/"env": *"\([^"]*\)"/\1/')
      local ts=$(echo "$line" | grep -o '"timestamp": *"[^"]*"' | sed 's/"timestamp": *"\([^"]*\)"/\1/' | cut -d'+' -f1 | tr 'T' ' ')

      printf "  %-20s %-15s %-15s %s\n" "$frontend" "$provider" "$env" "$ts"
    done
  fi
}

# Show environment variables
cmd_env() {
  local name="$1"
  local json_mode="${JSON_OUTPUT:-false}"

  if [[ -z "$name" ]]; then
    log_error "Frontend name required"
    return 1
  fi

  init_frontend

  if [[ "$json_mode" != "true" ]]; then
    show_command_header "nself frontend" "Environment: $name"
    echo ""
  fi

  # Find frontend config
  local found=false
  for i in {1..20}; do
    local name_var="FRONTEND_APP_${i}_NAME"
    if [[ "${!name_var:-}" == "$name" ]]; then
      found=true
      break
    fi
  done

  if [[ "$found" != "true" ]]; then
    log_error "Frontend not found: $name"
    return 1
  fi

  # Generate frontend environment variables
  local api_url="https://api.${BASE_DOMAIN}/v1/graphql"
  local auth_url="https://auth.${BASE_DOMAIN}"
  local storage_url="https://minio.${BASE_DOMAIN}"

  if [[ "$json_mode" == "true" ]]; then
    cat <<EOF
{
  "NEXT_PUBLIC_GRAPHQL_URL": "$api_url",
  "NEXT_PUBLIC_AUTH_URL": "$auth_url",
  "NEXT_PUBLIC_STORAGE_URL": "$storage_url",
  "VITE_GRAPHQL_URL": "$api_url",
  "VITE_AUTH_URL": "$auth_url",
  "REACT_APP_GRAPHQL_URL": "$api_url",
  "REACT_APP_AUTH_URL": "$auth_url"
}
EOF
  else
    printf "${COLOR_CYAN}➞ Environment Variables${COLOR_RESET}\n"
    echo ""
    echo "# Next.js"
    echo "NEXT_PUBLIC_GRAPHQL_URL=$api_url"
    echo "NEXT_PUBLIC_AUTH_URL=$auth_url"
    echo "NEXT_PUBLIC_STORAGE_URL=$storage_url"
    echo ""
    echo "# Vite"
    echo "VITE_GRAPHQL_URL=$api_url"
    echo "VITE_AUTH_URL=$auth_url"
    echo ""
    echo "# Create React App"
    echo "REACT_APP_GRAPHQL_URL=$api_url"
    echo "REACT_APP_AUTH_URL=$auth_url"
    echo ""
    log_info "Copy these to your frontend's .env.local"
  fi
}

# Main command handler
cmd_frontend() {
  local subcommand="${1:-status}"

  # Check for help first
  if [[ "$subcommand" == "-h" ]] || [[ "$subcommand" == "--help" ]]; then
    show_frontend_help
    return 0
  fi

  # Parse global options
  local args=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --port)
        FRONTEND_PORT="$2"
        shift 2
        ;;
      --route)
        FRONTEND_ROUTE="$2"
        shift 2
        ;;
      --provider)
        DEPLOY_PROVIDER="$2"
        shift 2
        ;;
      --env)
        TARGET_ENV="$2"
        shift 2
        ;;
      --limit)
        LIMIT="$2"
        shift 2
        ;;
      --json)
        JSON_OUTPUT=true
        shift
        ;;
      -h | --help)
        show_frontend_help
        return 0
        ;;
      *)
        args+=("$1")
        shift
        ;;
    esac
  done

  # Restore positional arguments
  set -- "${args[@]}"
  subcommand="${1:-status}"

  case "$subcommand" in
    status)
      cmd_status
      ;;
    list)
      cmd_list
      ;;
    add)
      shift
      cmd_add "$@"
      ;;
    remove)
      shift
      cmd_remove "$@"
      ;;
    deploy)
      shift
      cmd_deploy "$@"
      ;;
    logs)
      shift
      cmd_logs "$@"
      ;;
    env)
      shift
      cmd_env "$@"
      ;;
    *)
      log_error "Unknown subcommand: $subcommand"
      show_frontend_help
      return 1
      ;;
  esac
}

# Export for use as library
export -f cmd_frontend

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_frontend_help
      exit 0
    fi
  done
  pre_command "frontend" || exit $?
  cmd_frontend "$@"
  exit_code=$?
  post_command "frontend" $exit_code
  exit $exit_code
fi
