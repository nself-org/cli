#!/usr/bin/env bash

# demo.sh - Demo setup functionality for nself init --demo
#
# Creates a complete demo environment with all services enabled,
# custom backend services, frontend apps, and remote schemas

# Source required utilities
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source display utilities for consistent styling
if [[ -f "$DEMO_DIR/../utils/display.sh" ]]; then
  source "$DEMO_DIR/../utils/display.sh"
else
  # Fallback definitions if display.sh not available
  show_command_header() {
    echo ""
    printf "\033[1;34m%s\033[0m\n" "$1"
    echo "$2"
    echo ""
  }
  safe_echo() {
    echo "$@"
  }
  log_success() {
    printf "\033[32m✓\033[0m %s\n" "$1"
  }
  log_info() {
    printf "\033[34mℹ\033[0m %s\n" "$1"
  }
  log_warning() {
    printf "\033[33m⚠\033[0m %s\n" "$1"
  }
  log_error() {
    printf "\033[31m✗\033[0m %s\n" "$1"
  }
fi

source "$DEMO_DIR/../utils/env.sh" 2>/dev/null || true
source "$DEMO_DIR/gitignore.sh" 2>/dev/null || true

# Setup complete demo environment
# Inputs: $1 - script directory
# Outputs: Creates demo configuration files
# Returns: 0 on success, error code on failure
setup_demo() {
  local script_dir="${1:-$DEMO_DIR}"
  # Find the templates directory relative to the init module
  local templates_dir
  templates_dir="$(cd "$DEMO_DIR" && cd ../../templates/demo && pwd)"

  # Show standard header with demo subtitle
  show_command_header "nself init --demo" "Create a complete demo application"

  echo "✓ All core services (PostgreSQL, Hasura, Auth, Nginx)"
  echo "✓ All optional services enabled (17 services)"
  echo "✓ Full monitoring bundle (10 services)"
  echo "✓ 4 Custom backend services (Express, BullMQ, Go gRPC, Python)"
  echo "✓ 2 Frontend applications (app1 & app2)"
  echo "✓ Full nginx routing with SSL/TLS"
  echo ""

  # Check templates exist
  if [[ ! -d "$templates_dir" ]] || [[ ! -f "$templates_dir/.env.dev" ]]; then
    log_error "Demo templates not found at $templates_dir"
    log_info "Please ensure nself is properly installed"
    return 1
  fi

  # Copy all demo environment files
  # .env - Local overrides (git-ignored)
  if [[ -f "$templates_dir/.env" ]]; then
    cp "$templates_dir/.env" .env
  fi

  # .env.dev - Development environment (default)
  if [[ -f "$templates_dir/.env.dev" ]]; then
    cp "$templates_dir/.env.dev" .env.dev
  fi

  # .env.staging - Staging environment
  if [[ -f "$templates_dir/.env.staging" ]]; then
    cp "$templates_dir/.env.staging" .env.staging
  fi

  # .env.prod - Production environment
  if [[ -f "$templates_dir/.env.prod" ]]; then
    cp "$templates_dir/.env.prod" .env.prod
  fi

  # .env.example - Complete reference (from envs template)
  local envs_template_dir="$(cd "$DEMO_DIR" && cd ../../templates/envs && pwd)"
  if [[ -f "$envs_template_dir/.env.example" ]]; then
    cp "$envs_template_dir/.env.example" .env.example
  fi

  # Create .env if it doesn't exist from template
  if [[ ! -f ".env" ]]; then
    cat >.env <<'EOF'
# Local Configuration Overrides for Demo
# Essential variables are pre-filled from demo configuration

# Project identification
PROJECT_NAME=demo-app
ENV=dev

# Core domain configuration
BASE_DOMAIN=localhost

# Database configuration
POSTGRES_USER=postgres
POSTGRES_PASSWORD=demo-postgres-$(openssl rand -hex 8)
POSTGRES_DB=demo-app_db

# Network configuration
DOCKER_NETWORK=demo-app_network

# Auth configuration (generated securely for demo)
AUTH_JWT_SECRET=demo-jwt-$(openssl rand -base64 32 | tr -d '/+=' | head -c 32)
AUTH_SMTP_USER=
AUTH_SMTP_PASS=

# Hasura configuration
HASURA_GRAPHQL_ADMIN_SECRET=demo-admin-$(openssl rand -base64 16 | tr -d '/+=' | head -c 24)
HASURA_GRAPHQL_JWT_SECRET='{"type":"HS256","key":"'$(openssl rand -base64 32 | tr -d '/+=' | head -c 32)'"}'
DATABASE_URL=postgres://postgres:postgres@postgres:5432/demo-app_db

# Add any personal overrides below:
# POSTGRES_PORT=5433
# REDIS_PORT=6380
EOF
  fi

  # Ensure gitignore
  if [[ -f "$script_dir/gitignore.sh" ]]; then
    source "$script_dir/gitignore.sh"
    ensure_gitignore >/dev/null 2>&1
  fi

  echo "Next steps:"
  echo ""
  safe_echo "${COLOR_BLUE:-}1.${COLOR_RESET:-} Edit .env to customize (optional)"
  safe_echo "   ${COLOR_DIM:-}All services are pre-configured for demo${COLOR_RESET:-}"
  echo ""
  safe_echo "${COLOR_BLUE:-}2.${COLOR_RESET:-} nself build - Generate project files"
  safe_echo "   ${COLOR_DIM:-}Creates Docker configs and services${COLOR_RESET:-}"
  echo ""
  safe_echo "${COLOR_BLUE:-}3.${COLOR_RESET:-} nself start - Start your backend"
  safe_echo "   ${COLOR_DIM:-}Launches all demo services${COLOR_RESET:-}"
  echo ""

  # Add help line at bottom
  echo "For more help, use: nself help or nself help init"
  echo ""

  return 0
}

# Export the function
export -f setup_demo
