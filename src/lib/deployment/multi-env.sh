#!/usr/bin/env bash


# multi-env.sh - Multi-environment deployment support

# Environment configurations
ENVIRONMENTS_DIR="${ENVIRONMENTS_DIR:-./environments}"

set -euo pipefail

CURRENT_ENV_FILE="${CURRENT_ENV_FILE:-./.current-env}"

# Initialize environment directory
init_environments() {
  mkdir -p "$ENVIRONMENTS_DIR"

  # Create default environments if they don't exist
  for env in development staging production; do
    if [[ ! -f "$ENVIRONMENTS_DIR/$env.env" ]]; then
      create_default_env "$env"
    fi
  done
}

# Create default environment configuration
create_default_env() {
  local env_name="$1"
  local env_file="$ENVIRONMENTS_DIR/$env_name.env"

  case "$env_name" in
    development)
      cat >"$env_file" <<EOF
# Development Environment
ENV=development
DEBUG=true
LOG_LEVEL=debug
BASE_DOMAIN=local.nself.org
SSL_ENABLED=true
MONITORING_ENABLED=false
BACKUP_ENABLED=false
EOF
      ;;
    staging)
      cat >"$env_file" <<EOF
# Staging Environment
ENV=staging
DEBUG=false
LOG_LEVEL=info
BASE_DOMAIN=staging.example.com
SSL_ENABLED=true
MONITORING_ENABLED=true
BACKUP_ENABLED=true
BACKUP_SCHEDULE=daily
EOF
      ;;
    production)
      cat >"$env_file" <<EOF
# Production Environment
ENV=production
DEBUG=false
LOG_LEVEL=warning
BASE_DOMAIN=example.com
SSL_ENABLED=true
MONITORING_ENABLED=true
BACKUP_ENABLED=true
BACKUP_SCHEDULE=hourly
ALERTING_ENABLED=true
EOF
      ;;
  esac
}

# Switch to environment
switch_environment() {
  local env_name="$1"
  local env_file="$ENVIRONMENTS_DIR/$env_name.env"

  if [[ ! -f "$env_file" ]]; then
    echo "Environment not found: $env_name"
    return 1
  fi

  # Backup current environment
  if [[ -f ".env.local" ]]; then
    cp .env.local ".env.local.backup-$(date +%Y%m%d-%H%M%S)"
  fi

  # Load new environment
  cp "$env_file" .env.local
  echo "$env_name" >"$CURRENT_ENV_FILE"

  echo "Switched to environment: $env_name"
}

# Get current environment
get_current_environment() {
  if [[ -f "$CURRENT_ENV_FILE" ]]; then
    cat "$CURRENT_ENV_FILE"
  else
    echo "development"
  fi
}

# List environments
list_environments() {
  local current=$(get_current_environment)

  echo "Available environments:"
  for env_file in "$ENVIRONMENTS_DIR"/*.env; do
    if [[ -f "$env_file" ]]; then
      local env_name=$(basename "$env_file" .env)
      if [[ "$env_name" == "$current" ]]; then
        echo "  * $env_name (current)"
      else
        echo "    $env_name"
      fi
    fi
  done
}

# Deploy to environment
deploy_to_environment() {
  local env_name="$1"
  local current=$(get_current_environment)

  if [[ "$env_name" != "$current" ]]; then
    switch_environment "$env_name"
  fi

  # Environment-specific deployment steps
  case "$env_name" in
    production)
      # Production deployment
      echo "Deploying to production..."

      # Run pre-deployment checks
      nself doctor || return 1

      # Create backup
      nself backup create pre-deploy

      # Deploy with zero-downtime
      nself deploy --zero-downtime

      # Run post-deployment tests
      nself test production
      ;;
    staging)
      # Staging deployment
      echo "Deploying to staging..."

      # Deploy normally
      nself deploy

      # Run integration tests
      nself test integration
      ;;
    development)
      # Development deployment
      echo "Deploying to development..."

      # Simple deployment
      nself build
      nself start
      ;;
  esac
}

# Export functions
export -f init_environments
export -f switch_environment
export -f get_current_environment
export -f list_environments
export -f deploy_to_environment
